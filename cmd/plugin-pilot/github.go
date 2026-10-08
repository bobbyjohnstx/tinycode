package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type githubProvider struct {
	cfg providerConfig
}

func newGitHubProvider() *githubProvider {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return &githubProvider{
		cfg: providerConfig{
			baseURL:    "https://api.github.com",
			token:      token,
			authHeader: "Authorization",
			authValue:  "Bearer " + token,
			extraHeaders: map[string]string{
				"Accept": "application/vnd.github+json",
			},
		},
	}
}

// newGitHubProviderWithConfig creates a githubProvider with a custom config (for testing).
func newGitHubProviderWithConfig(cfg providerConfig) *githubProvider {
	return &githubProvider{cfg: cfg}
}

func (g *githubProvider) Name() string { return "github" }

func (g *githubProvider) checkToken() error {
	if g.cfg.token == "" {
		return &ProviderError{
			Msg:      "GitHub token not configured. Set GITHUB_TOKEN or GH_TOKEN environment variable.",
			Provider: "github",
		}
	}
	return nil
}

func (g *githubProvider) handleStatus(status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	switch status {
	case http.StatusNotFound:
		return &ProviderError{Msg: "Repository or issue not found", StatusCode: 404, Provider: "github"}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{Msg: "Authentication failed. Check your GITHUB_TOKEN.", StatusCode: status, Provider: "github"}
	default:
		return &ProviderError{Msg: fmt.Sprintf("GitHub API error: %d", status), StatusCode: status, Provider: "github"}
	}
}

func normalizeGitHubIssue(raw json.RawMessage) NormalizedIssue {
	var data struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		HTMLURL   string `json:"html_url"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Labels    []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	_ = json.Unmarshal(raw, &data)

	labels := make([]string, len(data.Labels))
	for i, l := range data.Labels {
		labels[i] = l.Name
	}

	return NormalizedIssue{
		Number:    data.Number,
		Title:     data.Title,
		Body:      data.Body,
		State:     data.State,
		Labels:    labels,
		URL:       data.HTMLURL,
		CreatedAt: data.CreatedAt,
		UpdatedAt: data.UpdatedAt,
	}
}

func (g *githubProvider) ListIssues(ctx context.Context, params ListIssuesParams) ([]NormalizedIssue, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/repos/%s/%s/issues", params.Owner, params.Repo)
	sep := "?"
	if params.State != "" {
		path += sep + "state=" + params.State
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
		issues[i] = normalizeGitHubIssue(raw)
	}
	return issues, nil
}

func (g *githubProvider) CreateIssue(ctx context.Context, params CreateIssueParams) (*NormalizedIssue, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"title": params.Title,
	}
	if params.Body != "" {
		reqBody["body"] = params.Body
	}
	if len(params.Labels) > 0 {
		reqBody["labels"] = params.Labels
	}

	body, status, err := doRequest(ctx, g.cfg, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", params.Owner, params.Repo), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	issue := normalizeGitHubIssue(body)
	return &issue, nil
}

func (g *githubProvider) UpdateIssue(ctx context.Context, params UpdateIssueParams) (*NormalizedIssue, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	reqBody := map[string]any{}
	if params.Title != "" {
		reqBody["title"] = params.Title
	}
	if params.Body != "" {
		reqBody["body"] = params.Body
	}
	if params.State != "" {
		reqBody["state"] = params.State
	}

	body, status, err := doRequest(ctx, g.cfg, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/issues/%d", params.Owner, params.Repo, params.IssueNumber), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	issue := normalizeGitHubIssue(body)
	return &issue, nil
}

func (g *githubProvider) CommentOnIssue(ctx context.Context, params CommentParams) (*NormalizedComment, error) {
	if err := g.checkToken(); err != nil {
		return nil, err
	}

	reqBody := map[string]any{"body": params.Body}
	body, status, err := doRequest(ctx, g.cfg, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", params.Owner, params.Repo, params.IssueNumber), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	var data struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		HTMLURL   string `json:"html_url"`
		CreatedAt string `json:"created_at"`
	}
	_ = json.Unmarshal(body, &data)

	return &NormalizedComment{
		ID:        data.ID,
		Body:      data.Body,
		URL:       data.HTMLURL,
		CreatedAt: data.CreatedAt,
	}, nil
}
