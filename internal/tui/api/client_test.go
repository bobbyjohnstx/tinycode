package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:8080", "/tmp/project")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:8080")
	}
	if c.directory != "/tmp/project" {
		t.Errorf("directory = %q, want %q", c.directory, "/tmp/project")
	}
	if c.http == nil {
		t.Error("http client should not be nil")
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

	c := New(srv.URL, "/tmp/project")
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

	c := New(srv.URL, "/tmp/project")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
	err := c.ReplyPermission("ses_1", "perm_42", "allow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/session/ses_1/permissions/perm_42" {
		t.Errorf("path = %q, want /session/ses_1/permissions/perm_42", gotPath)
	}
	if gotBody["action"] != "allow" {
		t.Errorf("action = %q, want %q", gotBody["action"], "allow")
	}
}

func TestErrorResponse_WithJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid input"})
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
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

	c := New(srv.URL, "/tmp")
	c.ListAgents()

	if gotContentType != "" {
		t.Errorf("Content-Type should be empty for GET, got %q", gotContentType)
	}
}
