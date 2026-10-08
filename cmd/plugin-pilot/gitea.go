package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type giteaProvider struct {
	cfg providerConfig
}

func newGiteaProvider() *giteaProvider {
	baseURL := os.Getenv("GITEA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	token := os.Getenv("GITEA_TOKEN")
	return &giteaProvider{
		cfg: providerConfig{
			baseURL:    baseURL + "/api/v1",
			token:      token,
			authHeader: "Authorization",
			authValue:  "token " + token,
		},
	}
}

// newGiteaProviderWithConfig creates a giteaProvider with a custom config (for testing).
func newGiteaProviderWithConfig(cfg providerConfig) *giteaProvider {
	return &giteaProvider{cfg: cfg}
}

func (g *giteaProvider) Name() string { return "gitea" }

func (g *giteaProvider) checkToken() error {
	if g.cfg.token == "" {
		return &ProviderError{
			Msg:      "Gitea token not configured. Set GITEA_TOKEN environment variable.",
			Provider: "gitea",
		}
	}
	return nil
}

func (g *giteaProvider) handleStatus(status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	switch status {
	case http.StatusNotFound:
		return &ProviderError{Msg: "Repository or issue not found", StatusCode: 404, Provider: "gitea"}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{Msg: "Authentication failed. Check your GITEA_TOKEN.", StatusCode: status, Provider: "gitea"}
	default:
		return &ProviderError{Msg: fmt.Sprintf("Gitea API error: %d", status), StatusCode: status, Provider: "gitea"}
	}
}

func normalizeGiteaIssue(raw json.RawMessage) NormalizedIssue {
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

func (g *giteaProvider) resolveLabels(ctx context.Context, owner, repo string, names []string) ([]int, error) {
	body, status, err := doRequest(ctx, g.cfg, "", fmt.Sprintf("/repos/%s/%s/labels", owner, repo), nil)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	var allLabels []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &allLabels); err != nil {
		return nil, fmt.Errorf("parsing labels: %w", err)
	}

	labelMap := make(map[string]int, len(allLabels))
	for _, l := range allLabels {
		labelMap[l.Name] = l.ID
	}

	var ids []int
	var missing []string
	for _, name := range names {
		if id, ok := labelMap[name]; ok {
			ids = append(ids, id)
		} else {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		quoted := make([]string, len(missing))
		for i, n := range missing {
			quoted[i] = fmt.Sprintf("%q", n)
		}
		return nil, &ProviderError{
			Msg:      fmt.Sprintf("Labels not found in repository: %s", strings.Join(quoted, ", ")),
			Provider: "gitea",
		}
	}

	return ids, nil
}

func (g *giteaProvider) ListIssues(ctx context.Context, params ListIssuesParams) ([]NormalizedIssue, error) {
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
		path += sep + fmt.Sprintf("limit=%d", params.Limit)
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
		issues[i] = normalizeGiteaIssue(raw)
	}
	return issues, nil
}

func (g *giteaProvider) CreateIssue(ctx context.Context, params CreateIssueParams) (*NormalizedIssue, error) {
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
		ids, err := g.resolveLabels(ctx, params.Owner, params.Repo, params.Labels)
		if err != nil {
			return nil, err
		}
		reqBody["labels"] = ids
	}

	body, status, err := doRequest(ctx, g.cfg, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", params.Owner, params.Repo), reqBody)
	if err != nil {
		return nil, err
	}
	if err := g.handleStatus(status); err != nil {
		return nil, err
	}

	issue := normalizeGiteaIssue(body)
	return &issue, nil
}

func (g *giteaProvider) UpdateIssue(ctx context.Context, params UpdateIssueParams) (*NormalizedIssue, error) {
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

	issue := normalizeGiteaIssue(body)
	return &issue, nil
}

func (g *giteaProvider) CommentOnIssue(ctx context.Context, params CommentParams) (*NormalizedComment, error) {
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
