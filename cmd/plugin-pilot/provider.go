package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// NormalizedIssue is a provider-agnostic issue representation.
type NormalizedIssue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	Labels    []string `json:"labels"`
	URL       string   `json:"url"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// NormalizedComment is a provider-agnostic comment representation.
type NormalizedComment struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

// ListIssuesParams holds parameters for listing issues.
type ListIssuesParams struct {
	Owner  string
	Repo   string
	State  string
	Labels string
	Page   int
	Limit  int
}

// CreateIssueParams holds parameters for creating an issue.
type CreateIssueParams struct {
	Owner  string
	Repo   string
	Title  string
	Body   string
	Labels []string
}

// UpdateIssueParams holds parameters for updating an issue.
type UpdateIssueParams struct {
	Owner       string
	Repo        string
	IssueNumber int
	Title       string
	Body        string
	State       string
}

// CommentParams holds parameters for commenting on an issue.
type CommentParams struct {
	Owner       string
	Repo        string
	IssueNumber int
	Body        string
}

// IssueProvider is the interface that each platform implements.
type IssueProvider interface {
	Name() string
	ListIssues(ctx context.Context, params ListIssuesParams) ([]NormalizedIssue, error)
	CreateIssue(ctx context.Context, params CreateIssueParams) (*NormalizedIssue, error)
	UpdateIssue(ctx context.Context, params UpdateIssueParams) (*NormalizedIssue, error)
	CommentOnIssue(ctx context.Context, params CommentParams) (*NormalizedComment, error)
}

// ProviderError is returned when a provider API call fails.
type ProviderError struct {
	Msg        string
	StatusCode int
	Provider   string
}

func (e *ProviderError) Error() string {
	return e.Msg
}

// providerConfig holds common HTTP config for a provider.
type providerConfig struct {
	baseURL      string
	token        string
	authHeader   string
	authValue    string
	extraHeaders map[string]string
}

const maxProviderBody = 2 << 20

var providerHTTP = &http.Client{Timeout: 30 * time.Second}

// doRequest performs an HTTP request against the provider API and returns the
// decoded JSON response body. method defaults to GET if empty.
func doRequest(ctx context.Context, cfg providerConfig, method, path string, body any) (json.RawMessage, int, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.authHeader != "" && cfg.authValue != "" {
		req.Header.Set(cfg.authHeader, cfg.authValue)
	}
	for k, v := range cfg.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := providerHTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	if len(respBody) > maxProviderBody {
		return nil, resp.StatusCode, fmt.Errorf("response exceeds %d bytes", maxProviderBody)
	}

	return json.RawMessage(respBody), resp.StatusCode, nil
}

// createProvider builds the appropriate IssueProvider based on git remote URL
// and environment variables.
func createProvider(directory string) IssueProvider {
	envOverride := os.Getenv("PILOT_PROVIDER")

	var remoteURL string
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = directory
	out, err := cmd.Output()
	if err == nil {
		remoteURL = strings.TrimSpace(string(out))
	}

	providerName := detectProvider(remoteURL, envOverride)

	switch providerName {
	case "github":
		return newGitHubProvider()
	case "gitlab":
		return newGitLabProvider()
	default:
		return newGiteaProvider()
	}
}
