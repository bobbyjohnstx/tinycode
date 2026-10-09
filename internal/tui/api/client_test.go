package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:8080", "/tmp/project", "")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:8080")
	}
	if c.directory != "/tmp/project" {
		t.Errorf("directory = %q, want %q", c.directory, "/tmp/project")
	}
	if c.http == nil {
		t.Error("http client should not be nil")
	}
	if c.http.Timeout != defaultHTTPTimeout {
		t.Errorf("http Timeout = %v, want %v", c.http.Timeout, defaultHTTPTimeout)
	}
	if c.sseHTTP == nil {
		t.Error("sseHTTP client should not be nil")
	}
	if c.sseHTTP.Timeout != 0 {
		t.Errorf("sseHTTP Timeout = %v, want 0 (no timeout)", c.sseHTTP.Timeout)
	}
}

func TestCreateSession(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":        "ses_abc123",
			"title":     "Test Session",
			"projectID": "prj_test",
			"directory": "/tmp",
			"version":   "1.0",
			"tokens":    map[string]any{},
			"time":      map[string]any{"created": 0, "updated": 0},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp/project", "")
	info, err := c.CreateSession(SessionCreateInput{Title: "Test Session"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session" {
		t.Errorf("path = %q, want /session", gotPath)
	}
	if info.ID != "ses_abc123" {
		t.Errorf("session ID = %q, want %q", info.ID, "ses_abc123")
	}
	if info.Title != "Test Session" {
		t.Errorf("session Title = %q, want %q", info.Title, "Test Session")
	}
}

func TestListSessions(t *testing.T) {
	var gotPath, gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id": "ses_1", "title": "S1", "projectID": "prj_test",
				"directory": "/tmp", "version": "1.0",
				"tokens": map[string]any{}, "time": map[string]any{"created": 0, "updated": 0},
			},
			{
				"id": "ses_2", "title": "S2", "projectID": "prj_test",
				"directory": "/tmp", "version": "1.0",
				"tokens": map[string]any{}, "time": map[string]any{"created": 0, "updated": 0},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp/project", "")
	sessions, err := c.ListSessions(10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/session" {
		t.Errorf("path = %q, want /session", gotPath)
	}
	if gotQuery == "" {
		t.Error("query string should not be empty")
	}
	if len(sessions) != 2 {
		t.Errorf("sessions count = %d, want 2", len(sessions))
	}
}

func TestGetSession(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "ses_xyz", "title": "Got It", "projectID": "prj_test",
			"directory": "/tmp", "version": "1.0",
			"tokens": map[string]any{}, "time": map[string]any{"created": 0, "updated": 0},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	info, err := c.GetSession("ses_xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/session/ses_xyz" {
		t.Errorf("path = %q, want /session/ses_xyz", gotPath)
	}
	if info.ID != "ses_xyz" {
		t.Errorf("session ID = %q, want %q", info.ID, "ses_xyz")
	}
}

func TestDeleteSession(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.DeleteSession("ses_del")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/session/ses_del" {
		t.Errorf("path = %q, want /session/ses_del", gotPath)
	}
}

func TestSendPrompt_204(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.SendPrompt("ses_1", PromptInput{
		Parts: []PromptPart{{Type: "text", Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_1/prompt_async" {
		t.Errorf("path = %q, want /session/ses_1/prompt_async", gotPath)
	}
}

func TestAbortSession(t *testing.T) {
	var gotMethod, gotPath string
	var bodyClosed atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"sessionID": "ses_abort",
			"status":    "aborted",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	base := http.DefaultTransport
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		resp.Body = &closeTrackingBody{ReadCloser: resp.Body, closed: &bodyClosed}
		return resp, nil
	})

	err := c.AbortSession("ses_abort")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_abort/abort" {
		t.Errorf("path = %q, want /session/ses_abort/abort", gotPath)
	}
	if !bodyClosed.Load() {
		t.Error("expected response body to be closed")
	}
}

type closeTrackingBody struct {
	io.ReadCloser
	closed *atomic.Bool
}

func (b *closeTrackingBody) Close() error {
	b.closed.Store(true)
	return b.ReadCloser.Close()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestListProviders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ProviderListResponse{
			All: []ProviderInfo{
				{ID: "ollama", Name: "Ollama", Source: "local"},
			},
			Connected: []string{"ollama"},
			Default:   map[string]string{"model": "llama3"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	resp, err := c.ListProviders()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.All) != 1 {
		t.Errorf("providers count = %d, want 1", len(resp.All))
	}
	if resp.All[0].ID != "ollama" {
		t.Errorf("provider ID = %q, want %q", resp.All[0].ID, "ollama")
	}
}

func TestListAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AgentInfo{
			{Name: "coder", Description: "Code agent", Mode: "tool_use"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	agents, err := c.ListAgents()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "coder" {
		t.Errorf("agents = %v, want [{coder ...}]", agents)
	}
}

func TestListCommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]CommandInfo{
			{Name: "init", Description: "Project setup", Source: "builtin"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	commands, err := c.ListCommands()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commands) != 1 || commands[0].Name != "init" {
		t.Errorf("commands = %v, want [{init ...}]", commands)
	}
}

func TestListMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "msg_1", "role": "user"},
			{"id": "msg_2", "role": "assistant"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	msgs, err := c.ListMessages("ses_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("messages count = %d, want 2", len(msgs))
	}
}

func TestReplyPermission(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.ReplyPermission("ses_1", "perm_42", "once")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_1/permissions/perm_42" {
		t.Errorf("path = %q, want /session/ses_1/permissions/perm_42", gotPath)
	}
	if gotBody["reply"] != "once" {
		t.Errorf("reply = %q, want %q", gotBody["reply"], "once")
	}
	if gotBody["action"] != "once" {
		t.Errorf("action = %q, want %q (backward compat)", gotBody["action"], "once")
	}
}

func TestCreateSession_EncodesDirectory(t *testing.T) {
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "ses_enc", "title": "T", "projectID": "prj",
			"directory": "/tmp/my dir", "version": "1.0",
			"tokens": map[string]any{}, "time": map[string]any{"created": 0, "updated": 0},
		})
	}))
	defer srv.Close()

	dir := "/tmp/my dir & stuff"
	c := New(srv.URL, dir, "")
	if _, err := c.CreateSession(SessionCreateInput{Title: "T"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := gotQuery.Get("directory"); got != dir {
		t.Errorf("directory query = %q, want %q", got, dir)
	}
}

func TestListSessions_EncodesDirectory(t *testing.T) {
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	dir := "/tmp/proj & a/b"
	c := New(srv.URL, dir, "")
	if _, err := c.ListSessions(5, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := gotQuery.Get("directory"); got != dir {
		t.Errorf("directory query = %q, want %q", got, dir)
	}
	if gotQuery.Get("limit") != "5" || gotQuery.Get("offset") != "1" {
		t.Errorf("limit/offset = %q/%q, want 5/1", gotQuery.Get("limit"), gotQuery.Get("offset"))
	}
}

func TestBearerAuthorization(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AgentInfo{})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "secret-token")
	if _, err := c.ListAgents(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
}

func TestForkSessionAtMessage(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":        "ses_fork1",
			"parentID":  "ses_parent",
			"title":     "fork at turn 2",
			"projectID": "prj_test",
			"directory": "/tmp",
			"version":   "1.0",
			"tokens":    map[string]any{},
			"time":      map[string]any{"created": 0, "updated": 0},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp/project", "")
	info, err := c.ForkSessionAtMessage("ses_parent", "msg_42", "fork at turn 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_parent/fork" {
		t.Errorf("path = %q, want /session/ses_parent/fork", gotPath)
	}
	if gotBody["messageID"] != "msg_42" {
		t.Errorf("body messageID = %v, want %q", gotBody["messageID"], "msg_42")
	}
	if gotBody["title"] != "fork at turn 2" {
		t.Errorf("body title = %v, want %q", gotBody["title"], "fork at turn 2")
	}
	if info.ID != "ses_fork1" {
		t.Errorf("session ID = %q, want %q", info.ID, "ses_fork1")
	}
	if info.ParentID != "ses_parent" {
		t.Errorf("session ParentID = %q, want %q", info.ParentID, "ses_parent")
	}
}

func TestSummarizeSession_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"compacted":false}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	if err := c.SummarizeSession("ses_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestErrorResponse_WithJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid input"})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.GetSession("bad")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

func TestErrorResponse_WithoutJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.GetSession("bad")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestContentType_SetForPOST(t *testing.T) {
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	c.SendPrompt("ses_1", PromptInput{Parts: []PromptPart{{Type: "text"}}})

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
}

func TestContentType_NotSetForGET(t *testing.T) {
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AgentInfo{})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	c.ListAgents()

	if gotContentType != "" {
		t.Errorf("Content-Type should be empty for GET, got %q", gotContentType)
	}
}

func TestUpdateSessionTitle_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.UpdateSessionTitle("ses_1", "New Title")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/session/ses_1" {
		t.Errorf("path = %q, want /session/ses_1", gotPath)
	}
	if gotBody["title"] != "New Title" {
		t.Errorf("title = %q, want %q", gotBody["title"], "New Title")
	}
}

func TestUpdateSessionTitle_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.UpdateSessionTitle("ses_1", "Title")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestUpdateSessionAgent_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.UpdateSessionAgent("ses_2", "coder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/session/ses_2" {
		t.Errorf("path = %q, want /session/ses_2", gotPath)
	}
	if gotBody["agent"] != "coder" {
		t.Errorf("agent = %q, want %q", gotBody["agent"], "coder")
	}
}

func TestUpdateSessionAgent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.UpdateSessionAgent("ses_2", "coder")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSetProviderAuth_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	creds := map[string]string{"apiKey": "sk-test-123"}
	err := c.SetProviderAuth("openai", creds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PUT" {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/auth/openai" {
		t.Errorf("path = %q, want /auth/openai", gotPath)
	}
	if gotBody["apiKey"] != "sk-test-123" {
		t.Errorf("apiKey = %q, want %q", gotBody["apiKey"], "sk-test-123")
	}
}

func TestSetProviderAuth_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.SetProviderAuth("openai", map[string]string{"apiKey": "x"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestListAllAgents_IncludesDisabledQueryParam(t *testing.T) {
	var gotPath, gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AgentInfo{
			{Name: "coder", Description: "Code agent", Mode: "tool_use"},
			{Name: "disabled-agent", Description: "Off", Mode: "tool_use", Disabled: true},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	agents, err := c.ListAllAgents()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/agent" {
		t.Errorf("path = %q, want /agent", gotPath)
	}
	if gotQuery != "include=disabled" {
		t.Errorf("query = %q, want include=disabled", gotQuery)
	}
	if len(agents) != 2 {
		t.Errorf("agents count = %d, want 2", len(agents))
	}
}

func TestListAllAgents_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.ListAllAgents()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPatchConfig_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.PatchConfig(map[string]any{"theme": "dark"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/config" {
		t.Errorf("path = %q, want /config", gotPath)
	}
	if gotBody["theme"] != "dark" {
		t.Errorf("theme = %v, want %q", gotBody["theme"], "dark")
	}
}

func TestPatchConfig_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.PatchConfig(map[string]any{"theme": "dark"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetMCPStatus_ReturnsServerStatus(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]map[string]any{
			"filesystem": {"status": "connected", "tools": 5.0},
			"github":     {"status": "error", "error": "timeout"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	status, err := c.GetMCPStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/mcp/status" {
		t.Errorf("path = %q, want /mcp/status", gotPath)
	}
	if len(status) != 2 {
		t.Errorf("status count = %d, want 2", len(status))
	}
	if status["filesystem"]["status"] != "connected" {
		t.Errorf("filesystem status = %v, want connected", status["filesystem"]["status"])
	}
}

func TestGetMCPStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.GetMCPStatus()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetLSPStatus_ReturnsStatus(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LSPStatusResponse{
			Enabled:  true,
			Errors:   2,
			Warnings: 5,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	status, err := c.GetLSPStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/lsp" {
		t.Errorf("path = %q, want /lsp", gotPath)
	}
	if !status.Enabled {
		t.Error("expected Enabled to be true")
	}
	if status.Errors != 2 {
		t.Errorf("Errors = %d, want 2", status.Errors)
	}
	if status.Warnings != 5 {
		t.Errorf("Warnings = %d, want 5", status.Warnings)
	}
}

func TestGetLSPStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.GetLSPStatus()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestReconnectMCP_Success(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.ReconnectMCP("filesystem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/mcp/filesystem/reconnect" {
		t.Errorf("path = %q, want /mcp/filesystem/reconnect", gotPath)
	}
}

func TestReconnectMCP_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.ReconnectMCP("filesystem")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestListPlugins_ReturnsPlugins(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]PluginInfo{
			{ID: "plugin-1", Name: "my-plugin"},
			{ID: "plugin-2", Name: "other-plugin"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	plugins, err := c.ListPlugins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/plugin" {
		t.Errorf("path = %q, want /plugin", gotPath)
	}
	if len(plugins) != 2 {
		t.Errorf("plugins count = %d, want 2", len(plugins))
	}
	if plugins[0].ID != "plugin-1" {
		t.Errorf("plugin[0].ID = %q, want %q", plugins[0].ID, "plugin-1")
	}
	if plugins[1].Name != "other-plugin" {
		t.Errorf("plugin[1].Name = %q, want %q", plugins[1].Name, "other-plugin")
	}
}

func TestListPlugins_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.ListPlugins()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetProviderBalance_ReturnsBalance(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		remaining := 42.5
		usage := 7.5
		json.NewEncoder(w).Encode(BalanceResponse{
			Remaining: &remaining,
			Usage:     &usage,
			Provider:  "anthropic",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	balance, err := c.GetProviderBalance("anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/provider/anthropic/balance" {
		t.Errorf("path = %q, want /provider/anthropic/balance", gotPath)
	}
	if balance.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", balance.Provider, "anthropic")
	}
	if balance.Remaining == nil || *balance.Remaining != 42.5 {
		t.Errorf("Remaining = %v, want 42.5", balance.Remaining)
	}
	if balance.Usage == nil || *balance.Usage != 7.5 {
		t.Errorf("Usage = %v, want 7.5", balance.Usage)
	}
}

func TestGetProviderBalance_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.GetProviderBalance("anthropic")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// TestSessionLifecycleActions_TableDriven covers doNoBody-based session actions:
// ArchiveSession, RevertSession, UnrevertSession.
func TestSessionLifecycleActions_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		callFn     func(c *Client) error
		wantMethod string
		wantPath   string
	}{
		{
			name:       "ArchiveSession sends POST to /session/{id}/archive",
			callFn:     func(c *Client) error { return c.ArchiveSession("ses_arc") },
			wantMethod: "POST",
			wantPath:   "/session/ses_arc/archive",
		},
		{
			name:       "RevertSession sends POST to /session/{id}/revert",
			callFn:     func(c *Client) error { return c.RevertSession("ses_rev") },
			wantMethod: "POST",
			wantPath:   "/session/ses_rev/revert",
		},
		{
			name:       "UnrevertSession sends POST to /session/{id}/unrevert",
			callFn:     func(c *Client) error { return c.UnrevertSession("ses_unrev") },
			wantMethod: "POST",
			wantPath:   "/session/ses_unrev/unrevert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			c := New(srv.URL, "/tmp", "")
			err := tt.callFn(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("method = %q, want %q", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

func TestSessionLifecycleActions_ServerError(t *testing.T) {
	tests := []struct {
		name   string
		callFn func(c *Client) error
	}{
		{"ArchiveSession returns error on 500", func(c *Client) error { return c.ArchiveSession("ses_1") }},
		{"RevertSession returns error on 500", func(c *Client) error { return c.RevertSession("ses_1") }},
		{"UnrevertSession returns error on 500", func(c *Client) error { return c.UnrevertSession("ses_1") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("fail"))
			}))
			defer srv.Close()

			c := New(srv.URL, "/tmp", "")
			err := tt.callFn(c)
			if err == nil {
				t.Fatal("expected error for 500 response")
			}
		})
	}
}

func TestBranchSession_ReturnsNewSession(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":        "ses_branch1",
			"parentID":  "ses_orig",
			"title":     "branched session",
			"projectID": "prj_test",
			"directory": "/tmp",
			"version":   "1.0",
			"tokens":    map[string]any{},
			"time":      map[string]any{"created": 0, "updated": 0},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	info, err := c.BranchSession("ses_orig", "branched session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_orig/fork" {
		t.Errorf("path = %q, want /session/ses_orig/fork", gotPath)
	}
	if gotBody["title"] != "branched session" {
		t.Errorf("body title = %v, want %q", gotBody["title"], "branched session")
	}
	// BranchSession should NOT include messageID in the body
	if _, hasMessageID := gotBody["messageID"]; hasMessageID {
		t.Errorf("body should not contain messageID for BranchSession, got %v", gotBody["messageID"])
	}
	if info.ID != "ses_branch1" {
		t.Errorf("session ID = %q, want %q", info.ID, "ses_branch1")
	}
	if info.ParentID != "ses_orig" {
		t.Errorf("session ParentID = %q, want %q", info.ParentID, "ses_orig")
	}
}

func TestBranchSession_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.BranchSession("ses_1", "branch")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRewindSession_SendsMessageID(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.RewindSession("ses_rw", "msg_99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_rw/rewind" {
		t.Errorf("path = %q, want /session/ses_rw/rewind", gotPath)
	}
	if gotBody["messageID"] != "msg_99" {
		t.Errorf("messageID = %q, want %q", gotBody["messageID"], "msg_99")
	}
}

func TestRewindSession_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	err := c.RewindSession("ses_1", "msg_1")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestBtw_ReturnsAnswer(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"answer": "42"})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	answer, err := c.Btw("ses_btw", "what is the meaning of life?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_btw/btw" {
		t.Errorf("path = %q, want /session/ses_btw/btw", gotPath)
	}
	if gotBody["question"] != "what is the meaning of life?" {
		t.Errorf("question = %q, want %q", gotBody["question"], "what is the meaning of life?")
	}
	if answer != "42" {
		t.Errorf("answer = %q, want %q", answer, "42")
	}
}

func TestBtw_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	_, err := c.Btw("ses_1", "question")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestUpdateSessionTitle_ClosesResponseBody(t *testing.T) {
	var bodyClosed atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	base := http.DefaultTransport
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		resp.Body = &closeTrackingBody{ReadCloser: resp.Body, closed: &bodyClosed}
		return resp, nil
	})

	err := c.UpdateSessionTitle("ses_1", "Title")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bodyClosed.Load() {
		t.Error("expected response body to be closed")
	}
}

func TestPatchConfig_ClosesResponseBody(t *testing.T) {
	var bodyClosed atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	base := http.DefaultTransport
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		resp.Body = &closeTrackingBody{ReadCloser: resp.Body, closed: &bodyClosed}
		return resp, nil
	})

	err := c.PatchConfig(map[string]any{"theme": "dark"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bodyClosed.Load() {
		t.Error("expected response body to be closed")
	}
}

func TestSetProviderAuth_ClosesResponseBody(t *testing.T) {
	var bodyClosed atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	base := http.DefaultTransport
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		resp.Body = &closeTrackingBody{ReadCloser: resp.Body, closed: &bodyClosed}
		return resp, nil
	})

	err := c.SetProviderAuth("openai", map[string]string{"apiKey": "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bodyClosed.Load() {
		t.Error("expected response body to be closed")
	}
}

func TestGetProviderBalance_NilValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"remaining":null,"usage":null,"provider":"anthropic"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp", "")
	balance, err := c.GetProviderBalance("anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balance.Remaining != nil {
		t.Errorf("Remaining should be nil, got %v", *balance.Remaining)
	}
	if balance.Usage != nil {
		t.Errorf("Usage should be nil, got %v", *balance.Usage)
	}
	if balance.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", balance.Provider, "anthropic")
	}
}
