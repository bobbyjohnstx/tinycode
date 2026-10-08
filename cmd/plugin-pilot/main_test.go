package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Remote URL parsing tests ---

func TestParseGitRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want *ParsedRemote
	}{
		{
			name: "https URL",
			url:  "https://github.com/owner/repo.git",
			want: &ParsedRemote{Host: "github.com", Owner: "owner", Repo: "repo"},
		},
		{
			name: "https URL without .git",
			url:  "https://github.com/owner/repo",
			want: &ParsedRemote{Host: "github.com", Owner: "owner", Repo: "repo"},
		},
		{
			name: "git@ SSH URL",
			url:  "git@github.com:owner/repo.git",
			want: &ParsedRemote{Host: "github.com", Owner: "owner", Repo: "repo"},
		},
		{
			name: "git@ SSH URL without .git",
			url:  "git@gitlab.com:owner/repo",
			want: &ParsedRemote{Host: "gitlab.com", Owner: "owner", Repo: "repo"},
		},
		{
			name: "ssh:// protocol URL",
			url:  "ssh://git@gitea.local/owner/repo.git",
			want: &ParsedRemote{Host: "gitea.local", Owner: "owner", Repo: "repo"},
		},
		{
			name: "nested groups (GitLab style)",
			url:  "https://gitlab.com/group/subgroup/repo.git",
			want: &ParsedRemote{Host: "gitlab.com", Owner: "group/subgroup", Repo: "repo"},
		},
		{
			name: "deeply nested groups",
			url:  "git@gitlab.com:org/team/sub/repo.git",
			want: &ParsedRemote{Host: "gitlab.com", Owner: "org/team/sub", Repo: "repo"},
		},
		{
			name: "invalid URL",
			url:  "not-a-url",
			want: nil,
		},
		{
			name: "empty string",
			url:  "",
			want: nil,
		},
		{
			name: "single path segment",
			url:  "https://github.com/onlyone",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGitRemoteURL(tt.url)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %+v, got nil", tt.want)
			}
			if got.Host != tt.want.Host {
				t.Errorf("Host: expected %q, got %q", tt.want.Host, got.Host)
			}
			if got.Owner != tt.want.Owner {
				t.Errorf("Owner: expected %q, got %q", tt.want.Owner, got.Owner)
			}
			if got.Repo != tt.want.Repo {
				t.Errorf("Repo: expected %q, got %q", tt.want.Repo, got.Repo)
			}
		})
	}
}

// --- Provider detection tests ---

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name        string
		remoteURL   string
		envOverride string
		want        string
	}{
		{name: "github.com host", remoteURL: "https://github.com/owner/repo", want: "github"},
		{name: "gitlab.com host", remoteURL: "git@gitlab.com:owner/repo.git", want: "gitlab"},
		{name: "unknown host defaults to gitea", remoteURL: "https://gitea.local/owner/repo", want: "gitea"},
		{name: "env override github", remoteURL: "https://gitea.local/owner/repo", envOverride: "github", want: "github"},
		{name: "env override gitlab", remoteURL: "https://github.com/owner/repo", envOverride: "gitlab", want: "gitlab"},
		{name: "env override gitea", remoteURL: "https://github.com/owner/repo", envOverride: "gitea", want: "gitea"},
		{name: "invalid env override ignored", remoteURL: "https://github.com/owner/repo", envOverride: "invalid", want: "github"},
		{name: "empty remote defaults to gitea", remoteURL: "", want: "gitea"},
		{name: "invalid remote defaults to gitea", remoteURL: "garbage", want: "gitea"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectProvider(tt.remoteURL, tt.envOverride)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// --- Plugin structure tests ---

func TestPluginID(t *testing.T) {
	p := newPlugin(&stubProvider{})
	if p.ID != "pilot" {
		t.Errorf("expected plugin ID 'pilot', got %q", p.ID)
	}
}

func TestPluginHasFourTools(t *testing.T) {
	p := newPlugin(&stubProvider{})
	if len(p.Tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(p.Tools))
	}

	expected := []string{"pilot_issues_list", "pilot_issue_create", "pilot_issue_update", "pilot_issue_comment"}
	for i, name := range expected {
		if p.Tools[i].Name != name {
			t.Errorf("tool[%d]: expected %q, got %q", i, name, p.Tools[i].Name)
		}
	}
}

// --- Issue list formatting tests ---

func TestFormatIssueListEmpty(t *testing.T) {
	result := formatIssueList(nil, "owner", "repo", "open")
	expected := "No issues found for owner/repo (open)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatIssueListWithIssues(t *testing.T) {
	issues := []NormalizedIssue{
		{Number: 1, Title: "Bug fix", State: "open", Labels: []string{"bug"}, CreatedAt: "2024-01-15T10:00:00Z"},
		{Number: 2, Title: "Feature", State: "open", Labels: []string{}, CreatedAt: "2024-02-20T12:00:00Z"},
	}
	result := formatIssueList(issues, "owner", "repo", "open")

	if got := "## Issues for owner/repo (open)"; !contains(result, got) {
		t.Errorf("missing header in output")
	}
	if !contains(result, "**#1** Bug fix") {
		t.Errorf("missing issue #1 in output")
	}
	if !contains(result, "Labels: bug") {
		t.Errorf("missing labels for issue #1")
	}
	if !contains(result, "Labels: none") {
		t.Errorf("missing 'none' label for issue #2")
	}
	if !contains(result, "Created: 2024-01-15") {
		t.Errorf("missing date for issue #1")
	}
}

// --- Provider HTTP tests (Gitea) ---

func TestGiteaListIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/owner/repo/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("expected state=open, got %q", r.URL.Query().Get("state"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"number": 1, "title": "Test Issue", "state": "open", "html_url": "http://localhost/issues/1", "created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z", "labels": []map[string]any{{"name": "bug"}}},
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "test-token",
		authHeader: "Authorization",
		authValue:  "token test-token",
	})

	issues, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "owner", Repo: "repo", State: "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %q", issues[0].Title)
	}
	if len(issues[0].Labels) != 1 || issues[0].Labels[0] != "bug" {
		t.Errorf("expected labels [bug], got %v", issues[0].Labels)
	}
}

func TestGiteaCreateIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 42, "title": "New Issue", "state": "open",
			"html_url": "http://localhost/issues/42", "created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-01T00:00:00Z", "labels": []any{},
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "test-token",
		authHeader: "Authorization",
		authValue:  "token test-token",
	})

	issue, err := p.CreateIssue(context.Background(), CreateIssueParams{Owner: "owner", Repo: "repo", Title: "New Issue"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Number != 42 {
		t.Errorf("expected issue number 42, got %d", issue.Number)
	}
}

func TestGiteaTokenRequired(t *testing.T) {
	p := newGiteaProviderWithConfig(providerConfig{baseURL: "http://localhost", token: ""})
	_, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "o", Repo: "r"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !contains(err.Error(), "GITEA_TOKEN") {
		t.Errorf("expected error mentioning GITEA_TOKEN, got %q", err.Error())
	}
}

func TestGiteaHandleStatus404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "tok",
		authHeader: "Authorization",
		authValue:  "token tok",
	})
	_, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "o", Repo: "r", State: "open"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T", err)
	}
	if pe.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", pe.StatusCode)
	}
}

// --- Provider HTTP tests (GitHub) ---

func TestGitHubListIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/octocat/hello/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing GitHub Accept header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"number": 10, "title": "GH Issue", "state": "open", "html_url": "https://github.com/octocat/hello/issues/10", "created_at": "2024-03-01T00:00:00Z", "updated_at": "2024-03-01T00:00:00Z", "labels": []map[string]any{{"name": "enhancement"}}},
		})
	}))
	defer srv.Close()

	p := newGitHubProviderWithConfig(providerConfig{
		baseURL:    srv.URL,
		token:      "gh-token",
		authHeader: "Authorization",
		authValue:  "Bearer gh-token",
		extraHeaders: map[string]string{
			"Accept": "application/vnd.github+json",
		},
	})

	issues, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "octocat", Repo: "hello", State: "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Title != "GH Issue" {
		t.Errorf("expected title 'GH Issue', got %q", issues[0].Title)
	}
}

func TestGitHubTokenRequired(t *testing.T) {
	p := newGitHubProviderWithConfig(providerConfig{baseURL: "http://localhost", token: ""})
	_, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "o", Repo: "r"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("expected error mentioning GITHUB_TOKEN, got %q", err.Error())
	}
}

// --- Provider HTTP tests (GitLab) ---

func TestGitLabListIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go's HTTP server decodes %2F to /, so the received path has the decoded form.
		if r.URL.Path != "/api/v4/projects/owner%2Frepo/issues" && r.URL.RawPath != "/api/v4/projects/owner%2Frepo/issues" {
			// Accept either: the raw path contains %2F or the decoded path shows owner/repo.
			if r.URL.Path != "/api/v4/projects/owner/repo/issues" {
				t.Errorf("unexpected path: %s (raw: %s)", r.URL.Path, r.URL.RawPath)
			}
		}
		if r.URL.Query().Get("state") != "opened" {
			t.Errorf("expected state=opened (GitLab format), got %q", r.URL.Query().Get("state"))
		}
		if r.Header.Get("PRIVATE-TOKEN") != "gl-token" {
			t.Errorf("missing or wrong PRIVATE-TOKEN header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"iid": 5, "title": "GL Issue", "state": "opened", "description": "desc", "web_url": "https://gitlab.com/issues/5", "created_at": "2024-04-01T00:00:00Z", "updated_at": "2024-04-01T00:00:00Z", "labels": []string{"backend"}},
		})
	}))
	defer srv.Close()

	p := newGitLabProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v4",
		token:      "gl-token",
		authHeader: "PRIVATE-TOKEN",
		authValue:  "gl-token",
	})

	issues, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "owner", Repo: "repo", State: "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Number != 5 {
		t.Errorf("expected iid 5, got %d", issues[0].Number)
	}
	if issues[0].State != "open" {
		t.Errorf("expected normalized state 'open', got %q", issues[0].State)
	}
	if issues[0].Body != "desc" {
		t.Errorf("expected body 'desc', got %q", issues[0].Body)
	}
}

func TestGitLabStateMapping(t *testing.T) {
	if got := mapStateFromGitLab("opened"); got != "open" {
		t.Errorf("mapStateFromGitLab('opened'): expected 'open', got %q", got)
	}
	if got := mapStateFromGitLab("closed"); got != "closed" {
		t.Errorf("mapStateFromGitLab('closed'): expected 'closed', got %q", got)
	}
	if got := mapStateToGitLab("open"); got != "opened" {
		t.Errorf("mapStateToGitLab('open'): expected 'opened', got %q", got)
	}
	if got := mapStateToGitLab("closed"); got != "closed" {
		t.Errorf("mapStateToGitLab('closed'): expected 'closed', got %q", got)
	}
	if got := stateEvent("closed"); got != "close" {
		t.Errorf("stateEvent('closed'): expected 'close', got %q", got)
	}
	if got := stateEvent("open"); got != "reopen" {
		t.Errorf("stateEvent('open'): expected 'reopen', got %q", got)
	}
}

func TestGitLabUpdateIssueUsesStateEvent(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iid": 1, "title": "Updated", "state": "closed",
			"web_url": "https://gitlab.com/issues/1", "created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-02T00:00:00Z", "labels": []string{},
		})
	}))
	defer srv.Close()

	p := newGitLabProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v4",
		token:      "tok",
		authHeader: "PRIVATE-TOKEN",
		authValue:  "tok",
	})

	_, err := p.UpdateIssue(context.Background(), UpdateIssueParams{Owner: "o", Repo: "r", IssueNumber: 1, State: "closed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody["state_event"] != "close" {
		t.Errorf("expected state_event 'close', got %v", receivedBody["state_event"])
	}
}

func TestGitLabTokenRequired(t *testing.T) {
	p := newGitLabProviderWithConfig(providerConfig{baseURL: "http://localhost", token: ""})
	_, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "o", Repo: "r"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !contains(err.Error(), "GITLAB_TOKEN") {
		t.Errorf("expected error mentioning GITLAB_TOKEN, got %q", err.Error())
	}
}

// --- Gitea label resolution test ---

func TestGiteaResolveLabelsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "bug"},
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "tok",
		authHeader: "Authorization",
		authValue:  "token tok",
	})

	_, err := p.resolveLabels(context.Background(), "owner", "repo", []string{"bug", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing labels")
	}
	if !contains(err.Error(), "nonexistent") {
		t.Errorf("expected error mentioning missing label, got %q", err.Error())
	}
}

// --- Comment tests ---

func TestGiteaCommentOnIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 99, "body": "test comment", "html_url": "http://localhost/comments/99",
			"created_at": "2024-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "tok",
		authHeader: "Authorization",
		authValue:  "token tok",
	})

	comment, err := p.CommentOnIssue(context.Background(), CommentParams{Owner: "o", Repo: "r", IssueNumber: 1, Body: "test comment"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ID != 99 {
		t.Errorf("expected comment ID 99, got %d", comment.ID)
	}
}

// --- Stub provider for plugin structure tests ---

type stubProvider struct{}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) ListIssues(context.Context, ListIssuesParams) ([]NormalizedIssue, error) {
	return nil, nil
}
func (s *stubProvider) CreateIssue(context.Context, CreateIssueParams) (*NormalizedIssue, error) {
	return nil, nil
}
func (s *stubProvider) UpdateIssue(context.Context, UpdateIssueParams) (*NormalizedIssue, error) {
	return nil, nil
}
func (s *stubProvider) CommentOnIssue(context.Context, CommentParams) (*NormalizedComment, error) {
	return nil, nil
}

// --- Helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
