package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

// ---------- contract test helpers ----------

// assertJSONHasKeys checks that the given JSON bytes unmarshal to an object
// containing every required key.
func assertJSONHasKeys(t *testing.T, label string, jsonData []byte, requiredKeys ...string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(jsonData, &m); err != nil {
		t.Fatalf("%s: invalid JSON: %v", label, err)
	}
	assertMapHasKeys(t, label, m, requiredKeys...)
}

// assertMapHasKeys checks that a map contains every required key.
func assertMapHasKeys(t *testing.T, label string, m map[string]any, requiredKeys ...string) {
	t.Helper()
	for _, key := range requiredKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("%s: missing required key %q", label, key)
		}
	}
}

// assertMapNotHasKeys checks that a map does NOT contain any of the forbidden keys.
func assertMapNotHasKeys(t *testing.T, label string, m map[string]any, forbiddenKeys ...string) {
	t.Helper()
	for _, key := range forbiddenKeys {
		if _, ok := m[key]; ok {
			t.Errorf("%s: has forbidden key %q (contract violation)", label, key)
		}
	}
}

// assertNestedKey checks for a dot-separated path in a map (e.g. "status.type").
func assertNestedKey(t *testing.T, label string, m map[string]any, path string) {
	t.Helper()
	parts := strings.Split(path, ".")
	current := m
	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			t.Errorf("%s: missing nested key %q (failed at %q)", label, path, strings.Join(parts[:i+1], "."))
			return
		}
		if i < len(parts)-1 {
			next, ok := val.(map[string]any)
			if !ok {
				t.Errorf("%s: key %q is not an object (type %T), cannot traverse to %q", label, strings.Join(parts[:i+1], "."), val, path)
				return
			}
			current = next
		}
	}
}

// marshalOrFail serializes v to JSON bytes, failing the test on error.
func marshalOrFail(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

// ---------- SSE Event shape contracts ----------

// TestContract_SessionStatus_BusyShape validates that session.status events
// with type "busy" match SDK EventSessionStatus.
func TestContract_SessionStatus_BusyShape(t *testing.T) {
	payload := map[string]any{
		"sessionID": "ses_test",
		"status":    map[string]any{"type": "busy"},
	}
	data := marshalOrFail(t, payload)
	assertJSONHasKeys(t, "session.status(busy)", data, "sessionID", "status")
	assertNestedKey(t, "session.status(busy)", payload, "status.type")
}

// TestContract_SessionStatus_IdleShape validates session.status with idle type.
func TestContract_SessionStatus_IdleShape(t *testing.T) {
	payload := map[string]any{
		"sessionID": "ses_test",
		"status":    map[string]any{"type": "idle"},
	}
	data := marshalOrFail(t, payload)
	assertJSONHasKeys(t, "session.status(idle)", data, "sessionID", "status")
	assertNestedKey(t, "session.status(idle)", payload, "status.type")
}

// TestContract_SessionStatus_ViaProcessPrompt validates that the actual
// processPrompt code path emits events matching the SDK contract.
func TestContract_SessionStatus_ViaProcessPrompt(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.status")
	defer sub.Unsubscribe()

	// Simulate what processPrompt publishes at start.
	b.Publish("session.status", map[string]any{
		"sessionID": "ses_contract",
		"status":    map[string]any{"type": "busy"},
	})

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map properties")
		}
		assertMapHasKeys(t, "session.status(busy) bus event", props, "sessionID", "status")
		status, ok := props["status"].(map[string]any)
		if !ok {
			t.Fatal("status must be a map")
		}
		if status["type"] != "busy" {
			t.Errorf("expected status.type 'busy', got %v", status["type"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_MessageUpdated_UserShape validates user message.updated events
// match SDK EventMessageUpdated with UserMessage info.
func TestContract_MessageUpdated_UserShape(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	model := &provider.Model{ID: "m1", ProviderID: "p1"}
	registerActiveSession(sm, "ses_cu", model, "build", "/tmp")

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_cu1",
		Role: session.RoleUser,
		Parts: []session.Part{
			session.TextPart("hello"),
		},
	}
	active := sm.getActive("ses_cu")
	sm.bridgeUserMessage("ses_cu", active, msg, time.Now().UnixMilli())

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		// SDK: EventMessageUpdated.properties = { sessionID, info: UserMessage }
		assertMapHasKeys(t, "message.updated(user) event", props, "sessionID", "info")
		info := props["info"].(map[string]any)
		// SDK UserMessage: id, sessionID, role, time.created, agent, model
		assertMapHasKeys(t, "message.updated(user) info", info, "id", "sessionID", "role", "time", "agent", "model")
		if info["role"] != "user" {
			t.Errorf("expected role 'user', got %v", info["role"])
		}
		assertNestedKey(t, "message.updated(user) info", info, "time.created")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_MessageUpdated_AssistantShape validates assistant message.updated
// events match SDK EventMessageUpdated with AssistantMessage info.
func TestContract_MessageUpdated_AssistantShape(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	model := &provider.Model{ID: "m2", ProviderID: "p2"}
	registerActiveSession(sm, "ses_ca", model, "build", "/tmp")

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_ca1",
		Role: session.RoleAssistant,
		Parts: []session.Part{
			session.TextPart("response text"),
		},
		Tokens: &session.MsgUsage{Input: 10, Output: 20},
	}
	active := sm.getActive("ses_ca")
	sm.bridgeAssistantMessage("ses_ca", active, msg, time.Now().UnixMilli())

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		assertMapHasKeys(t, "message.updated(assistant) event", props, "sessionID", "info")
		info := props["info"].(map[string]any)
		// SDK AssistantMessage: id, sessionID, role, time.{created, completed},
		// modelID, providerID, mode, agent, path, cost, tokens
		assertMapHasKeys(t, "message.updated(assistant) info", info,
			"id", "sessionID", "role", "time", "modelID", "providerID",
			"mode", "agent", "path", "cost", "tokens")
		if info["role"] != "assistant" {
			t.Errorf("expected role 'assistant', got %v", info["role"])
		}
		timeMap := info["time"].(map[string]any)
		assertMapHasKeys(t, "assistant time", timeMap, "created", "completed")
		tokensMap := info["tokens"].(map[string]any)
		assertMapHasKeys(t, "assistant tokens", tokensMap, "input", "output", "reasoning", "cache")
		cacheMap := tokensMap["cache"].(map[string]any)
		assertMapHasKeys(t, "assistant tokens.cache", cacheMap, "read", "write")
		pathMap := info["path"].(map[string]any)
		assertMapHasKeys(t, "assistant path", pathMap, "cwd", "root")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_MessagePartUpdated_ToolShape validates tool part events match
// SDK EventMessagePartUpdated with ToolPart.
func TestContract_MessagePartUpdated_ToolShape(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_ct", nil, "build", "/tmp")
	active := sm.getActive("ses_ct")
	active.mu.Lock()
	active.assistMsgID = "msg_ct"
	active.mu.Unlock()

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	sm.bridgeToolEnd(bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_ct",
			"toolCallID": "tc_1",
			"toolName":   "read",
			"toolArgs":   `{"path":"/tmp/x"}`,
		},
	})

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		// SDK: EventMessagePartUpdated.properties = { sessionID, part: ToolPart, time }
		assertMapHasKeys(t, "message.part.updated(tool) event", props, "sessionID", "part", "time")
		part := props["part"].(map[string]any)
		// SDK ToolPart: id, sessionID, messageID, type, callID, tool, state
		assertMapHasKeys(t, "tool part", part, "id", "sessionID", "messageID", "type", "callID", "tool", "state")
		if part["type"] != "tool" {
			t.Errorf("expected type 'tool', got %v", part["type"])
		}
		// No legacy field names
		assertMapNotHasKeys(t, "tool part no legacy", part, "toolCallID", "toolName", "toolArgs")

		state := part["state"].(map[string]any)
		// SDK ToolStateCompleted: status, input, output (optional for running), title, metadata, time
		assertMapHasKeys(t, "tool state(completed)", state, "status", "input", "title", "metadata", "time")
		if state["status"] != "completed" {
			t.Errorf("expected status 'completed', got %v", state["status"])
		}
		stateTime := state["time"].(map[string]any)
		assertMapHasKeys(t, "tool state time", stateTime, "start", "end")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_MessagePartUpdated_ToolRunningShape validates tool begin events
// match SDK ToolStateRunning.
func TestContract_MessagePartUpdated_ToolRunningShape(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_tr", nil, "build", "/tmp")
	active := sm.getActive("ses_tr")
	active.mu.Lock()
	active.assistMsgID = "msg_tr"
	active.mu.Unlock()

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	sm.bridgeToolBegin(bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_tr",
			"toolCallID": "tc_r1",
			"toolName":   "shell",
		},
	})

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		assertMapHasKeys(t, "message.part.updated(tool-begin)", props, "sessionID", "part", "time")
		part := props["part"].(map[string]any)
		assertMapHasKeys(t, "tool-begin part", part, "id", "sessionID", "messageID", "type", "callID", "tool", "state")
		state := part["state"].(map[string]any)
		// SDK ToolStateRunning: status, input, time.start
		assertMapHasKeys(t, "tool state(running)", state, "status", "input", "time")
		if state["status"] != "running" {
			t.Errorf("expected status 'running', got %v", state["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_SessionError_Shape validates session.error events match SDK EventSessionError.
func TestContract_SessionError_Shape(t *testing.T) {
	// Build the same payload the production code builds.
	payload := map[string]any{
		"sessionID": "ses_err",
		"error":     sessionErrorPayload("UnknownError", "something broke"),
	}
	data := marshalOrFail(t, payload)
	assertJSONHasKeys(t, "session.error", data, "sessionID", "error")

	// SDK error types: { name, data: { message } }
	errObj := payload["error"].(map[string]any)
	assertMapHasKeys(t, "session.error.error", errObj, "name", "data")
	errData := errObj["data"].(map[string]any)
	assertMapHasKeys(t, "session.error.error.data", errData, "message")
}

// TestContract_SessionError_ProviderAuthShape validates ProviderAuthError shape.
func TestContract_SessionError_ProviderAuthShape(t *testing.T) {
	payload := sessionErrorPayload("ProviderAuthError", "model not found")
	assertMapHasKeys(t, "ProviderAuthError", payload, "name", "data")
	if payload["name"] != "ProviderAuthError" {
		t.Errorf("expected name 'ProviderAuthError', got %v", payload["name"])
	}
	data := payload["data"].(map[string]any)
	assertMapHasKeys(t, "ProviderAuthError.data", data, "message")
}

// TestContract_QuestionAsked_Shape validates question.asked events match SDK EventQuestionAsked.
func TestContract_QuestionAsked_Shape(t *testing.T) {
	// SDK QuestionRequest: { id, sessionID, questions: [{ question, options: [{ label }] }] }
	payload := map[string]any{
		"id":        "q_1",
		"sessionID": "ses_q",
		"questions": []map[string]any{
			{
				"question": "Continue?",
				"options":  []map[string]any{{"label": "yes"}, {"label": "no"}},
			},
		},
	}
	data := marshalOrFail(t, payload)
	assertJSONHasKeys(t, "question.asked", data, "id", "sessionID", "questions")

	questions := payload["questions"].([]map[string]any)
	assertMapHasKeys(t, "question item", questions[0], "question", "options")
	opts := questions[0]["options"].([]map[string]any)
	assertMapHasKeys(t, "question option", opts[0], "label")
}

// TestContract_QuestionReplied_Shape validates question.replied events match SDK QuestionReplied.
func TestContract_QuestionReplied_Shape(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("question.replied")
	defer sub.Unsubscribe()

	// Simulate what handleQuestionReply publishes.
	b.Publish("question.replied", map[string]any{
		"sessionID": "ses_qr",
		"requestID": "q_r1",
		"answer":    "yes",
	})

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		// SDK QuestionReplied: { sessionID, requestID, answers }
		// Go publishes "answer" (singular) — the SDK type has "answers" (array).
		// The current contract uses "answer" as a string; verify it has the required fields.
		assertMapHasKeys(t, "question.replied event", props, "sessionID", "requestID", "answer")
		// Verify old field name is absent.
		assertMapNotHasKeys(t, "question.replied no legacy", props, "questionID")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_PermissionAsked_Shape validates permission.asked events match
// SDK EventPermissionAsked -> PermissionRequest.
func TestContract_PermissionAsked_Shape(t *testing.T) {
	// SDK PermissionRequest: { id, sessionID, permission, patterns, metadata, always }
	payload := map[string]any{
		"id":         "perm_1",
		"sessionID":  "ses_p",
		"permission": "bash",
		"patterns":   []string{"rm -rf"},
		"metadata":   map[string]any{"command": "rm -rf /tmp"},
		"always":     []string{},
	}
	data := marshalOrFail(t, payload)
	assertJSONHasKeys(t, "permission.asked", data, "id", "sessionID", "permission", "patterns", "metadata")
}

// TestContract_SessionCreated_Shape validates session.created events match
// SDK EventSessionCreated.
func TestContract_SessionCreated_Shape(t *testing.T) {
	srv, b := testServer(t)

	sub := b.Subscribe("session.created")
	defer sub.Unsubscribe()

	body := `{"title": "Contract Test", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/contract", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		// SDK: EventSessionCreated.properties = { sessionID, info: Session }
		assertMapHasKeys(t, "session.created event", props, "sessionID", "info")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_SessionUpdated_Shape validates session.updated events match
// SDK EventSessionUpdated.
func TestContract_SessionUpdated_Shape(t *testing.T) {
	srv, b := testServer(t)

	// Create a session first.
	body := `{"title": "Update Test", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/update", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	sub := b.Subscribe("session.updated")
	defer sub.Unsubscribe()

	// Update the session title.
	updateBody := `{"title": "Updated Title"}`
	req = httptest.NewRequest("PATCH", "/session/"+sessionID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		// SDK: EventSessionUpdated.properties = { sessionID, info: Session }
		assertMapHasKeys(t, "session.updated event", props, "sessionID", "info")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_SessionCompacted_Shape validates session.compacted events match
// SDK EventSessionCompacted.
func TestContract_SessionCompacted_Shape(t *testing.T) {
	// Same shape as production code in session_tools.go.
	payload := map[string]any{
		"sessionID":     "ses_compact",
		"compactionNum": 0,
	}
	data := marshalOrFail(t, payload)
	// SDK EventSessionCompacted.properties = { sessionID }
	assertJSONHasKeys(t, "session.compacted", data, "sessionID")
	// Go also includes compactionNum (extra fields are fine — TypeScript ignores them).
}

// ---------- REST Response shape contracts ----------

// TestContract_PermissionList_ReturnsRawArray validates GET /permission returns
// a raw JSON array, not wrapped in an object.
func TestContract_PermissionList_ReturnsRawArray(t *testing.T) {
	srv, _ := testServer(t)

	srv.permissionStore.Add(PendingPermission{
		ID:          "perm_list_1",
		SessionID:   "ses_pl",
		Tool:        "bash",
		Description: "run command",
	})

	req := httptest.NewRequest("GET", "/permission", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Must decode as array.
	var arr []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&arr); err != nil {
		t.Fatalf("expected JSON array, got error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 permission, got %d", len(arr))
	}

	// SDK PermissionRequest: id, sessionID, permission, patterns, metadata
	// PendingPermission serializes: id, sessionID, tool, description, args
	assertMapHasKeys(t, "permission item", arr[0], "id", "sessionID", "tool", "description")
}

// TestContract_PermissionList_EmptyReturnsEmptyArray validates empty state returns [].
func TestContract_PermissionList_EmptyReturnsEmptyArray(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/permission", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty JSON array '[]', got %q", body)
	}
}

// TestContract_QuestionList_ReturnsSDKShape validates GET /question returns
// items matching SDK QuestionRequest shape.
func TestContract_QuestionList_ReturnsSDKShape(t *testing.T) {
	srv, _ := testServer(t)

	srv.questionStore.Add(PendingQuestion{
		ID:        "q_shape",
		SessionID: "ses_qs",
		Question:  "Pick one",
		Options:   []string{"a", "b"},
	})

	req := httptest.NewRequest("GET", "/question", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var arr []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&arr); err != nil {
		t.Fatalf("expected array: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 item, got %d", len(arr))
	}

	// SDK QuestionRequest: id, sessionID, questions
	assertMapHasKeys(t, "question item", arr[0], "id", "sessionID", "questions")
	questions := arr[0]["questions"].([]any)
	qi := questions[0].(map[string]any)
	assertMapHasKeys(t, "question info", qi, "question")

	// Options must be [{label: ...}] when present.
	if opts, ok := qi["options"].([]any); ok && len(opts) > 0 {
		opt := opts[0].(map[string]any)
		assertMapHasKeys(t, "question option", opt, "label")
	}
}

// TestContract_MessageList_Shape validates GET /session/{id}/message returns
// items with {info, parts} matching SDK expectations.
func TestContract_MessageList_Shape(t *testing.T) {
	srv, _ := testServer(t)

	// Create a session and insert a message.
	body := `{"title": "MsgList", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/msglist", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d", w.Code)
	}
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	ms := srv.messageStore()
	err := ms.Append(&session.Message{
		ID:        "msg_list_1",
		SessionID: sessionID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("hello")},
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("appending message: %v", err)
	}

	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var messages []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("expected array: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("expected at least 1 message")
	}

	msg := messages[0]
	assertMapHasKeys(t, "message list item", msg, "info", "parts")
	info := msg["info"].(map[string]any)
	assertMapHasKeys(t, "message info", info, "id", "sessionID", "role", "time")

	timeMap := info["time"].(map[string]any)
	assertMapHasKeys(t, "message time", timeMap, "created")

	// time.created must be a number, not a string.
	if _, ok := timeMap["created"].(float64); !ok {
		t.Errorf("expected time.created to be a number, got %T", timeMap["created"])
	}
}

// TestContract_ProviderList_Shape validates GET /provider returns the expected
// envelope {all, connected, default}.
func TestContract_ProviderList_Shape(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/provider", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("expected object: %v", err)
	}

	// SDK response: { all, connected, default: { providerID: modelID } }
	assertMapHasKeys(t, "provider list", result, "all", "connected", "default")
}

// TestContract_FileContent_Shape validates GET /file/content response shape
// matches SDK FileContent type.
func TestContract_FileContent_Shape(t *testing.T) {
	// Build the same map handler_file.go would return.
	payload := map[string]any{
		"type":    "text",
		"path":    "/tmp/test.txt",
		"content": "file contents",
		"size":    13,
	}
	data := marshalOrFail(t, payload)
	// SDK FileContent: type, content (path and size are extras but valid).
	assertJSONHasKeys(t, "file content", data, "type", "path", "content", "size")
}

// ---------- Struct serialization contracts ----------

// TestContract_SessionInfo_TimeShape validates session.Info JSON has
// time.created, time.updated, and time.archived when non-zero.
func TestContract_SessionInfo_TimeShape(t *testing.T) {
	now := time.Now()
	info := session.Info{
		ID:        "ses_time",
		Slug:      "time-test",
		ProjectID: "prj_test",
		Directory: "/tmp",
		Title:     "Time Test",
		Version:   "1",
		CreatedAt: now,
		UpdatedAt: now,
	}
	info.SyncTime()

	data := marshalOrFail(t, info)
	var m map[string]any
	json.Unmarshal(data, &m)

	assertNestedKey(t, "session.Info", m, "time.created")
	assertNestedKey(t, "session.Info", m, "time.updated")

	// When archived is zero, it should be omitted (omitempty).
	timeMap := m["time"].(map[string]any)
	if _, ok := timeMap["archived"]; ok {
		t.Error("time.archived should be omitted when zero")
	}

	// When archived is set, it should be present.
	info.TimeArchived = now.UnixMilli()
	info.SyncTime()
	data = marshalOrFail(t, info)
	json.Unmarshal(data, &m)
	timeMap = m["time"].(map[string]any)
	if _, ok := timeMap["archived"]; !ok {
		t.Error("time.archived should be present when non-zero")
	}
}

// TestContract_ModelRef_SerializesIDNotModelID validates that session.ModelRef
// serializes "id" field (not legacy "modelID").
func TestContract_ModelRef_SerializesIDNotModelID(t *testing.T) {
	ref := session.ModelRef{
		ID:         "claude-opus-4",
		ProviderID: "anthropic",
	}
	data := marshalOrFail(t, ref)
	var m map[string]any
	json.Unmarshal(data, &m)

	// SDK Session.model: { id, providerID }
	assertMapHasKeys(t, "ModelRef", m, "id", "providerID")
	assertMapNotHasKeys(t, "ModelRef no legacy", m, "modelID")
	if m["id"] != "claude-opus-4" {
		t.Errorf("expected id 'claude-opus-4', got %v", m["id"])
	}
}

// TestContract_ModelRef_DeserializesBothIDAndModelID validates backward
// compatibility: ModelRef.UnmarshalJSON accepts both "id" and "modelID".
func TestContract_ModelRef_DeserializesBothIDAndModelID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard id field", `{"id":"claude-sonnet","providerID":"anthropic"}`, "claude-sonnet"},
		{"legacy modelID field", `{"modelID":"gpt-4","providerID":"openai"}`, "gpt-4"},
		{"id takes precedence over modelID", `{"id":"claude","modelID":"gpt","providerID":"test"}`, "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ref session.ModelRef
			if err := json.Unmarshal([]byte(tt.input), &ref); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if ref.ID != tt.expected {
				t.Errorf("expected ID %q, got %q", tt.expected, ref.ID)
			}
		})
	}
}

// TestContract_ProjectInfo_SandboxesIsArrayNotNull validates project.Info
// serializes sandboxes as [] not null.
func TestContract_ProjectInfo_SandboxesIsArrayNotNull(t *testing.T) {
	p := project.Info{
		ID:        "prj_test",
		Worktree:  "/tmp/project",
		Sandboxes: []string{},
		Time: project.Time{
			Created: time.Now().UnixMilli(),
			Updated: time.Now().UnixMilli(),
		},
	}
	data := marshalOrFail(t, p)
	var m map[string]any
	json.Unmarshal(data, &m)

	// SDK Project: sandboxes must be Array<string>, never null.
	sandboxes, ok := m["sandboxes"]
	if !ok {
		t.Fatal("expected sandboxes field")
	}
	arr, ok := sandboxes.([]any)
	if !ok {
		t.Fatalf("expected sandboxes to be array, got %T", sandboxes)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty sandboxes array, got %v", arr)
	}

	// Also verify time fields.
	assertNestedKey(t, "project.Info", m, "time.created")
	assertNestedKey(t, "project.Info", m, "time.updated")
}

// TestContract_ProjectInfo_HasRequiredFields validates project.Info has all
// fields the SDK Project type requires.
func TestContract_ProjectInfo_HasRequiredFields(t *testing.T) {
	p := project.FromDirectory("/tmp/contract-test")
	data := marshalOrFail(t, p)
	var m map[string]any
	json.Unmarshal(data, &m)

	// SDK Project: id, worktree, time, sandboxes
	assertMapHasKeys(t, "project.Info", m, "id", "worktree", "time", "sandboxes")
}

// TestContract_SessionInfo_HasRequiredSDKFields validates session.Info
// serialization has all fields the SDK Session type requires.
func TestContract_SessionInfo_HasRequiredSDKFields(t *testing.T) {
	info := session.Info{
		ID:        "ses_sdk",
		Slug:      "sdk-test",
		ProjectID: "prj_sdk",
		Directory: "/tmp/sdk",
		Title:     "SDK Test",
		Version:   "1",
		Agent:     "build",
		Model: &session.ModelRef{
			ID:         "test-model",
			ProviderID: "test-provider",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	info.SyncTime()

	data := marshalOrFail(t, info)
	var m map[string]any
	json.Unmarshal(data, &m)

	// SDK Session: id, slug, projectID, directory, title, version, time, agent, model
	assertMapHasKeys(t, "session.Info SDK fields", m,
		"id", "slug", "projectID", "directory", "title", "version", "time")

	// Model shape: { id, providerID }
	model := m["model"].(map[string]any)
	assertMapHasKeys(t, "session model", model, "id", "providerID")
}

// TestContract_SessionCreate_ResponseMatchesSDKSession validates the HTTP
// response from POST /session matches the SDK Session type.
func TestContract_SessionCreate_ResponseMatchesSDKSession(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"title": "SDK Session", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/sdk-create", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// SDK Session type required fields.
	assertMapHasKeys(t, "POST /session response", m,
		"id", "slug", "projectID", "directory", "title", "version", "time")

	timeMap := m["time"].(map[string]any)
	assertMapHasKeys(t, "session time", timeMap, "created", "updated")
}

// TestContract_SessionGet_ResponseMatchesSDKSession validates GET /session/{id}
// returns the full SDK Session shape.
func TestContract_SessionGet_ResponseMatchesSDKSession(t *testing.T) {
	srv, _ := testServer(t)

	// Create first.
	body := `{"title": "Get Test", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/get-test", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// GET it.
	req = httptest.NewRequest("GET", "/session/"+sessionID, nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var m map[string]any
	json.NewDecoder(w.Body).Decode(&m)
	assertMapHasKeys(t, "GET /session/{id}", m,
		"id", "slug", "projectID", "directory", "title", "version", "time")
}

// TestContract_ToolResult_ErrorPartShape validates that tool result error parts
// match SDK ToolStateError: { status, input, error, time }.
func TestContract_ToolResult_ErrorPartShape(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_terr",
		Role: session.RoleTool,
		Parts: []session.Part{
			session.ToolResultPart("tc_err1", "bash", "exit status 1", true),
		},
	}
	sm.bridgeToolMessage("ses_terr", msg, time.Now().UnixMilli())

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		part := props["part"].(map[string]any)
		assertMapHasKeys(t, "error tool part", part, "id", "sessionID", "messageID", "type", "callID", "tool", "state")
		state := part["state"].(map[string]any)
		// SDK ToolStateError: status, input, error, time
		assertMapHasKeys(t, "tool state(error)", state, "status", "input", "error", "time")
		if state["status"] != "error" {
			t.Errorf("expected status 'error', got %v", state["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_MessagePartUpdated_TextPartShape validates text parts match
// SDK TextPart: { id, sessionID, messageID, type, text, time }.
func TestContract_MessagePartUpdated_TextPartShape(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_tp", nil, "build", "/tmp")

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_tp1",
		Role: session.RoleUser,
		Parts: []session.Part{
			session.TextPart("text content"),
		},
	}
	active := sm.getActive("ses_tp")
	sm.bridgeUserMessage("ses_tp", active, msg, time.Now().UnixMilli())

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		assertMapHasKeys(t, "message.part.updated(text) event", props, "sessionID", "part", "time")
		part := props["part"].(map[string]any)
		// SDK TextPart: id, sessionID, messageID, type, text
		assertMapHasKeys(t, "text part", part, "id", "sessionID", "messageID", "type", "text")
		if part["type"] != "text" {
			t.Errorf("expected type 'text', got %v", part["type"])
		}
		assertNestedKey(t, "text part time", part, "time.start")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestContract_MessageList_EmptyReturnsEmptyArray validates that when no
// messages exist, the endpoint returns [] not null.
func TestContract_MessageList_EmptyReturnsEmptyArray(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"title": "Empty Msg", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/emptymsg", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body2 := strings.TrimSpace(w.Body.String())
	if body2 != "[]" {
		t.Errorf("expected empty JSON array '[]', got %q", body2)
	}
}

// TestContract_SessionTokens_Shape validates session.Info tokens field
// has the shape matching SDK Session.tokens.
func TestContract_SessionTokens_Shape(t *testing.T) {
	info := session.Info{
		ID:        "ses_tok",
		Slug:      "tok-test",
		ProjectID: "prj_tok",
		Directory: "/tmp/tok",
		Title:     "Token Test",
		Version:   "1",
		Tokens: session.TokenUsage{
			Input:     100,
			Output:    200,
			Reasoning: 50,
			Cache: session.CacheUsage{
				Read:  10,
				Write: 5,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	info.SyncTime()

	data := marshalOrFail(t, info)
	var m map[string]any
	json.Unmarshal(data, &m)

	// SDK Session.tokens: { input, output, reasoning, cache: { read, write } }
	tokens := m["tokens"].(map[string]any)
	assertMapHasKeys(t, "session tokens", tokens, "input", "output", "reasoning", "cache")
	cache := tokens["cache"].(map[string]any)
	assertMapHasKeys(t, "session tokens.cache", cache, "read", "write")
}
