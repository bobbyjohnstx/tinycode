package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
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

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("response should decode as array: %v (body: %s)", err, w.Body.String())
	}

	// Should be an array (possibly empty), not an object with "clean"/"changes".
	// Verify it did not wrap in an object.
	raw := w.Body.Bytes()
	if len(raw) > 0 && raw[0] == '{' {
		t.Fatal("response is an object, expected an array")
	}
}

func TestVCSStatus_EmptyReturnsEmptyArray(t *testing.T) {
	// Use a non-git directory to trigger the error path.
	srv, _ := testServer(t)
	dir := t.TempDir() // not a git repo

	req := httptest.NewRequest("GET", "/vcs/status?directory="+dir, nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Fatalf("expected [], got %s", body)
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

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("response should be array: %v", err)
	}

	if len(result) == 0 {
		t.Skip("no git changes to validate field shape")
	}

	item := result[0]
	for _, field := range []string{"file", "status", "additions", "deletions"} {
		if _, ok := item[field]; !ok {
			t.Errorf("missing expected field %q in VCS status item", field)
		}
	}

	// Verify old fields are not present
	for _, field := range []string{"clean", "changes"} {
		if _, ok := item[field]; ok {
			t.Errorf("old field %q should not be present in VCS status items", field)
		}
	}
}

func TestFileStatus_EmptyReturnsEmptyArray(t *testing.T) {
	srv, _ := testServer(t)
	dir := t.TempDir()

	req := httptest.NewRequest("GET", "/file/status?directory="+dir, nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Fatalf("expected [], got %s", body)
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

	// Must be a JSON array, not {"todos": [...]}
	raw := strings.TrimSpace(w.Body.String())
	if raw[0] != '[' {
		t.Fatalf("expected array, got: %s", raw)
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("failed to decode array: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("expected at least one todo item")
	}

	item := result[0]

	// Must have "content" (not "text")
	if _, ok := item["content"]; !ok {
		t.Error("expected 'content' field in todo item")
	}
	if _, ok := item["text"]; ok {
		t.Error("'text' field should not be present (use 'content' instead)")
	}

	// Must have status and priority
	if item["status"] != "open" {
		t.Errorf("expected status 'open', got %v", item["status"])
	}
	if item["priority"] != "normal" {
		t.Errorf("expected priority 'normal', got %v", item["priority"])
	}

	// Must have id
	if _, ok := item["id"]; !ok {
		t.Error("expected 'id' field in todo item")
	}

	// Must NOT have old fields
	for _, field := range []string{"messageID", "line"} {
		if _, ok := item[field]; ok {
			t.Errorf("old field %q should not be present in todo items", field)
		}
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

	body2 := strings.TrimSpace(w.Body.String())
	if body2 != "[]" {
		t.Fatalf("expected [], got %s", body2)
	}
}
