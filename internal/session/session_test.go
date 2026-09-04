package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"

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
	`
	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("creating schema: %v", err)
	}
	return db
}

func TestStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, err := store.Create(CreateInput{
		ProjectID: "proj-1",
		Directory: "/tmp/test",
		Title:     "Test Session",
		Agent:     "build",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if !strings.HasPrefix(info.ID, "ses_") {
		t.Errorf("expected ses_ prefix, got %s", info.ID)
	}
	if info.Title != "Test Session" {
		t.Errorf("expected title 'Test Session', got %q", info.Title)
	}

	got, err := store.Get(info.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Test Session" {
		t.Errorf("expected title 'Test Session', got %q", got.Title)
	}
	if got.Agent != "build" {
		t.Errorf("expected agent 'build', got %q", got.Agent)
	}
}

func TestStore_CreateWithModel(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, err := store.Create(CreateInput{
		ProjectID: "proj-1",
		Directory: "/tmp",
		Model:     &ModelRef{ID: "llama3.2", ProviderID: "ollama"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(info.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Model == nil {
		t.Fatal("expected model to be set")
	}
	if got.Model.ID != "llama3.2" {
		t.Errorf("expected model id 'llama3.2', got %q", got.Model.ID)
	}
}

func TestStore_CreateWithParent(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	parent, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	child, err := store.Create(CreateInput{
		ProjectID: "proj-1",
		Directory: "/tmp",
		ParentID:  parent.ID,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, _ := store.Get(child.ID)
	if got.ParentID != parent.ID {
		t.Errorf("expected parent %s, got %s", parent.ID, got.ParentID)
	}
	if !strings.Contains(got.Title, "Child session") {
		t.Errorf("expected child title prefix, got %q", got.Title)
	}
}

func TestStore_List(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	for i := 0; i < 5; i++ {
		store.Create(CreateInput{
			ProjectID: "proj-1",
			Directory: "/tmp",
			Title:     fmt.Sprintf("Session %d", i),
		})
		time.Sleep(time.Millisecond)
	}

	sessions, err := store.List("proj-1", 3, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}

	all, _ := store.List("proj-1", 100, 0)
	if len(all) != 5 {
		t.Errorf("expected 5 total, got %d", len(all))
	}
}

func TestStore_UpdateTitle(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	store.UpdateTitle(info.ID, "Updated Title")

	got, _ := store.Get(info.ID)
	if got.Title != "Updated Title" {
		t.Errorf("expected updated title, got %q", got.Title)
	}
}

func TestStore_UpdateCost(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	store.UpdateCost(info.ID, 0.05, TokenUsage{Input: 100, Output: 50})

	got, _ := store.Get(info.ID)
	if got.Cost != 0.05 {
		t.Errorf("expected cost 0.05, got %f", got.Cost)
	}
	if got.Tokens.Input != 100 {
		t.Errorf("expected 100 input tokens, got %d", got.Tokens.Input)
	}
}

func TestStore_UpdateSummary(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	store.UpdateSummary(info.ID, Summary{Additions: 10, Deletions: 5, Files: 3})

	got, _ := store.Get(info.ID)
	if got.Summary == nil {
		t.Fatal("expected summary")
	}
	if got.Summary.Additions != 10 || got.Summary.Deletions != 5 || got.Summary.Files != 3 {
		t.Errorf("unexpected summary: %+v", got.Summary)
	}
}

func TestStore_Archive(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	store.Archive(info.ID)

	got, _ := store.Get(info.ID)
	if got.TimeArchived == 0 {
		t.Error("expected session to be archived")
	}
}

func TestStore_Delete(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	store.Delete(info.ID)

	_, err := store.Get(info.ID)
	if err == nil {
		t.Error("expected error getting deleted session")
	}
}

func TestMessageStore_AppendAndList(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	ses, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})

	msg := &Message{
		ID:        "msg_test1",
		SessionID: ses.ID,
		Role:      RoleUser,
		Parts:     []Part{TextPart("hello world")},
		CreatedAt: time.Now(),
	}

	if err := ms.Append(msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	messages, err := ms.List(ses.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Role != RoleUser {
		t.Errorf("expected user role, got %s", messages[0].Role)
	}
	if len(messages[0].Parts) != 1 || messages[0].Parts[0].Text != "hello world" {
		t.Error("unexpected message parts")
	}
}

func TestMessageStore_ToolCallParts(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	ses, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})

	msg := &Message{
		ID:        "msg_test2",
		SessionID: ses.ID,
		Role:      RoleAssistant,
		Parts: []Part{
			TextPart("Let me read the file"),
			ToolCallPart("call_1", "read", `{"file_path": "/tmp/test.txt"}`),
		},
		Model:     "llama3.2",
		CreatedAt: time.Now(),
	}

	ms.Append(msg)
	messages, _ := ms.List(ses.ID)

	if len(messages[0].Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(messages[0].Parts))
	}
	if messages[0].Parts[1].ToolName != "read" {
		t.Errorf("expected tool name 'read', got %q", messages[0].Parts[1].ToolName)
	}
}

func TestMessageStore_Count(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	ses, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})

	for i := 0; i < 5; i++ {
		ms.Append(&Message{
			ID:        fmt.Sprintf("msg_%d", i),
			SessionID: ses.ID,
			Role:      RoleUser,
			Parts:     []Part{TextPart("msg")},
			CreatedAt: time.Now(),
		})
	}

	count, err := ms.Count(ses.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 messages, got %d", count)
	}
}

func TestMessageStore_Delete(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	ses, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	ms.Append(&Message{
		ID: "msg_del", SessionID: ses.ID, Role: RoleUser,
		Parts: []Part{TextPart("test")}, CreatedAt: time.Now(),
	})

	ms.Delete(ses.ID)
	count, _ := ms.Count(ses.ID)
	if count != 0 {
		t.Errorf("expected 0 messages after delete, got %d", count)
	}
}

// Compaction tests

func TestLazyEstimator_EstimateMessage(t *testing.T) {
	e := NewLazyEstimator(0.25)

	msg := &Message{
		Parts: []Part{TextPart(strings.Repeat("x", 400))},
	}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected ~100 tokens, got %d", tokens)
	}
}

func TestLazyEstimator_MinTokens(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{TextPart("")}}
	tokens := e.EstimateMessage(msg)
	if tokens < 4 {
		t.Errorf("expected at least 4 tokens, got %d", tokens)
	}
}

func TestLazyEstimator_FindPreserveBoundary(t *testing.T) {
	e := NewLazyEstimator(0.25)

	messages := make([]Message, 10)
	for i := range messages {
		messages[i] = Message{
			Parts: []Part{TextPart(strings.Repeat("x", 4000))},
		}
	}

	boundary := e.FindPreserveBoundary(messages, 5000)
	if boundary >= len(messages) {
		t.Error("boundary should be within messages")
	}
	if boundary <= 0 {
		t.Error("boundary should preserve some prefix")
	}
}

func TestLazyEstimator_FindPreserveBoundary_SmallBudget(t *testing.T) {
	e := NewLazyEstimator(0.25)

	messages := []Message{
		{Parts: []Part{TextPart(strings.Repeat("x", 400))}},
		{Parts: []Part{TextPart(strings.Repeat("x", 400))}},
	}

	boundary := e.FindPreserveBoundary(messages, MinPreserveRecentTokens)
	if boundary > len(messages) {
		t.Errorf("boundary %d should not exceed message count %d", boundary, len(messages))
	}
}

func TestTrackFiles(t *testing.T) {
	messages := []Message{
		{Parts: []Part{
			{Type: PartToolCall, ToolName: "read", ToolArgs: `{"file_path": "/tmp/a.go"}`},
		}},
		{Parts: []Part{
			{Type: PartToolCall, ToolName: "edit", ToolArgs: `{"file_path": "/tmp/b.go"}`},
		}},
		{Parts: []Part{
			{Type: PartToolCall, ToolName: "write", ToolArgs: `{"path": "/tmp/c.go"}`},
		}},
	}

	readFiles, modifiedFiles := trackFiles(messages)

	readSet := make(map[string]bool)
	for _, f := range readFiles {
		readSet[f] = true
	}
	modSet := make(map[string]bool)
	for _, f := range modifiedFiles {
		modSet[f] = true
	}

	if !readSet["/tmp/a.go"] {
		t.Error("expected /tmp/a.go in read files")
	}
	if !modSet["/tmp/b.go"] {
		t.Error("expected /tmp/b.go in modified files")
	}
	if !modSet["/tmp/c.go"] {
		t.Error("expected /tmp/c.go in modified files")
	}
}

func TestMaskObservations(t *testing.T) {
	messages := []Message{
		{Parts: []Part{
			{Type: PartToolResult, ToolCallID: "c1", ToolName: "read", ToolResult: "file contents here"},
		}},
	}

	masked := maskObservations(messages)
	if masked[0].Parts[0].ToolResult != "[output masked for compaction]" {
		t.Errorf("expected masked output, got %q", masked[0].Parts[0].ToolResult)
	}
	if masked[0].Parts[0].ToolCallID != "c1" {
		t.Error("expected tool call ID preserved")
	}
	// Verify original not mutated
	if messages[0].Parts[0].ToolResult == "[output masked for compaction]" {
		t.Error("original should not be mutated")
	}
}

func TestBuildCompactionPrompt(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("hi there")}},
	}

	prompt := buildCompactionPrompt(messages, "", []string{"/tmp/a.go"}, []string{"/tmp/b.go"})

	if !strings.Contains(prompt, "<conversation>") {
		t.Error("expected <conversation> tag")
	}
	if !strings.Contains(prompt, "hello") {
		t.Error("expected user message content")
	}
	if !strings.Contains(prompt, "<read-files>") {
		t.Error("expected read-files block")
	}
	if !strings.Contains(prompt, "<modified-files>") {
		t.Error("expected modified-files block")
	}
}

func TestBuildCompactionPrompt_WithPriorSummary(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("test")}},
	}

	prompt := buildCompactionPrompt(messages, "prior summary here", nil, nil)
	if !strings.Contains(prompt, "<prior-summary>") {
		t.Error("expected prior-summary tag")
	}
	if !strings.Contains(prompt, "prior summary here") {
		t.Error("expected prior summary content")
	}
}

// Processor tests with mock LLM

type mockLLMClient struct {
	mu        sync.Mutex
	responses []mockResponse
	callCount int
}

type mockResponse struct {
	events []llm.Event
	err    error
}

func (m *mockLLMClient) Stream(ctx context.Context, req llm.Request, opts ...llm.StreamOption) (<-chan llm.Event, error) {
	m.mu.Lock()
	idx := m.callCount
	m.callCount++
	m.mu.Unlock()

	if idx >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}

	resp := m.responses[idx]
	if resp.err != nil {
		return nil, resp.err
	}

	ch := make(chan llm.Event, len(resp.events))
	for _, e := range resp.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

type mockToolExecutor struct {
	results map[string]string
}

func (m *mockToolExecutor) Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error) {
	if result, ok := m.results[name]; ok {
		return result, false, nil
	}
	return "unknown tool", true, nil
}

func (m *mockToolExecutor) ToolDefs(agentPerms []string) []llm.Tool {
	return nil
}

func TestProcessor_BasicTextResponse(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "Hello "},
				{Type: llm.EventTextDelta, Text: "world"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID: "ses_test",
		Model:     &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &mockToolExecutor{}, b)

	result := p.Process(context.Background(), "hello")

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Aborted {
		t.Error("expected not aborted")
	}

	messages := result.Messages
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}

	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != RoleAssistant {
		t.Errorf("expected assistant role, got %s", lastMsg.Role)
	}

	var text string
	for _, part := range lastMsg.Parts {
		if part.Type == PartText {
			text = part.Text
		}
	}
	if text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", text)
	}
}

func TestProcessor_ToolCallLoop(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventToolCallBegin, ToolCallID: "call_1", ToolName: "read"},
				{Type: llm.EventToolCallDelta, ToolCallID: "call_1", ToolCallArgs: `{"file_path": "test.txt"}`},
				{Type: llm.EventToolCallEnd, ToolCallID: "call_1", ToolName: "read", ToolCallArgs: `{"file_path": "test.txt"}`},
				{Type: llm.EventFinish, FinishReason: "tool_calls"},
			}},
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "The file contains: test data"},
				{Type: llm.EventFinish, FinishReason: "stop"},
			}},
		},
	}

	tools := &mockToolExecutor{
		results: map[string]string{"read": "test data"},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID: "ses_test",
		Model:     &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, tools, b)

	result := p.Process(context.Background(), "read test.txt")

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Should have: user msg, assistant (tool call), tool result, assistant (text)
	if len(result.Messages) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(result.Messages))
	}

	lastMsg := result.Messages[len(result.Messages)-1]
	if lastMsg.Role != RoleAssistant {
		t.Errorf("expected final assistant message, got %s", lastMsg.Role)
	}
}

func TestProcessor_Abort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	client := llm.NewOpenAIClient(srv.URL, "")

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID: "ses_test",
		Model:     &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &mockToolExecutor{}, b)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := p.Process(ctx, "hello")

	if result.Error == nil && !result.Aborted {
		t.Error("expected error or abort")
	}
}

func TestProcessor_EventBusPublish(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "hi"},
				{Type: llm.EventFinish, FinishReason: "stop"},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.message")
	defer sub.Unsubscribe()

	p := NewProcessor(ProcessorConfig{
		SessionID: "ses_test",
		Model:     &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &mockToolExecutor{}, b)

	p.Process(context.Background(), "hello")

	var msgCount int
	timeout := time.After(time.Second)
	for {
		select {
		case <-sub.C:
			msgCount++
			if msgCount >= 2 {
				return
			}
		case <-timeout:
			t.Fatalf("expected at least 2 session.message events, got %d", msgCount)
		}
	}
}

func TestDefaultTitle(t *testing.T) {
	title := DefaultTitle(false)
	if !strings.HasPrefix(title, "New session - ") {
		t.Errorf("expected 'New session' prefix, got %q", title)
	}

	childTitle := DefaultTitle(true)
	if !strings.HasPrefix(childTitle, "Child session - ") {
		t.Errorf("expected 'Child session' prefix, got %q", childTitle)
	}
}
