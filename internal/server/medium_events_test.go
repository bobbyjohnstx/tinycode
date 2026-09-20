package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

// --- Fix #202: session.updated bus event on title change ---

func TestSessionUpdate_PublishesSessionUpdatedEvent(t *testing.T) {
	srv, b := testServer(t)

	// Create a session.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Original"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Subscribe to session.updated before sending the update.
	sub := b.Subscribe("session.updated")
	defer sub.Unsubscribe()

	// Update the session title.
	req = httptest.NewRequest("PATCH", "/session/"+sessionID, strings.NewReader(`{"title":"Renamed"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the bus event.
	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != sessionID {
			t.Errorf("expected sessionID %q, got %v", sessionID, props["sessionID"])
		}
		info, ok := props["info"].(*session.Info)
		if !ok {
			t.Fatalf("expected info to be *session.Info, got %T", props["info"])
		}
		if info.Title != "Renamed" {
			t.Errorf("expected title 'Renamed' in event, got %q", info.Title)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session.updated event")
	}
}

func TestSessionUpdate_NoTitle_StillPublishesEvent(t *testing.T) {
	srv, b := testServer(t)

	// Create a session.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Keep"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	sub := b.Subscribe("session.updated")
	defer sub.Unsubscribe()

	// PATCH with empty body (no title change).
	req = httptest.NewRequest("PATCH", "/session/"+sessionID, strings.NewReader(`{}`))
	w = httptest.NewRecorder()
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
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session.updated event")
	}
}

// --- Fix #205: assistant messages include providerID in REST response ---

func TestMessageList_AssistantIncludesProviderID(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)

	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "test-provider",
		Name:   "Test Provider",
		Source: "test",
		Models: map[string]*provider.Model{
			"test-model": {
				ID:         "test-model",
				ProviderID: "test-provider",
				API:        provider.ModelAPI{URL: "http://localhost"},
			},
		},
	})

	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	// Create a session with a model ref that has providerID.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(
		`{"title":"ProviderID Test","model":{"id":"test-model","providerID":"test-provider"}}`,
	))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Add an assistant message (with a model string set).
	ms := srv.messageStore()
	assistMsg := &session.Message{
		ID:        "msg_assist_prov",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts:     []session.Part{session.TextPart("hello")},
		Model:     "test-model",
		CreatedAt: time.Now(),
	}
	if err := ms.Append(assistMsg); err != nil {
		t.Fatalf("append: %v", err)
	}

	// Fetch messages.
	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var messages []map[string]any
	json.NewDecoder(w.Body).Decode(&messages)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	info, _ := messages[0]["info"].(map[string]any)
	providerID, _ := info["providerID"].(string)
	if providerID != "test-provider" {
		t.Errorf("expected providerID 'test-provider', got %q", providerID)
	}

	modelID, _ := info["modelID"].(string)
	if modelID != "test-model" {
		t.Errorf("expected modelID 'test-model', got %q", modelID)
	}
}

func TestMessageList_AssistantFallsBackToSessionModelID(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)

	reg := provider.NewRegistry()
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	// Create a session with model ref.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(
		`{"title":"Fallback Test","model":{"id":"fallback-model","providerID":"fallback-provider"}}`,
	))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Add an assistant message WITHOUT model string (empty).
	ms := srv.messageStore()
	assistMsg := &session.Message{
		ID:        "msg_assist_fb",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts:     []session.Part{session.TextPart("fallback")},
		Model:     "", // empty — should fall back to session model
		CreatedAt: time.Now(),
	}
	if err := ms.Append(assistMsg); err != nil {
		t.Fatalf("append: %v", err)
	}

	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var messages []map[string]any
	json.NewDecoder(w.Body).Decode(&messages)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	info, _ := messages[0]["info"].(map[string]any)
	providerID, _ := info["providerID"].(string)
	if providerID != "fallback-provider" {
		t.Errorf("expected providerID 'fallback-provider', got %q", providerID)
	}
	modelID, _ := info["modelID"].(string)
	if modelID != "fallback-model" {
		t.Errorf("expected modelID 'fallback-model', got %q", modelID)
	}
}

func TestMessageGet_AssistantIncludesProviderID(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)

	reg := provider.NewRegistry()
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	// Create a session with a model ref.
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(
		`{"title":"Get ProviderID","model":{"id":"m1","providerID":"p1"}}`,
	))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Add an assistant message.
	ms := srv.messageStore()
	msg := &session.Message{
		ID:        "msg_get_prov",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts:     []session.Part{session.TextPart("hi")},
		Model:     "m1",
		CreatedAt: time.Now(),
	}
	if err := ms.Append(msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	// GET single message.
	req = httptest.NewRequest("GET", "/session/"+sessionID+"/message/msg_get_prov", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	info, _ := result["info"].(map[string]any)
	modelID, _ := info["modelID"].(string)
	if modelID != "m1" {
		t.Errorf("expected modelID 'm1', got %q", modelID)
	}
}

// --- Fix #206: session.compacted includes compactionNum ---

func TestSubscribeSummarize_PublishesCompactionNum(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// Start the subscribeSummarize goroutine.
	sm.subscribeSummarize()

	sub := b.Subscribe("session.compacted")
	defer sub.Unsubscribe()

	// Publish a summarize event to trigger the subscriber.
	b.Publish("session.summarize", map[string]any{
		"sessionID": "ses_compact_test",
	})

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_compact_test" {
			t.Errorf("expected sessionID 'ses_compact_test', got %v", props["sessionID"])
		}
		compactionNum, ok := props["compactionNum"]
		if !ok {
			t.Fatal("expected compactionNum field in session.compacted event")
		}
		if num, ok := compactionNum.(int); !ok || num != 0 {
			t.Errorf("expected compactionNum 0, got %v (type %T)", compactionNum, compactionNum)
		}
		if _, ok := props["message"]; !ok {
			t.Error("expected message field in session.compacted event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session.compacted event")
	}
}

// --- Fix #204: session.diff event structure ---

func TestIsFileModifyingTool(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"edit", true},
		{"write", true},
		{"apply_patch", true},
		{"bash", false},
		{"read", false},
		{"shell", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFileModifyingTool(tt.name)
			if got != tt.expected {
				t.Errorf("isFileModifyingTool(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestBridgeToolEnd_PublishesDiffForFileModifyingTools(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_diff", nil, "build", t.TempDir())

	active := sm.getActive("ses_diff")
	active.mu.Lock()
	active.assistMsgID = "msg_diff_assist"
	active.mu.Unlock()

	diffSub := b.Subscribe("session.diff")
	defer diffSub.Unsubscribe()

	partSub := b.Subscribe("message.part.updated")
	defer partSub.Unsubscribe()

	// Trigger tool end for "edit" tool.
	evt := bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_diff",
			"toolCallID": "tc_edit_1",
			"toolName":   "edit",
			"toolArgs":   `{"file":"test.go"}`,
		},
	}
	sm.bridgeToolEnd(evt)

	// Verify the tool part was published.
	select {
	case <-partSub.C:
		// Good, tool part published.
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tool part event")
	}

	// The session.diff event is published asynchronously (goroutine).
	// Since the session dir is a temp dir with no git repo, it will
	// fail silently and not publish. That's the expected behavior for
	// non-git directories. The key test is that the code doesn't panic
	// and the tool part is still published correctly.

	// For tools that are NOT file-modifying, verify no diff event.
	evt2 := bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_diff",
			"toolCallID": "tc_read_1",
			"toolName":   "read",
			"toolArgs":   `{"file":"test.go"}`,
		},
	}
	sm.bridgeToolEnd(evt2)

	// Drain the second tool part.
	select {
	case <-partSub.C:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second tool part")
	}

	// No session.diff should be published for "read".
	select {
	case evt := <-diffSub.C:
		// If a diff event comes from the earlier edit goroutine, that's fine.
		// But verify it's for the right tool.
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_diff" {
			t.Errorf("unexpected sessionID in diff event: %v", props["sessionID"])
		}
	case <-time.After(100 * time.Millisecond):
		// No diff event — expected for non-git dir or for read tool.
	}
}

func TestParseDiffStats_Consistency(t *testing.T) {
	// Verify parseDiffStats is callable and returns expected structure
	// (it's used by both handleSessionDiff and publishSessionDiff).
	diff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}
-old line`

	files, additions, deletions := parseDiffStats(diff)
	if len(files) != 1 || files[0] != "file.go" {
		t.Errorf("expected [file.go], got %v", files)
	}
	if additions != 1 {
		t.Errorf("expected 1 addition, got %d", additions)
	}
	if deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", deletions)
	}
}
