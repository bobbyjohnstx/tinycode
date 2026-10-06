package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"

	_ "modernc.org/sqlite"
)

func TestVCSStatus_ReturnsArray(t *testing.T) {
	srv, _ := testServer(t)

	// Use a directory that is a git repo so we get real results.
	// The test repo root itself is a git repo.
	req := httptest.NewRequest("GET", "/vcs/status", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("response should decode as object: %v (body: %s)", err, w.Body.String())
	}

	// Should be an object with "clean" and "changes" fields.
	if _, ok := result["clean"]; !ok {
		t.Fatal("expected 'clean' field in response")
	}
	if _, ok := result["changes"]; !ok {
		t.Fatal("expected 'changes' field in response")
	}
}

func TestVCSStatus_EmptyReturnsEmptyArray(t *testing.T) {
	// Use a non-git directory to trigger the error path.
	dir := t.TempDir() // not a git repo
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/vcs/status", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("expected JSON object: %v", err)
	}
	// Non-git must not look like a clean repo: error present, clean=false.
	if result["clean"] != false {
		t.Errorf("expected clean=false for non-git dir, got %v", result["clean"])
	}
	if _, ok := result["error"]; !ok {
		t.Fatal("expected 'error' field for non-git directory")
	}
	changes, _ := result["changes"].([]any)
	if len(changes) != 0 {
		t.Fatalf("expected empty changes, got %v", changes)
	}
}

func TestVCSStatus_HasExpectedFields(t *testing.T) {
	srv, _ := testServer(t)

	// Point at the actual repo, which should have some changes.
	req := httptest.NewRequest("GET", "/vcs/status", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("response should be object: %v", err)
	}

	// Top-level fields
	if _, ok := result["clean"]; !ok {
		t.Error("missing 'clean' field")
	}
	changesRaw, ok := result["changes"]
	if !ok {
		t.Fatal("missing 'changes' field")
	}

	changes, ok := changesRaw.([]any)
	if !ok || len(changes) == 0 {
		t.Skip("no git changes to validate field shape")
	}

	item, _ := changes[0].(map[string]any)
	for _, field := range []string{"file", "status"} {
		if _, ok := item[field]; !ok {
			t.Errorf("missing expected field %q in VCS status change item", field)
		}
	}
}

func TestFileStatus_EmptyReturnsEmptyArray(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/file/status", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("expected JSON object: %v", err)
	}
	if result["clean"] != false {
		t.Errorf("expected clean=false for non-git dir, got %v", result["clean"])
	}
	if _, ok := result["error"]; !ok {
		t.Fatal("expected 'error' field for non-git directory")
	}
	changes, _ := result["changes"].([]any)
	if len(changes) != 0 {
		t.Fatalf("expected empty changes, got %v", changes)
	}
}

func TestSessionTodo_ReturnsArrayNotObject(t *testing.T) {
	srv, _ := testServer(t)

	// Create a session with a message containing a TODO.
	body := `{"title": "Todo Test", "agent": "build"}`
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp/todo-test", strings.NewReader(body))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d: %s", createW.Code, createW.Body.String())
	}

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Insert a message with a TODO line.
	ms := srv.messageStore()
	err := ms.Append(&session.Message{
		ID:        "msg_test_todo_1",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts: []session.Part{
			session.TextPart("Here is a TODO: fix the tests"),
		},
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("appending message: %v", err)
	}

	// GET /session/{id}/todo
	req := httptest.NewRequest("GET", "/session/"+sessionID+"/todo", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Response is {"todos": [...]}
	var envelope map[string]any
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	todosRaw, ok := envelope["todos"]
	if !ok {
		t.Fatal("expected 'todos' field in response")
	}
	todos, ok := todosRaw.([]any)
	if !ok || len(todos) == 0 {
		t.Fatal("expected at least one todo item")
	}

	item, _ := todos[0].(map[string]any)

	// Must have "text" and "messageID" fields
	if _, ok := item["text"]; !ok {
		t.Error("expected 'text' field in todo item")
	}
	if _, ok := item["messageID"]; !ok {
		t.Error("expected 'messageID' field in todo item")
	}
	if _, ok := item["line"]; !ok {
		t.Error("expected 'line' field in todo item")
	}
}

func TestSessionTodo_EmptyReturnsEmptyArray(t *testing.T) {
	srv, _ := testServer(t)

	// Create a session with no messages.
	body := `{"title": "Empty Todo", "agent": "build"}`
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp/todo-empty", strings.NewReader(body))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d: %s", createW.Code, createW.Body.String())
	}

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	req := httptest.NewRequest("GET", "/session/"+sessionID+"/todo", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var envelope map[string]any
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("expected JSON object: %v", err)
	}
	todos, _ := envelope["todos"].([]any)
	if len(todos) != 0 {
		t.Fatalf("expected empty todos, got %v", todos)
	}
}
