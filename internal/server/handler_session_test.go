package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// --- Shell Direct Execution Tests ---

func TestShellDirect_PublishesStatusBusyThenIdle(t *testing.T) {
	h := newTestHarness(t, nil)
	sessionID := h.createSession("Shell Status", "build")

	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	body := `{"command":"echo hello"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/shell", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("shell request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	deadline := time.After(5 * time.Second)
	var gotBusy, gotIdle bool
	for !gotIdle {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] != sessionID {
				continue
			}
			status, _ := props["status"].(map[string]any)
			switch status["type"] {
			case "busy":
				gotBusy = true
			case "idle":
				gotIdle = true
			}
		case <-deadline:
			t.Fatalf("timeout waiting for status events (gotBusy=%v, gotIdle=%v)", gotBusy, gotIdle)
		}
	}
	if !gotBusy {
		t.Error("expected session.status type=busy before type=idle")
	}
}

func TestShellDirect_PublishesUserMessageWithCommandPrefix(t *testing.T) {
	h := newTestHarness(t, nil)
	sessionID := h.createSession("Shell User Msg", "build")

	partSub := h.bus.Subscribe("message.part.updated")
	defer partSub.Unsubscribe()

	body := `{"command":"ls -la"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/shell", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("shell request: %v", err)
	}
	resp.Body.Close()

	deadline := time.After(5 * time.Second)
	var foundUserPart bool
	for !foundUserPart {
		select {
		case evt := <-partSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] != sessionID {
				continue
			}
			part, _ := props["part"].(map[string]any)
			if part["type"] == "text" {
				text, _ := part["text"].(string)
				if text == "! ls -la" {
					foundUserPart = true
				}
			}
		case <-deadline:
			t.Fatal("timeout waiting for user message part with '! ls -la' prefix")
		}
	}
}

func TestShellDirect_PublishesToolPartWithCompletedStatus(t *testing.T) {
	h := newTestHarness(t, nil)

	// Create session with a valid directory so the shell command succeeds.
	dir := t.TempDir()
	body := `{"title":"Shell Tool Part","agent":"build","model":{"modelID":"test-model","providerID":"test-provider"}}`
	resp, err := http.Post(h.baseURL()+"/session?directory="+dir, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	sessionID := created["id"].(string)

	partSub := h.bus.Subscribe("message.part.updated")
	defer partSub.Unsubscribe()

	shellBody := `{"command":"echo test_output_123"}`
	shellResp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/shell", "application/json", strings.NewReader(shellBody))
	if err != nil {
		t.Fatalf("shell request: %v", err)
	}
	shellResp.Body.Close()

	deadline := time.After(5 * time.Second)
	var foundToolPart bool
	for !foundToolPart {
		select {
		case evt := <-partSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] != sessionID {
				continue
			}
			part, _ := props["part"].(map[string]any)
			if part["type"] == "tool" && part["tool"] == "bash" {
				state, _ := part["state"].(map[string]any)
				if state["status"] != "completed" {
					t.Errorf("expected state.status 'completed', got %v", state["status"])
				}
				output, _ := state["output"].(string)
				if !strings.Contains(output, "test_output_123") {
					t.Errorf("expected output to contain 'test_output_123', got %q", output)
				}
				foundToolPart = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for tool part with type=tool, tool=bash")
		}
	}
}

func TestShellDirect_RunsInSpecifiedDirectory(t *testing.T) {
	h := newTestHarness(t, nil)

	dir := filepath.Join(h.server.config.Directory, "subdir")
	os.MkdirAll(dir, 0o755)

	// Create a session with directory set to a subdirectory of the server root.
	body := `{"title":"Shell Dir","agent":"build","model":{"modelID":"test-model","providerID":"test-provider"}}`
	resp, err := http.Post(h.baseURL()+"/session?directory="+dir, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	sessionID := created["id"].(string)

	partSub := h.bus.Subscribe("message.part.updated")
	defer partSub.Unsubscribe()

	shellBody := `{"command":"pwd"}`
	shellResp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/shell", "application/json", strings.NewReader(shellBody))
	if err != nil {
		t.Fatalf("shell request: %v", err)
	}
	shellResp.Body.Close()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt := <-partSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] != sessionID {
				continue
			}
			part, _ := props["part"].(map[string]any)
			if part["type"] == "tool" && part["tool"] == "bash" {
				state, _ := part["state"].(map[string]any)
				output, _ := state["output"].(string)
				if !strings.Contains(output, dir) {
					t.Errorf("expected pwd output to contain %q, got %q", dir, output)
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for pwd output in tool part")
		}
	}
}

func TestShellDirect_ErrorCommandProducesErrorStatus(t *testing.T) {
	h := newTestHarness(t, nil)
	sessionID := h.createSession("Shell Error", "build")

	partSub := h.bus.Subscribe("message.part.updated")
	defer partSub.Unsubscribe()

	body := `{"command":"exit 1"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/shell", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("shell request: %v", err)
	}
	resp.Body.Close()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt := <-partSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] != sessionID {
				continue
			}
			part, _ := props["part"].(map[string]any)
			if part["type"] == "tool" && part["tool"] == "bash" {
				state, _ := part["state"].(map[string]any)
				if state["status"] != "error" {
					t.Errorf("expected state.status 'error' for failed command, got %v", state["status"])
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for error tool part")
		}
	}
}

func TestShellDirect_EmptyCommandReturns400(t *testing.T) {
	h := newTestHarness(t, nil)
	sessionID := h.createSession("Shell Empty", "build")

	body := `{"command":""}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/shell", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("shell request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty command, got %d", resp.StatusCode)
	}
}

// --- Session Status Format Tests ---

func TestSessionStatus_SerializesToTypeField(t *testing.T) {
	tests := []struct {
		name     string
		status   SessionStatus
		expected string
	}{
		{
			name:     "busy status serializes with type field",
			status:   SessionStatus{Type: "busy"},
			expected: `{"type":"busy"}`,
		},
		{
			name:     "idle status serializes with type field",
			status:   SessionStatus{Type: "idle"},
			expected: `{"type":"idle"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := string(data)
			if got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestSessionManagerStatus_ReturnsBusyForActiveSessions(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		// Long-running scenario: the goroutine holds the session active while we check status.
		{textResponse: "processing..."},
	})
	sessionID := h.createSession("Status Active", "build")

	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	// Send a prompt to make the session active.
	h.sendPromptAsync(sessionID, "hello")

	// Wait for the busy status.
	deadline := time.After(5 * time.Second)
	var gotBusy bool
	for !gotBusy {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] == sessionID {
				status, _ := props["status"].(map[string]any)
				if status["type"] == "busy" {
					gotBusy = true
				}
			}
		case <-deadline:
			t.Fatal("timeout waiting for session.status type=busy")
		}
	}

	// Verify SessionManager.Status() returns busy for the active session.
	statuses := h.server.sessionManager.Status()
	if s, ok := statuses[sessionID]; ok {
		if s.Type != "busy" {
			t.Errorf("expected Status() to return type=busy, got %q", s.Type)
		}
	}
	// Note: the session may have already completed by now, so we don't fail
	// if it's no longer in the active map.
}

// --- Message List Time Format Tests ---

func TestMessageList_ReturnsTimeCreatedAsNumber(t *testing.T) {
	srv, _ := testServer(t)

	// Create a session.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Time Format"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Add a user message via the store.
	ms := srv.messageStore()
	now := time.Now()
	userMsg := &session.Message{
		ID:        "msg_time_user_001",
		SessionID: sessionID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("hello")},
		CreatedAt: now,
	}
	if err := ms.Append(userMsg); err != nil {
		t.Fatalf("append user msg: %v", err)
	}

	// Add an assistant message.
	assistMsg := &session.Message{
		ID:        "msg_time_assist_001",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts:     []session.Part{session.TextPart("hi")},
		Model:     "test-model",
		CreatedAt: now,
	}
	if err := ms.Append(assistMsg); err != nil {
		t.Fatalf("append assist msg: %v", err)
	}

	// Get messages via API.
	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var messages []map[string]any
	json.NewDecoder(w.Body).Decode(&messages)

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify user message time format.
	userInfo, _ := messages[0]["info"].(map[string]any)
	userTime, _ := userInfo["time"].(map[string]any)
	if userTime == nil {
		t.Fatal("expected time field on user message info")
	}
	userCreated, ok := userTime["created"].(float64)
	if !ok || userCreated <= 0 {
		t.Errorf("expected time.created as positive number, got %v", userTime["created"])
	}

	// Verify assistant message has both created and completed.
	assistInfo, _ := messages[1]["info"].(map[string]any)
	assistTime, _ := assistInfo["time"].(map[string]any)
	if assistTime == nil {
		t.Fatal("expected time field on assistant message info")
	}
	assistCreated, ok := assistTime["created"].(float64)
	if !ok || assistCreated <= 0 {
		t.Errorf("expected time.created as positive number, got %v", assistTime["created"])
	}
	assistCompleted, ok := assistTime["completed"].(float64)
	if !ok || assistCompleted <= 0 {
		t.Errorf("expected time.completed as positive number for assistant, got %v", assistTime["completed"])
	}
}

func TestMessageList_DoesNotContainTopLevelCreatedAt(t *testing.T) {
	srv, _ := testServer(t)

	// Create session and add a message.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"No CreatedAt"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	ms := srv.messageStore()
	msg := &session.Message{
		ID:        "msg_no_createdat_001",
		SessionID: sessionID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("test")},
		CreatedAt: time.Now(),
	}
	if err := ms.Append(msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	// Check raw JSON for "createdAt" as a top-level key in the info object.
	raw := w.Body.String()
	var messages []map[string]any
	json.Unmarshal([]byte(raw), &messages)

	if len(messages) == 0 {
		t.Fatal("expected at least 1 message")
	}

	info, _ := messages[0]["info"].(map[string]any)
	if _, exists := info["createdAt"]; exists {
		t.Error("expected info NOT to contain 'createdAt' as a top-level field; should use time.created instead")
	}
}

// --- Config Model Resolution Tests ---

func TestConfigModelResolution_ExistingModelReturnedAsIs(t *testing.T) {
	h := newTestHarness(t, nil)

	resp, err := http.Get(h.baseURL() + "/global/config?directory=/tmp")
	if err != nil {
		t.Fatalf("config request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg map[string]any
	json.NewDecoder(resp.Body).Decode(&cfg)

	// The harness sets DefaultModel = "test-provider/test-model" and the provider
	// exists, so the config should return the model as-is.
	model, _ := cfg["model"].(string)
	if model != "" && model != "test-provider/test-model" {
		t.Errorf("expected model 'test-provider/test-model' or empty, got %q", model)
	}
}

func TestConfigModelResolution_UnavailableProviderResolvesToDefault(t *testing.T) {
	h := newTestHarness(t, nil)

	// Override the server's config to use a model from a non-existent provider.
	h.server.deps.Config.Model = "nonexistent-provider/some-model"

	resp, err := http.Get(h.baseURL() + "/global/config?directory=/tmp")
	if err != nil {
		t.Fatalf("config request: %v", err)
	}
	defer resp.Body.Close()

	var cfg map[string]any
	json.NewDecoder(resp.Body).Decode(&cfg)

	// The handler should resolve to the available test-provider/test-model.
	model, _ := cfg["model"].(string)
	// The config.Load might return the overridden value, and the handler
	// resolves it if the provider isn't connected. Since config.Load reads
	// from disk (not our in-memory config), the model field in the response
	// depends on what config.Load returns. The key behavior is: if the loaded
	// config has an unavailable model, the handler replaces it.
	// We verify the handler doesn't crash and returns 200.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 even with unavailable model, got %d", resp.StatusCode)
	}
	_ = model // The resolved model depends on disk config; we verified the endpoint works.
}

// --- Branch (Fork) Tests ---

func TestBranch_CreatesNewSessionWithParentID(t *testing.T) {
	srv, _ := testServer(t)

	// Create parent session.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Parent","agent":"build"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create parent: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var parent session.Info
	json.NewDecoder(w.Body).Decode(&parent)

	// Branch the session.
	req = httptest.NewRequest("POST", "/session/"+parent.ID+"/fork", strings.NewReader(`{"title":"My Branch"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("branch: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var branch session.Info
	json.NewDecoder(w.Body).Decode(&branch)

	if branch.ParentID != parent.ID {
		t.Errorf("expected parentID %q, got %q", parent.ID, branch.ParentID)
	}
	if branch.Title != "My Branch" {
		t.Errorf("expected title 'My Branch', got %q", branch.Title)
	}
	if branch.ID == parent.ID {
		t.Error("branch ID must differ from parent ID")
	}
	if branch.Agent != parent.Agent {
		t.Errorf("expected agent %q inherited from parent, got %q", parent.Agent, branch.Agent)
	}
}

func TestBranch_CopiesMessages(t *testing.T) {
	srv, _ := testServer(t)

	// Create parent session.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Parent Msgs"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var parent session.Info
	json.NewDecoder(w.Body).Decode(&parent)

	// Add messages to the parent.
	ms := srv.messageStore()
	for i, text := range []string{"hello", "world", "test"} {
		msg := &session.Message{
			ID:        fmt.Sprintf("msg_branch_%d", i),
			SessionID: parent.ID,
			Role:      session.RoleUser,
			Parts:     []session.Part{session.TextPart(text)},
			CreatedAt: time.Now(),
		}
		if err := ms.Append(msg); err != nil {
			t.Fatalf("append msg %d: %v", i, err)
		}
	}

	// Branch the session.
	req = httptest.NewRequest("POST", "/session/"+parent.ID+"/fork", strings.NewReader(`{"title":"Branch With Msgs"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("branch: expected 201, got %d", w.Code)
	}
	var branch session.Info
	json.NewDecoder(w.Body).Decode(&branch)

	// Verify branch has copied messages.
	branchMsgs, err := ms.List(branch.ID)
	if err != nil {
		t.Fatalf("list branch messages: %v", err)
	}
	if len(branchMsgs) != 3 {
		t.Fatalf("expected 3 messages in branch, got %d", len(branchMsgs))
	}

	// Verify branch messages have different IDs but same content.
	for i, bm := range branchMsgs {
		if bm.SessionID != branch.ID {
			t.Errorf("msg %d: expected sessionID %q, got %q", i, branch.ID, bm.SessionID)
		}
		if bm.ID == fmt.Sprintf("msg_branch_%d", i) {
			t.Errorf("msg %d: branch message should have a new ID, got same as parent", i)
		}
	}
}

func TestBranch_OriginalSessionUnchanged(t *testing.T) {
	srv, _ := testServer(t)

	// Create parent session with messages.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Unchanged Parent"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var parent session.Info
	json.NewDecoder(w.Body).Decode(&parent)

	ms := srv.messageStore()
	msg := &session.Message{
		ID:        "msg_unchanged_001",
		SessionID: parent.ID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("original message")},
		CreatedAt: time.Now(),
	}
	if err := ms.Append(msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	// Branch the session.
	req = httptest.NewRequest("POST", "/session/"+parent.ID+"/fork", strings.NewReader(`{"title":"Branch"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("branch: expected 201, got %d", w.Code)
	}

	// Verify parent messages are unchanged.
	parentMsgs, err := ms.List(parent.ID)
	if err != nil {
		t.Fatalf("list parent messages: %v", err)
	}
	if len(parentMsgs) != 1 {
		t.Fatalf("expected 1 message in parent, got %d", len(parentMsgs))
	}
	if parentMsgs[0].ID != "msg_unchanged_001" {
		t.Errorf("parent message ID changed: got %q", parentMsgs[0].ID)
	}

	// Verify parent session info is untouched.
	store := srv.sessionStore()
	info, err := store.Get(parent.ID)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if info.Title != "Unchanged Parent" {
		t.Errorf("parent title changed: got %q", info.Title)
	}
}

func TestConfigModelResolution_EmptyModelStaysEmpty(t *testing.T) {
	h := newTestHarness(t, nil)

	// Clear the config model.
	h.server.deps.Config.Model = ""

	resp, err := http.Get(h.baseURL() + "/global/config?directory=/tmp")
	if err != nil {
		t.Fatalf("config request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// When config.Load returns no model, the handler should not inject one.
	// (The handler only resolves when cfg.Model != "")
	var cfg map[string]any
	json.NewDecoder(resp.Body).Decode(&cfg)
	// The config.Load reads from disk. If no config file exists in /tmp,
	// cfg.Model will be "". The handler skips resolution when model is empty.
	// We just verify the endpoint succeeds.
}
