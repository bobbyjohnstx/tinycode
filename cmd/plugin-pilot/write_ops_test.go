package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- GitHub write operation tests ---

func TestGitHubCreateIssue_SendsPOSTAndReturnsCreatedIssue(t *testing.T) {
	var receivedBody map[string]any
	var receivedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 7, "title": "New Feature", "body": "Feature body", "state": "open",
			"html_url": "https://github.com/owner/repo/issues/7", "created_at": "2024-05-01T00:00:00Z",
			"updated_at": "2024-05-01T00:00:00Z", "labels": []map[string]any{{"name": "enhancement"}},
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

	issue, err := p.CreateIssue(context.Background(), CreateIssueParams{
		Owner: "owner", Repo: "repo", Title: "New Feature", Body: "Feature body", Labels: []string{"enhancement"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedBody["title"] != "New Feature" {
		t.Errorf("expected title 'New Feature', got %v", receivedBody["title"])
	}
	if receivedBody["body"] != "Feature body" {
		t.Errorf("expected body in request, got %v", receivedBody["body"])
	}
	if issue.Number != 7 {
		t.Errorf("expected issue number 7, got %d", issue.Number)
	}
	if issue.URL != "https://github.com/owner/repo/issues/7" {
		t.Errorf("expected URL, got %q", issue.URL)
	}
}

func TestGitHubUpdateIssue_SendsPATCHWithCorrectFields(t *testing.T) {
	var receivedBody map[string]any
	var receivedMethod, receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 3, "title": "Updated Title", "body": "Updated body", "state": "closed",
			"html_url": "https://github.com/owner/repo/issues/3", "created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-06-01T00:00:00Z", "labels": []any{},
		})
	}))
	defer srv.Close()

	p := newGitHubProviderWithConfig(providerConfig{
		baseURL:    srv.URL,
		token:      "gh-token",
		authHeader: "Authorization",
		authValue:  "Bearer gh-token",
	})

	issue, err := p.UpdateIssue(context.Background(), UpdateIssueParams{
		Owner: "owner", Repo: "repo", IssueNumber: 3, Title: "Updated Title", State: "closed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", receivedMethod)
	}
	if receivedPath != "/repos/owner/repo/issues/3" {
		t.Errorf("expected path /repos/owner/repo/issues/3, got %s", receivedPath)
	}
	if receivedBody["title"] != "Updated Title" {
		t.Errorf("expected title in body, got %v", receivedBody["title"])
	}
	if receivedBody["state"] != "closed" {
		t.Errorf("expected state=closed in body, got %v", receivedBody["state"])
	}
	if issue.Number != 3 {
		t.Errorf("expected issue 3, got %d", issue.Number)
	}
}

func TestGitHubCommentOnIssue_SendsPOSTToCommentsEndpoint(t *testing.T) {
	var receivedBody map[string]any
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 200, "body": "Looks good!", "html_url": "https://github.com/owner/repo/issues/5#issuecomment-200",
			"created_at": "2024-06-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	p := newGitHubProviderWithConfig(providerConfig{
		baseURL:    srv.URL,
		token:      "gh-token",
		authHeader: "Authorization",
		authValue:  "Bearer gh-token",
	})

	comment, err := p.CommentOnIssue(context.Background(), CommentParams{
		Owner: "owner", Repo: "repo", IssueNumber: 5, Body: "Looks good!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedPath != "/repos/owner/repo/issues/5/comments" {
		t.Errorf("expected comments endpoint, got %s", receivedPath)
	}
	if receivedBody["body"] != "Looks good!" {
		t.Errorf("expected comment body in request, got %v", receivedBody["body"])
	}
	if comment.ID != 200 {
		t.Errorf("expected comment ID 200, got %d", comment.ID)
	}
	if comment.Body != "Looks good!" {
		t.Errorf("expected comment body, got %q", comment.Body)
	}
}

// --- GitLab write operation tests ---

func TestGitLabCreateIssue_UsesDescriptionField(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iid": 10, "title": "GL Feature", "description": "Feature desc", "state": "opened",
			"web_url": "https://gitlab.com/owner/repo/-/issues/10", "created_at": "2024-07-01T00:00:00Z",
			"updated_at": "2024-07-01T00:00:00Z", "labels": []string{"feature"},
		})
	}))
	defer srv.Close()

	p := newGitLabProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v4",
		token:      "gl-tok",
		authHeader: "PRIVATE-TOKEN",
		authValue:  "gl-tok",
	})

	issue, err := p.CreateIssue(context.Background(), CreateIssueParams{
		Owner: "owner", Repo: "repo", Title: "GL Feature", Body: "Feature desc", Labels: []string{"feature", "backend"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// GitLab uses "description" not "body"
	if receivedBody["description"] != "Feature desc" {
		t.Errorf("expected 'description' field in request, got body keys: %v", receivedBody)
	}
	if receivedBody["body"] != nil {
		t.Errorf("GitLab should not send 'body', it should use 'description'")
	}
	// GitLab labels are sent as comma-separated string
	if labels, ok := receivedBody["labels"].(string); !ok || labels != "feature,backend" {
		t.Errorf("expected labels as comma-separated string, got %v", receivedBody["labels"])
	}
	if issue.Number != 10 {
		t.Errorf("expected iid 10, got %d", issue.Number)
	}
	if issue.State != "open" {
		t.Errorf("expected normalized state 'open', got %q", issue.State)
	}
}

func TestGitLabCommentOnIssue_SendsPOSTToNotesEndpoint(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 50, "body": "GL comment", "noteable_iid": 3, "created_at": "2024-07-15T00:00:00Z",
		})
	}))
	defer srv.Close()

	p := newGitLabProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v4",
		token:      "gl-tok",
		authHeader: "PRIVATE-TOKEN",
		authValue:  "gl-tok",
	})

	comment, err := p.CommentOnIssue(context.Background(), CommentParams{
		Owner: "owner", Repo: "repo", IssueNumber: 3, Body: "GL comment",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// GitLab uses /notes instead of /comments
	if !contains(receivedPath, "/notes") {
		t.Errorf("expected notes endpoint, got %s", receivedPath)
	}
	if receivedBody["body"] != "GL comment" {
		t.Errorf("expected comment body in request, got %v", receivedBody["body"])
	}
	if comment.ID != 50 {
		t.Errorf("expected comment ID 50, got %d", comment.ID)
	}
	// GitLab comment URL should be constructed from noteable_iid
	if comment.URL == "" {
		t.Error("expected non-empty URL constructed from noteable_iid")
	}
}

// --- Gitea write operation tests ---

func TestGiteaUpdateIssue_SendsPATCHWithCorrectFields(t *testing.T) {
	var receivedBody map[string]any
	var receivedMethod, receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 5, "title": "Updated", "body": "New body", "state": "closed",
			"html_url": "http://localhost/issues/5", "created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-02-01T00:00:00Z", "labels": []any{},
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "tok",
		authHeader: "Authorization",
		authValue:  "token tok",
	})

	issue, err := p.UpdateIssue(context.Background(), UpdateIssueParams{
		Owner: "owner", Repo: "repo", IssueNumber: 5, Title: "Updated", Body: "New body", State: "closed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", receivedMethod)
	}
	if receivedPath != "/api/v1/repos/owner/repo/issues/5" {
		t.Errorf("expected correct issue path, got %s", receivedPath)
	}
	if receivedBody["title"] != "Updated" {
		t.Errorf("expected title in body, got %v", receivedBody["title"])
	}
	if receivedBody["state"] != "closed" {
		t.Errorf("expected state in body, got %v", receivedBody["state"])
	}
	if issue.Number != 5 {
		t.Errorf("expected issue 5, got %d", issue.Number)
	}
}

func TestGiteaResolveLabels_ReturnsCorrectIDsForExistingLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "bug"},
			{"id": 2, "name": "feature"},
			{"id": 3, "name": "docs"},
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "tok",
		authHeader: "Authorization",
		authValue:  "token tok",
	})

	ids, err := p.resolveLabels(context.Background(), "owner", "repo", []string{"bug", "docs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	idSet := map[int]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet[1] || !idSet[3] {
		t.Errorf("expected IDs [1, 3], got %v", ids)
	}
}

func TestGiteaCreateIssue_WithLabelsResolvesAndIncludesIDs(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && contains(r.URL.Path, "/labels") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 10, "name": "bug"},
				{"id": 20, "name": "urgent"},
			})
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 50, "title": "Bug report", "state": "open",
			"html_url": "http://localhost/issues/50", "created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-01T00:00:00Z", "labels": []map[string]any{{"name": "bug"}, {"name": "urgent"}},
		})
	}))
	defer srv.Close()

	p := newGiteaProviderWithConfig(providerConfig{
		baseURL:    srv.URL + "/api/v1",
		token:      "tok",
		authHeader: "Authorization",
		authValue:  "token tok",
	})

	issue, err := p.CreateIssue(context.Background(), CreateIssueParams{
		Owner: "owner", Repo: "repo", Title: "Bug report", Labels: []string{"bug", "urgent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Number != 50 {
		t.Errorf("expected issue 50, got %d", issue.Number)
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls (labels + create), got %d", callCount)
	}
}

// --- Error handling tests ---

func TestGitHubHandleStatus_ReturnsDescriptiveErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantMsg    string
	}{
		{name: "unauthorized returns auth error", statusCode: 401, wantMsg: "Authentication failed"},
		{name: "forbidden returns auth error", statusCode: 403, wantMsg: "Authentication failed"},
		{name: "not found returns not found error", statusCode: 404, wantMsg: "not found"},
		{name: "server error returns status code", statusCode: 500, wantMsg: "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			p := newGitHubProviderWithConfig(providerConfig{
				baseURL:    srv.URL,
				token:      "tok",
				authHeader: "Authorization",
				authValue:  "Bearer tok",
			})
			_, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "o", Repo: "r", State: "open"})
			if err == nil {
				t.Fatal("expected error")
			}
			pe, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected *ProviderError, got %T", err)
			}
			if pe.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, pe.StatusCode)
			}
			if !contains(pe.Error(), tt.wantMsg) {
				t.Errorf("expected error containing %q, got %q", tt.wantMsg, pe.Error())
			}
			if pe.Provider != "github" {
				t.Errorf("expected provider 'github', got %q", pe.Provider)
			}
		})
	}
}

func TestGitLabHandleStatus_ReturnsDescriptiveErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantMsg    string
	}{
		{name: "unauthorized returns auth error", statusCode: 401, wantMsg: "Authentication failed"},
		{name: "forbidden returns auth error", statusCode: 403, wantMsg: "Authentication failed"},
		{name: "not found returns not found error", statusCode: 404, wantMsg: "not found"},
		{name: "server error returns status code", statusCode: 500, wantMsg: "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			p := newGitLabProviderWithConfig(providerConfig{
				baseURL:    srv.URL + "/api/v4",
				token:      "tok",
				authHeader: "PRIVATE-TOKEN",
				authValue:  "tok",
			})
			_, err := p.ListIssues(context.Background(), ListIssuesParams{Owner: "o", Repo: "r", State: "open"})
			if err == nil {
				t.Fatal("expected error")
			}
			pe, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected *ProviderError, got %T", err)
			}
			if pe.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, pe.StatusCode)
			}
			if !contains(pe.Error(), tt.wantMsg) {
				t.Errorf("expected error containing %q, got %q", tt.wantMsg, pe.Error())
			}
			if pe.Provider != "gitlab" {
				t.Errorf("expected provider 'gitlab', got %q", pe.Provider)
			}
		})
	}
}

func TestGiteaHandleStatus_AuthAndServerErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantMsg    string
	}{
		{name: "unauthorized returns auth error", statusCode: 401, wantMsg: "Authentication failed"},
		{name: "forbidden returns auth error", statusCode: 403, wantMsg: "Authentication failed"},
		{name: "server error returns status code", statusCode: 500, wantMsg: "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{}`))
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
				t.Fatal("expected error")
			}
			pe, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected *ProviderError, got %T", err)
			}
			if pe.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, pe.StatusCode)
			}
			if !contains(pe.Error(), tt.wantMsg) {
				t.Errorf("expected error containing %q, got %q", tt.wantMsg, pe.Error())
			}
		})
	}
}

// --- Token-required tests for write operations ---

func TestGitHubWriteOps_RequireToken(t *testing.T) {
	p := newGitHubProviderWithConfig(providerConfig{baseURL: "http://localhost", token: ""})

	_, err := p.CreateIssue(context.Background(), CreateIssueParams{Owner: "o", Repo: "r", Title: "t"})
	if err == nil || !contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("CreateIssue: expected token error, got %v", err)
	}

	_, err = p.UpdateIssue(context.Background(), UpdateIssueParams{Owner: "o", Repo: "r", IssueNumber: 1})
	if err == nil || !contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("UpdateIssue: expected token error, got %v", err)
	}

	_, err = p.CommentOnIssue(context.Background(), CommentParams{Owner: "o", Repo: "r", IssueNumber: 1, Body: "b"})
	if err == nil || !contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("CommentOnIssue: expected token error, got %v", err)
	}
}

func TestGitLabWriteOps_RequireToken(t *testing.T) {
	p := newGitLabProviderWithConfig(providerConfig{baseURL: "http://localhost", token: ""})

	_, err := p.CreateIssue(context.Background(), CreateIssueParams{Owner: "o", Repo: "r", Title: "t"})
	if err == nil || !contains(err.Error(), "GITLAB_TOKEN") {
		t.Errorf("CreateIssue: expected token error, got %v", err)
	}

	_, err = p.UpdateIssue(context.Background(), UpdateIssueParams{Owner: "o", Repo: "r", IssueNumber: 1})
	if err == nil || !contains(err.Error(), "GITLAB_TOKEN") {
		t.Errorf("UpdateIssue: expected token error, got %v", err)
	}

	_, err = p.CommentOnIssue(context.Background(), CommentParams{Owner: "o", Repo: "r", IssueNumber: 1, Body: "b"})
	if err == nil || !contains(err.Error(), "GITLAB_TOKEN") {
		t.Errorf("CommentOnIssue: expected token error, got %v", err)
	}
}

func TestGiteaWriteOps_RequireToken(t *testing.T) {
	p := newGiteaProviderWithConfig(providerConfig{baseURL: "http://localhost", token: ""})

	_, err := p.CreateIssue(context.Background(), CreateIssueParams{Owner: "o", Repo: "r", Title: "t"})
	if err == nil || !contains(err.Error(), "GITEA_TOKEN") {
		t.Errorf("CreateIssue: expected token error, got %v", err)
	}

	_, err = p.UpdateIssue(context.Background(), UpdateIssueParams{Owner: "o", Repo: "r", IssueNumber: 1})
	if err == nil || !contains(err.Error(), "GITEA_TOKEN") {
		t.Errorf("UpdateIssue: expected token error, got %v", err)
	}

	_, err = p.CommentOnIssue(context.Background(), CommentParams{Owner: "o", Repo: "r", IssueNumber: 1, Body: "b"})
	if err == nil || !contains(err.Error(), "GITEA_TOKEN") {
		t.Errorf("CommentOnIssue: expected token error, got %v", err)
	}
}
