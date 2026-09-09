package server

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE project (
			id TEXT PRIMARY KEY,
			worktree TEXT NOT NULL,
			vcs TEXT,
			name TEXT,
			icon_url TEXT,
			icon_url_override TEXT,
			icon_color TEXT,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			time_initialized INTEGER,
			sandboxes TEXT NOT NULL DEFAULT '[]',
			commands TEXT
		);
		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
			slug TEXT NOT NULL,
			directory TEXT NOT NULL,
			parent_id TEXT,
			title TEXT NOT NULL,
			agent TEXT,
			model TEXT,
			version TEXT NOT NULL DEFAULT '1',
			cost REAL NOT NULL DEFAULT 0,
			tokens_input INTEGER NOT NULL DEFAULT 0,
			tokens_output INTEGER NOT NULL DEFAULT 0,
			tokens_reasoning INTEGER NOT NULL DEFAULT 0,
			tokens_cache_read INTEGER NOT NULL DEFAULT 0,
			tokens_cache_write INTEGER NOT NULL DEFAULT 0,
			summary_additions INTEGER,
			summary_deletions INTEGER,
			summary_files INTEGER,
			summary_diffs TEXT,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			time_archived INTEGER
		);
		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL REFERENCES message(id) ON DELETE CASCADE,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
	`
	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("creating schema: %v", err)
	}
	return db
}

func testServer(t *testing.T) (*Server, *bus.Bus) {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { b.Close() })

	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})
	return srv, b
}

func TestHealth(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/global/health", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)

	if body["healthy"] != true {
		t.Error("expected healthy: true")
	}
	if _, ok := body["version"]; !ok {
		t.Error("expected version field")
	}
}

func TestVersion(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/global/version", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSessionCRUD(t *testing.T) {
	srv, _ := testServer(t)

	// Create session
	body := `{"title": "Test", "agent": "build"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp/test", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	if !strings.HasPrefix(sessionID, "ses_") {
		t.Errorf("expected ses_ prefix, got %s", sessionID)
	}
	if created["title"] != "Test" {
		t.Errorf("expected title 'Test', got %v", created["title"])
	}
	if _, ok := created["time"]; !ok {
		t.Error("expected time field in response")
	}

	// Get session
	req = httptest.NewRequest("GET", "/session/"+sessionID, nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Update session
	req = httptest.NewRequest("PATCH", "/session/"+sessionID, strings.NewReader(`{"title": "Updated"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	var updated map[string]any
	json.NewDecoder(w.Body).Decode(&updated)
	if updated["title"] != "Updated" {
		t.Errorf("expected title 'Updated', got %v", updated["title"])
	}

	// List sessions
	req = httptest.NewRequest("GET", "/session?directory=/tmp/test", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var listed []any
	json.NewDecoder(w.Body).Decode(&listed)
	if len(listed) != 1 {
		t.Errorf("expected 1 session, got %d", len(listed))
	}

	// Delete session
	req = httptest.NewRequest("DELETE", "/session/"+sessionID, nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/session/"+sessionID, nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestSessionCreate_DefaultDirectory(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session", strings.NewReader(`{"title": "Test"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionList_DefaultDirectory(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/session", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionPrompt(t *testing.T) {
	srv, b := testServer(t)

	// Create session first
	createBody := `{"title": "prompt-test"}`
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(createBody))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Subscribe to prompt events
	sub := b.Subscribe("session.prompt")
	defer sub.Unsubscribe()

	// Send prompt
	req = httptest.NewRequest("POST", "/session/"+sessionID+"/message", strings.NewReader(`{"content": "hello"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	// Verify event published
	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["content"] != "hello" {
			t.Errorf("expected content 'hello', got %v", props["content"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for prompt event")
	}
}

func TestSessionPrompt_EmptyContent(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/session/test/message", strings.NewReader(`{"content": ""}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSessionAbort(t *testing.T) {
	srv, b := testServer(t)

	sub := b.Subscribe("session.abort")
	defer sub.Unsubscribe()

	req := httptest.NewRequest("POST", "/session/ses_test/abort", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_test" {
			t.Errorf("expected session ID, got %v", props["sessionID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for abort event")
	}
}

func TestSessionFork(t *testing.T) {
	srv, _ := testServer(t)

	// Create parent
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Parent"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var parent map[string]any
	json.NewDecoder(w.Body).Decode(&parent)
	parentID := parent["id"].(string)

	// Fork
	req = httptest.NewRequest("POST", "/session/"+parentID+"/fork", strings.NewReader(`{"title":"Fork"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var forked map[string]any
	json.NewDecoder(w.Body).Decode(&forked)
	if forked["parentID"] != parentID {
		t.Errorf("expected parentID %s, got %v", parentID, forked["parentID"])
	}
}

func TestSessionFork_TruncatesAtMessageID(t *testing.T) {
	srv, _ := testServer(t)

	// Create parent session
	req := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"Parent"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var parent map[string]any
	json.NewDecoder(w.Body).Decode(&parent)
	parentID := parent["id"].(string)

	// Add 4 messages to the parent session
	ms := srv.messageStore()
	msgIDs := make([]string, 4)
	for i := 0; i < 4; i++ {
		msgIDs[i] = fmt.Sprintf("msg_fork_test_%03d", i)
		msg := &session.Message{
			ID:        msgIDs[i],
			SessionID: parentID,
			Role:      session.RoleUser,
			Parts:     []session.Part{session.TextPart(fmt.Sprintf("message %d", i))},
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := ms.Append(msg); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	// Fork with messageID pointing to the second message (index 1)
	forkBody := fmt.Sprintf(`{"title":"Fork Truncated","messageID":"%s"}`, msgIDs[1])
	req = httptest.NewRequest("POST", "/session/"+parentID+"/fork", strings.NewReader(forkBody))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var forked map[string]any
	json.NewDecoder(w.Body).Decode(&forked)
	forkedID := forked["id"].(string)

	// Verify forked session has only 2 messages (up to and including msgIDs[1])
	forkedMsgs, err := ms.List(forkedID)
	if err != nil {
		t.Fatalf("list forked messages: %v", err)
	}
	if len(forkedMsgs) != 2 {
		t.Fatalf("expected 2 messages in fork (truncated at midpoint), got %d", len(forkedMsgs))
	}

	// Fork without messageID should copy all messages
	req = httptest.NewRequest("POST", "/session/"+parentID+"/fork", strings.NewReader(`{"title":"Fork All"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var forkedAll map[string]any
	json.NewDecoder(w.Body).Decode(&forkedAll)
	forkedAllID := forkedAll["id"].(string)

	allMsgs, err := ms.List(forkedAllID)
	if err != nil {
		t.Fatalf("list forked-all messages: %v", err)
	}
	if len(allMsgs) != 4 {
		t.Fatalf("expected 4 messages in full fork, got %d", len(allMsgs))
	}
}

func TestMessageList_Empty(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/session/ses_nonexistent/message", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body []any
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 0 {
		t.Errorf("expected 0 messages, got %d", len(body))
	}
}

func TestProviderList_Empty(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/provider", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestProviderGet_NotFound(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/provider/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPermissionReply(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/permission/per_test/reply", strings.NewReader(`{"action": "allow"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPermissionReply_MissingAction(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/permission/per_test/reply", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConfigGet(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/global/config?directory=/tmp", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFileList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/file?path="+dir, nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if _, ok := body["files"]; !ok {
		t.Error("expected files field")
	}
}

func TestFileList_MissingPath(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/file", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFileRead_NotFound(t *testing.T) {
	dir := t.TempDir()

	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/file/content?path="+filepath.Join(dir, "nonexistent.txt"), nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// SSE tests

func TestSSEWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sse, ok := NewSSEWriter(w)
	if !ok {
		t.Fatal("expected SSEWriter creation to succeed")
	}

	sse.Send(SSEEvent{
		Event: "test.event",
		Data:  map[string]string{"key": "value"},
		ID:    "evt-1",
	})

	body := w.Body.String()
	if !strings.Contains(body, "event: test.event") {
		t.Error("expected event field")
	}
	if !strings.Contains(body, "id: evt-1") {
		t.Error("expected id field")
	}
	if !strings.Contains(body, `"key":"value"`) {
		t.Error("expected JSON data")
	}
}

func TestSSEHeartbeat(t *testing.T) {
	w := httptest.NewRecorder()
	sse, ok := NewSSEWriter(w)
	if !ok {
		t.Fatal("expected SSEWriter")
	}

	sse.Heartbeat()

	body := w.Body.String()
	if !strings.Contains(body, ": heartbeat") {
		t.Error("expected heartbeat comment")
	}
}

func TestSSEEventStream_ConnectedEvent(t *testing.T) {
	srv, _ := testServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest("GET", "/event", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.mux.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: server.connected") {
		t.Errorf("expected server.connected event in flat SSE format, got: %s", body)
	}
}

func TestSSESessionEventStream(t *testing.T) {
	srv, b := testServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest("GET", "/session/ses_test/event", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.mux.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	b.Publish("ses_test", map[string]any{"text": "hello"})

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "server.connected") {
		t.Error("expected server.connected event")
	}
}

// Server lifecycle tests

func TestServerListenAndShutdown(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	srv := New(Config{Port: 0, Hostname: "127.0.0.1"}, Dependencies{Bus: b, DB: db})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	if listener.Port == 0 {
		t.Error("expected non-zero port")
	}

	// Hit the health endpoint
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/global/health", listener.Port))
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestServerPortFallback(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	// Bind to a port first to cause conflict
	srv1 := New(Config{Port: 0, Hostname: "127.0.0.1"}, Dependencies{Bus: b, DB: db})
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	listener1, err := srv1.Listen(ctx1)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}

	// Try binding to the same port — should fall back to random
	srv2 := New(Config{Port: listener1.Port, Hostname: "127.0.0.1"}, Dependencies{Bus: b, DB: db})
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	listener2, err := srv2.Listen(ctx2)
	if err != nil {
		t.Fatalf("second listen should fallback: %v", err)
	}

	if listener2.Port == listener1.Port {
		t.Error("expected different port on fallback")
	}

	cancel1()
	cancel2()
	time.Sleep(100 * time.Millisecond)
}

// CORS tests

func TestCORSHeaders(t *testing.T) {
	srv, _ := testServer(t)

	handler := srv.httpServer.Handler

	req := httptest.NewRequest("GET", "/global/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Error("expected CORS origin header")
	}
}

func TestCORSPreflight(t *testing.T) {
	srv, _ := testServer(t)

	handler := srv.httpServer.Handler

	req := httptest.NewRequest("OPTIONS", "/session", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected allow-methods header")
	}
}

// Security headers test

func TestSecurityHeaders(t *testing.T) {
	srv, _ := testServer(t)

	handler := srv.httpServer.Handler

	req := httptest.NewRequest("GET", "/global/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options: nosniff")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options: DENY")
	}
}

// SSE stream integration test with real HTTP server

func TestSSEStreamWithRealServer(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	srv := New(Config{Port: 0, Hostname: "127.0.0.1"}, Dependencies{Bus: b, DB: db})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	sseCtx, sseCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer sseCancel()

	sseReq, _ := http.NewRequestWithContext(sseCtx, "GET", fmt.Sprintf("http://127.0.0.1:%d/event", listener.Port), nil)
	sseReq.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("SSE request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	scanner := bufio.NewScanner(resp.Body)
	var gotConnected bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "server.connected") {
			gotConnected = true
			break
		}
	}

	if !gotConnected {
		t.Error("expected server.connected event")
	}

	cancel()
}

// Respond helpers test

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "something went wrong" {
		t.Errorf("expected error message, got %v", body)
	}
}

func TestDecodeJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"key": "value"}`))
	var body struct {
		Key string `json:"key"`
	}
	err := decodeJSON(req, &body)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Key != "value" {
		t.Errorf("expected 'value', got %q", body.Key)
	}
}

// Close reading body that returns EOF correctly
func TestDecodeJSON_InvalidBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader("not json"))
	var body struct{}
	err := decodeJSON(req, &body)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// Ensure no import of io is left unused
var _ = io.EOF

func TestDeltaBatcher_Batches(t *testing.T) {
	var mu sync.Mutex
	var flushed []string
	b := newDeltaBatcher(func(text string) {
		mu.Lock()
		flushed = append(flushed, text)
		mu.Unlock()
	})

	b.Add("hello ")
	b.Add("world")

	// Wait for debounce to fire
	time.Sleep(25 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushed))
	}
	if flushed[0] != "hello world" {
		t.Errorf("expected 'hello world', got %q", flushed[0])
	}
}

func TestDeltaBatcher_ExplicitFlush(t *testing.T) {
	var flushed []string
	b := newDeltaBatcher(func(text string) {
		flushed = append(flushed, text)
	})

	b.Add("abc")
	b.Flush()

	if len(flushed) != 1 || flushed[0] != "abc" {
		t.Errorf("expected flush of 'abc', got %v", flushed)
	}

	// Second flush should be a no-op
	b.Flush()
	if len(flushed) != 1 {
		t.Error("expected no extra flush")
	}
}
