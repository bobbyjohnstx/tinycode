package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type gitlabProvider struct {
	cfg providerConfig
}

func newGitLabProvider() *gitlabProvider {
	baseURL := os.Getenv("GITLAB_URL")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	token := os.Getenv("GITLAB_TOKEN")
	return &gitlabProvider{
		cfg: providerConfig{
			baseURL:    baseURL + "/api/v4",
			token:      token,
			authHeader: "PRIVATE-TOKEN",
			authValue:  token,
		},
	}
}

// newGitLabProviderWithConfig creates a gitlabProvider with a custom config (for testing).
func newGitLabProviderWithConfig(cfg providerConfig) *gitlabProvider {
	return &gitlabProvider{cfg: cfg}
}

func (g *gitlabProvider) Name() string { return "gitlab" }

func (g *gitlabProvider) projectPath(owner, repo string) string {
	return url.PathEscape(owner + "/" + repo)
}

func (g *gitlabProvider) checkToken() error {
	if g.cfg.token == "" {
		return &ProviderError{
			Msg:      "GitLab token not configured. Set GITLAB_TOKEN environment variable.",
			Provider: "gitlab",
		}
	}
	return nil
}

func (g *gitlabProvider) handleStatus(status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	switch status {
	case http.StatusNotFound:
		return &ProviderError{Msg: "Repository or issue not found", StatusCode: 404, Provider: "gitlab"}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{Msg: "Authentication failed. Check your GITLAB_TOKEN.", StatusCode: status, Provider: "gitlab"}
	default:
		return &ProviderError{Msg: fmt.Sprintf("GitLab API error: %d", status), StatusCode: status, Provider: "gitlab"}
	}
}

// mapStateFromGitLab converts GitLab state to normalized state.
func mapStateFromGitLab(state string) string {
	if state == "opened" {
		return "open"
	}
	return "closed"
}

// mapStateToGitLab converts normalized state to GitLab state.
func mapStateToGitLab(state string) string {
	if state == "open" {
		return "opened"
	}
	return "closed"
}

// stateEvent converts a target state to a GitLab state_event value.
func stateEvent(state string) string {
	if state == "closed" {
		return "close"
	}
	return "reopen"
}

func normalizeGitLabIssue(raw json.RawMessage) NormalizedIssue {
	var data struct {
		IID         int      `json:"iid"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		State       string   `json:"state"`
		Labels      []string `json:"labels"`
		WebURL      string   `json:"web_url"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
	}
	_ = json.Unmarshal(raw, &data)

	labels := data.Labels
	if labels == nil {
		labels = []string{}
	}

	return NormalizedIssue{
		Number:    data.IID,
		Title:     data.Title,
		Body:      data.Description,
		State:     mapStateFromGitLab(data.State),
		Labels:    labels,
		URL:       data.WebURL,
		CreatedAt: data.CreatedAt,
		UpdatedAt: data.UpdatedAt,
	}
}

func (g *gitlabProvider) ListIssues(ctx context.Context, params ListIssuesParams) ([]NormalizedIssue, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	project := g.projectPath(params.Owner, params.Repo)
	path := fmt.Sprintf("/projects/%s/issues", project)
	sep := "?"
	if params.State != "" {
		path += sep + "state=" + mapStateToGitLab(params.State)
		sep = "&"
	}
	if params.Labels != "" {
		path += sep + "labels=" + params.Labels
		sep = "&"
	}
	if params.Page > 0 {
		path += sep + fmt.Sprintf("page=%d", params.Page)
		sep = "&"
	}
	if params.Limit > 0 {
		path += sep + fmt.Sprintf("per_page=%d", params.Limit)
	}

	body, status, err := doRequest(ctx, g.cfg, "", path, nil)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	var rawIssues []json.RawMessage
	if err := json.Unmarshal(body, &rawIssues); err != nil {
		return nil, fmt.Errorf("parsing issues: %w", err)
	}

	issues := make([]NormalizedIssue, len(rawIssues))
	for i, raw := range rawIssues {
		issues[i] = normalizeGitLabIssue(raw)
	}
	return issues, nil
}

func (g *gitlabProvider) CreateIssue(ctx context.Context, params CreateIssueParams) (*NormalizedIssue, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	project := g.projectPath(params.Owner, params.Repo)
	reqBody := map[string]any{
		"title": params.Title,
	}
	if params.Body != "" {
		reqBody["description"] = params.Body
	}
	if len(params.Labels) > 0 {
		reqBody["labels"] = strings.Join(params.Labels, ",")
	}

	body, status, err := doRequest(ctx, g.cfg, http.MethodPost, fmt.Sprintf("/projects/%s/issues", project), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	issue := normalizeGitLabIssue(body)
	return &issue, nil
}

func (g *gitlabProvider) UpdateIssue(ctx context.Context, params UpdateIssueParams) (*NormalizedIssue, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	project := g.projectPath(params.Owner, params.Repo)
	reqBody := map[string]any{}
	if params.Title != "" {
		reqBody["title"] = params.Title
	}
	if params.Body != "" {
		reqBody["description"] = params.Body
	}
	if params.State != "" {
		reqBody["state_event"] = stateEvent(params.State)
	}

	body, status, err := doRequest(ctx, g.cfg, http.MethodPut, fmt.Sprintf("/projects/%s/issues/%d", project, params.IssueNumber), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	issue := normalizeGitLabIssue(body)
	return &issue, nil
}

func (g *gitlabProvider) CommentOnIssue(ctx context.Context, params CommentParams) (*NormalizedComment, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	project := g.projectPath(params.Owner, params.Repo)
	reqBody := map[string]any{"body": params.Body}
	body, status, err := doRequest(ctx, g.cfg, http.MethodPost, fmt.Sprintf("/projects/%s/issues/%d/notes", project, params.IssueNumber), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	var data struct {
		ID          int    `json:"id"`
		Body        string `json:"body"`
		NoteableIID int    `json:"noteable_iid"`
		CreatedAt   string `json:"created_at"`
	}
	_ = json.Unmarshal(body, &data)

	commentURL := ""
	if data.NoteableIID > 0 {
		// Construct GitLab note URL from the base URL (strip /api/v4).
		base := strings.TrimSuffix(g.cfg.baseURL, "/api/v4")
		commentURL = fmt.Sprintf("%s/-/issues/%d#note_%d", base, data.NoteableIID, data.ID)
	}

	return &NormalizedComment{
		ID:        data.ID,
		Body:      data.Body,
		URL:       commentURL,
		CreatedAt: data.CreatedAt,
	}, nil
}
