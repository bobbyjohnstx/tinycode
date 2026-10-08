package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// --- handleSessionStatus ---

func TestHandleSessionStatus_ReturnsStatusMap(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/session/status", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The response should be a valid JSON object (map of session statuses).
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("expected valid JSON map, got error: %v", err)
	}
}

// --- handleSessionInit ---

func TestHandleSessionInit_ReturnsSessionInfo(t *testing.T) {
	srv, _ := testServer(t)

	// Create a session first.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Init Test"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Init the session.
	req := httptest.NewRequest("POST", "/session/"+sessionID+"/init", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var info map[string]any
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info["id"] != sessionID {
		t.Errorf("expected id %q, got %v", sessionID, info["id"])
	}
}

func TestHandleSessionInit_PublishesInitializedEvent(t *testing.T) {
	srv, b := testServer(t)

	// Create session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Init Event"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	sub := b.Subscribe("session.initialized")
	defer sub.Unsubscribe()

	req := httptest.NewRequest("POST", "/session/"+sessionID+"/init", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != sessionID {
			t.Errorf("expected sessionID %q, got %v", sessionID, props["sessionID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.initialized event")
	}
}

func TestHandleSessionInit_NotFoundReturns404(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_nonexistent/init", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- handleSessionCommand ---

func TestHandleSessionCommand_ReturnsAccepted(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_cmd_1/command",
		strings.NewReader(`{"command":"compact","args":"--force"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["sessionID"] != "ses_cmd_1" {
		t.Errorf("expected sessionID ses_cmd_1, got %v", result["sessionID"])
	}
	if result["command"] != "compact" {
		t.Errorf("expected command compact, got %v", result["command"])
	}
}

func TestHandleSessionCommand_PublishesCommandEvent(t *testing.T) {
	srv, b := testServer(t)

	sub := b.Subscribe("session.command")
	defer sub.Unsubscribe()

	req := httptest.NewRequest("POST", "/session/ses_cmd_2/command",
		strings.NewReader(`{"command":"help","args":"search"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_cmd_2" {
			t.Errorf("expected sessionID ses_cmd_2, got %v", props["sessionID"])
		}
		if props["command"] != "help" {
			t.Errorf("expected command help, got %v", props["command"])
		}
		if props["args"] != "search" {
			t.Errorf("expected args search, got %v", props["args"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.command event")
	}
}

func TestHandleSessionCommand_InvalidBodyReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_cmd_3/command",
		strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- handleSessionRewindMessages ---

func TestHandleSessionRewindMessages_DeletesMessagesAfterTarget(t *testing.T) {
	srv, _ := testServer(t)

	// Create session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Rewind"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Add messages.
	ms := srv.messageStore()
	msgIDs := make([]string, 4)
	for i := 0; i < 4; i++ {
		msgIDs[i] = fmt.Sprintf("msg_rewind_%d", i)
		msg := &session.Message{
			ID:        msgIDs[i],
			SessionID: sessionID,
			Role:      session.RoleUser,
			Parts:     []session.Part{session.TextPart(fmt.Sprintf("message %d", i))},
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := ms.Append(msg); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	// Rewind to message 1 (should delete messages 2 and 3).
	body := fmt.Sprintf(`{"messageID":"%s"}`, msgIDs[1])
	req := httptest.NewRequest("POST", "/session/"+sessionID+"/rewind", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)

	deletedCount, _ := result["deletedCount"].(float64)
	if deletedCount != 2 {
		t.Errorf("expected 2 deleted messages, got %v", deletedCount)
	}
	if result["rewindToMessage"] != msgIDs[1] {
		t.Errorf("expected rewindToMessage %q, got %v", msgIDs[1], result["rewindToMessage"])
	}

	// Verify remaining messages.
	remaining, err := ms.List(sessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining messages, got %d", len(remaining))
	}
}

func TestHandleSessionRewindMessages_MissingMessageIDReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_rewind/rewind",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing messageID, got %d", w.Code)
	}
}

func TestHandleSessionRewindMessages_EmptyMessageIDReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_rewind/rewind",
		strings.NewReader(`{"messageID":""}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty messageID, got %d", w.Code)
	}
}

func TestHandleSessionRewindMessages_InvalidBodyReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_rewind/rewind",
		strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}

func TestHandleSessionRewindMessages_NonexistentMessageReturns404(t *testing.T) {
	srv, _ := testServer(t)

	// Create session (so the session exists).
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Rewind 404"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	req := httptest.NewRequest("POST", "/session/"+sessionID+"/rewind",
		strings.NewReader(`{"messageID":"msg_does_not_exist"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent message, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleSessionChildren ---

func TestHandleSessionChildren_ReturnsEmptyArrayWhenNoChildren(t *testing.T) {
	srv, _ := testServer(t)

	// Create a session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Parent No Kids"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	req := httptest.NewRequest("GET", "/session/"+sessionID+"/children", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Must return [] not null.
	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty JSON array '[]', got %q", body)
	}
}

func TestHandleSessionChildren_ReturnsForkedSessions(t *testing.T) {
	srv, _ := testServer(t)

	// Create parent.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Parent With Kids"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var parent map[string]any
	json.NewDecoder(createW.Body).Decode(&parent)
	parentID := parent["id"].(string)

	// Fork the session twice.
	for _, title := range []string{"Child 1", "Child 2"} {
		forkReq := httptest.NewRequest("POST", "/session/"+parentID+"/fork",
			strings.NewReader(fmt.Sprintf(`{"title":"%s"}`, title)))
		forkW := httptest.NewRecorder()
		srv.mux.ServeHTTP(forkW, forkReq)
		if forkW.Code != http.StatusCreated {
			t.Fatalf("fork: expected 201, got %d", forkW.Code)
		}
	}

	// Get children.
	req := httptest.NewRequest("GET", "/session/"+parentID+"/children", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var children []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&children); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}
}

// --- handleSessionTodo ---

func TestHandleSessionTodo_ReturnsEmptyTodosWhenNoMessages(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/session/ses_no_messages/todo", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	todos, _ := result["todos"].([]any)
	if len(todos) != 0 {
		t.Errorf("expected 0 todos, got %d", len(todos))
	}
}

func TestHandleSessionTodo_FindsTodoAndFixmeInMessages(t *testing.T) {
	srv, _ := testServer(t)

	// Create session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Todo Test"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Add messages with TODO and FIXME markers.
	ms := srv.messageStore()
	msg := &session.Message{
		ID:        "msg_todo_1",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts: []session.Part{
			session.TextPart("Here is some work:\nTODO: add error handling\nsome other text\nFIXME: memory leak in loop"),
		},
		CreatedAt: time.Now(),
	}
	if err := ms.Append(msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	req := httptest.NewRequest("GET", "/session/"+sessionID+"/todo", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	todos, _ := result["todos"].([]any)
	if len(todos) != 2 {
		t.Fatalf("expected 2 todos, got %d: %v", len(todos), result)
	}

	// Verify first todo.
	first := todos[0].(map[string]any)
	if first["messageID"] != "msg_todo_1" {
		t.Errorf("expected messageID msg_todo_1, got %v", first["messageID"])
	}
	text := first["text"].(string)
	if !strings.Contains(text, "TODO") {
		t.Errorf("expected text to contain TODO, got %q", text)
	}
}

// --- handleSessionDiff ---

func TestHandleSessionDiff_ReturnsStructuredResponse(t *testing.T) {
	srv, _ := testServer(t)

	// The handler runs against the server's directory. We test the response shape
	// rather than actual diff content (the directory may not be a git repo).
	req := httptest.NewRequest("GET", "/session/ses_nonexistent/diff", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Verify response has expected fields.
	if _, ok := result["diff"]; !ok {
		t.Error("expected 'diff' field in response")
	}
	if _, ok := result["files"]; !ok {
		t.Error("expected 'files' field in response")
	}
	if summary, ok := result["summary"].(map[string]any); !ok {
		t.Error("expected 'summary' field as object in response")
	} else {
		if _, ok := summary["additions"]; !ok {
			t.Error("expected summary.additions")
		}
		if _, ok := summary["deletions"]; !ok {
			t.Error("expected summary.deletions")
		}
		if _, ok := summary["files"]; !ok {
			t.Error("expected summary.files")
		}
	}
}

// --- handleSessionArchive / handleSessionUnarchive ---

func TestHandleSessionArchive_SetsArchivedTimestamp(t *testing.T) {
	srv, _ := testServer(t)

	// Create session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Archive Me"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Archive.
	req := httptest.NewRequest("POST", "/session/"+sessionID+"/archive", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("archive: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify session is archived via the store.
	store := srv.sessionStore()
	info, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if info.TimeArchived == 0 {
		t.Error("expected TimeArchived to be set after archive")
	}
}

func TestHandleSessionArchive_PublishesUpdatedEvent(t *testing.T) {
	srv, b := testServer(t)

	// Create session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Archive Event"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	sub := b.Subscribe("session.updated")
	defer sub.Unsubscribe()

	req := httptest.NewRequest("POST", "/session/"+sessionID+"/archive", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != sessionID {
			t.Errorf("expected sessionID %q, got %v", sessionID, props["sessionID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.updated event")
	}
}

func TestHandleSessionUnarchive_ClearsArchivedTimestamp(t *testing.T) {
	srv, _ := testServer(t)

	// Create and archive session.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Unarchive Me"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Archive first.
	archiveReq := httptest.NewRequest("POST", "/session/"+sessionID+"/archive", nil)
	archiveW := httptest.NewRecorder()
	srv.mux.ServeHTTP(archiveW, archiveReq)
	if archiveW.Code != http.StatusNoContent {
		t.Fatalf("archive: expected 204, got %d", archiveW.Code)
	}

	// Unarchive.
	req := httptest.NewRequest("POST", "/session/"+sessionID+"/unarchive", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unarchive: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify session is no longer archived.
	store := srv.sessionStore()
	info, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if info.TimeArchived != 0 {
		t.Error("expected TimeArchived to be 0 after unarchive")
	}
}

// --- handleSessionPermissionReply ---

func TestHandleSessionPermissionReply_MapsOnceAlwaysReject(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantReply string
	}{
		{
			name:      "action once maps to once",
			body:      `{"action":"once"}`,
			wantReply: "once",
		},
		{
			name:      "action always maps to always",
			body:      `{"action":"always"}`,
			wantReply: "always",
		},
		{
			name:      "action reject maps to reject",
			body:      `{"action":"reject"}`,
			wantReply: "reject",
		},
		{
			name:      "action allow normalizes to once",
			body:      `{"action":"allow"}`,
			wantReply: "once",
		},
		{
			name:      "action deny normalizes to reject",
			body:      `{"action":"deny"}`,
			wantReply: "reject",
		},
		{
			name:      "reply field accepted (once)",
			body:      `{"reply":"once"}`,
			wantReply: "once",
		},
		{
			name:      "response field accepted (always)",
			body:      `{"response":"always"}`,
			wantReply: "always",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := testServer(t)
			sub := b.Subscribe("permission.replied")
			defer sub.Unsubscribe()

			req := httptest.NewRequest("POST", "/session/ses_perm/permissions/perm_test",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			select {
			case evt := <-sub.C:
				props := evt.Properties.(map[string]any)
				reply, _ := props["reply"].(string)
				if reply != tt.wantReply {
					t.Errorf("expected reply %q, got %q", tt.wantReply, reply)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for permission.replied event")
			}
		})
	}
}

func TestHandleSessionPermissionReply_UnknownActionReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_perm/permissions/perm_bad",
		strings.NewReader(`{"action":"maybe"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown action, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSessionPermissionReply_MissingAllFieldsReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_perm/permissions/perm_empty",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing action/reply/response, got %d", w.Code)
	}
}

func TestHandleSessionPermissionReply_InvalidBodyReturns400(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_perm/permissions/perm_inv",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}

// --- parseDiffStats ---

func TestParseDiffStats_ParsesAdditionsAndDeletions(t *testing.T) {
	diff := `diff --git a/file1.go b/file1.go
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {
-	println("old")
+	fmt.Println("new")
 }
diff --git a/file2.go b/file2.go
--- a/file2.go
+++ b/file2.go
@@ -1 +1 @@
-package old
+package new
`

	files, additions, deletions := parseDiffStats(diff)

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "file1.go" || files[1] != "file2.go" {
		t.Errorf("expected [file1.go, file2.go], got %v", files)
	}
	if additions != 3 {
		t.Errorf("expected 3 additions, got %d", additions)
	}
	if deletions != 2 {
		t.Errorf("expected 2 deletions, got %d", deletions)
	}
}

func TestParseDiffStats_EmptyDiffReturnsEmptySlice(t *testing.T) {
	files, additions, deletions := parseDiffStats("")

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
	if additions != 0 {
		t.Errorf("expected 0 additions, got %d", additions)
	}
	if deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", deletions)
	}
}

// --- handleSessionSummarize ---

func TestHandleSessionSummarize_NotFoundReturns404(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_nonexistent/summarize", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleSessionRevert ---

func TestHandleSessionRevert_NotFoundReturns404(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_nonexistent/revert", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent session, got %d", w.Code)
	}
}

// --- handleSessionUnrevert ---

func TestHandleSessionUnrevert_NotFoundReturns404(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/ses_nonexistent/unrevert", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent session, got %d", w.Code)
	}
}
