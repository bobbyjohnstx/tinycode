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

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"

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
		Model:     &ModelRef{ModelID: "llama3.2", ProviderID: "ollama"},
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
	if got.Model.ModelID != "llama3.2" {
		t.Errorf("expected model id 'llama3.2', got %q", got.Model.ModelID)
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

func TestSnapshotDiff_JSONSerialization(t *testing.T) {
	diffs := []SnapshotDiff{
		{File: "main.go", Additions: 10, Deletions: 2, Status: "modified"},
		{File: "new.go", Additions: 5, Deletions: 0, Status: "added"},
	}

	data, err := json.Marshal(diffs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded []SnapshotDiff
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(decoded))
	}
	if decoded[0].File != "main.go" {
		t.Errorf("expected file main.go, got %q", decoded[0].File)
	}
	if decoded[0].Additions != 10 || decoded[0].Deletions != 2 {
		t.Errorf("unexpected additions/deletions: %+v", decoded[0])
	}
	if decoded[1].Status != "added" {
		t.Errorf("expected status added, got %q", decoded[1].Status)
	}
}

func TestStore_UpdateSummaryWithDiffs(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})

	diffs := []SnapshotDiff{
		{File: "a.go", Additions: 3, Deletions: 1, Status: "modified"},
	}
	store.UpdateSummary(info.ID, Summary{
		Additions: 3,
		Deletions: 1,
		Files:     1,
		Diffs:     diffs,
	})

	got, err := store.Get(info.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("expected summary")
	}
	if len(got.Summary.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(got.Summary.Diffs))
	}
	if got.Summary.Diffs[0].File != "a.go" {
		t.Errorf("expected file a.go, got %q", got.Summary.Diffs[0].File)
	}
	if got.Summary.Diffs[0].Additions != 3 {
		t.Errorf("expected 3 additions, got %d", got.Summary.Diffs[0].Additions)
	}
}

func TestSummary_DiffsSerializesAsArray(t *testing.T) {
	s := Summary{Additions: 1, Deletions: 0, Files: 1, Diffs: []SnapshotDiff{}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["diffs"]; !ok {
		t.Fatal("expected diffs key")
	}
	arr, ok := raw["diffs"].([]any)
	if !ok {
		t.Fatalf("expected diffs to be array, got %T", raw["diffs"])
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %v", arr)
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

func TestMessageStore_DeleteAfterTime(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	ses, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})

	// Create messages with incrementing timestamps.
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ms.Append(&Message{
			ID:        fmt.Sprintf("msg_%d", i),
			SessionID: ses.ID,
			Role:      RoleUser,
			Parts:     []Part{TextPart(fmt.Sprintf("message %d", i))},
			CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
		})
	}

	// Delete messages after msg_2 (timestamp = baseTime + 2min).
	cutoffMs := baseTime.Add(2 * time.Minute).UnixMilli()
	deleted, err := ms.DeleteAfterTime(ses.ID, cutoffMs)
	if err != nil {
		t.Fatalf("delete after time: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	remaining, err := ms.List(ses.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected 3 remaining messages, got %d", len(remaining))
	}

	// Verify the correct messages remain.
	for i, msg := range remaining {
		expected := fmt.Sprintf("msg_%d", i)
		if msg.ID != expected {
			t.Errorf("remaining[%d].ID = %q, want %q", i, msg.ID, expected)
		}
	}
}

func TestMessageStore_DeleteAfterTime_NoneDeleted(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	ses, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})

	now := time.Now()
	ms.Append(&Message{
		ID: "msg_0", SessionID: ses.ID, Role: RoleUser,
		Parts: []Part{TextPart("hello")}, CreatedAt: now,
	})

	// Cutoff after all messages — nothing to delete.
	deleted, err := ms.DeleteAfterTime(ses.ID, now.Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatalf("delete after time: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}

	count, _ := ms.Count(ses.ID)
	if count != 1 {
		t.Errorf("expected 1 message remaining, got %d", count)
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
	// With only 1 result, it is within the 5-most-recent preserve window,
	// so it should NOT be masked.
	messages := []Message{
		{Parts: []Part{
			{Type: PartToolResult, ToolCallID: "c1", ToolName: "read", ToolResult: "file contents here"},
		}},
	}

	masked := maskObservations(messages)
	if masked[0].Parts[0].ToolResult != "file contents here" {
		t.Errorf("expected preserved output (within recent window), got %q", masked[0].Parts[0].ToolResult)
	}
	if masked[0].Parts[0].ToolCallID != "c1" {
		t.Error("expected tool call ID preserved")
	}
	// Verify original not mutated
	if messages[0].Parts[0].ToolResult != "file contents here" {
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

func TestMaskObservations_PreserveRecent(t *testing.T) {
	// Create 8 tool results. With preserveRecentOutputs=5, the first 3
	// should be masked and the last 5 should be preserved.
	var messages []Message
	for i := 0; i < 8; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart("c"+string(rune('0'+i)), "read", "output"+string(rune('0'+i)), false),
			},
		})
	}

	masked := maskObservations(messages)

	// First 3 should be masked
	for i := 0; i < 3; i++ {
		if masked[i].Parts[0].ToolResult != "[output masked for compaction]" {
			t.Errorf("message %d should be masked, got %q", i, masked[i].Parts[0].ToolResult)
		}
	}

	// Last 5 should be preserved
	for i := 3; i < 8; i++ {
		expected := "output" + string(rune('0'+i))
		if masked[i].Parts[0].ToolResult != expected {
			t.Errorf("message %d should be preserved as %q, got %q", i, expected, masked[i].Parts[0].ToolResult)
		}
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if truncate(short, 100) != "hello" {
		t.Error("short string should not be truncated")
	}

	long := strings.Repeat("x", 3000)
	result := truncate(long, 2000)
	if len(result) > 2020 { // 2000 + "... [truncated]"
		t.Errorf("truncated string too long: %d chars", len(result))
	}
	if !strings.HasSuffix(result, "... [truncated]") {
		t.Error("expected truncation marker")
	}
}

func TestBuildCompactionPrompt_TruncatesLongContent(t *testing.T) {
	longText := strings.Repeat("x", 5000)
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart(longText)}},
	}

	prompt := buildCompactionPrompt(messages, "", nil, nil)
	// The long text should be truncated to maxTextChars + marker
	if strings.Contains(prompt, longText) {
		t.Error("expected long text to be truncated")
	}
	if !strings.Contains(prompt, "... [truncated]") {
		t.Error("expected truncation marker in prompt")
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
		SessionID:       "ses_test",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1, // disable auto-continue for this test
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

func TestModelRef_MarshalJSON_UsesID(t *testing.T) {
	m := ModelRef{ModelID: "x", ProviderID: "p"}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["id"] != "x" {
		t.Errorf("expected id=x, got %v", raw["id"])
	}
	if _, ok := raw["modelID"]; ok {
		t.Error("expected no modelID key in marshalled output")
	}
}

func TestModelRef_UnmarshalJSON_LegacyModelID(t *testing.T) {
	var m ModelRef
	err := json.Unmarshal([]byte(`{"modelID":"x","providerID":"p"}`), &m)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.ModelID != "x" {
		t.Errorf("expected ModelID=x from legacy modelID, got %q", m.ModelID)
	}
	if m.ProviderID != "p" {
		t.Errorf("expected ProviderID=p, got %q", m.ProviderID)
	}
}

func TestModelRef_UnmarshalJSON_NewID(t *testing.T) {
	var m ModelRef
	err := json.Unmarshal([]byte(`{"id":"y","providerID":"q"}`), &m)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.ModelID != "y" {
		t.Errorf("expected ModelID=y, got %q", m.ModelID)
	}
}

func TestModelRef_UnmarshalJSON_NewIDTakesPrecedence(t *testing.T) {
	var m ModelRef
	err := json.Unmarshal([]byte(`{"id":"new","modelID":"old","providerID":"p"}`), &m)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.ModelID != "new" {
		t.Errorf("expected id to take precedence over modelID, got %q", m.ModelID)
	}
}

func TestTimeInfo_ArchivedOmitEmpty(t *testing.T) {
	ti := TimeInfo{Created: 1000, Updated: 2000}
	data, err := json.Marshal(ti)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "archived") {
		t.Errorf("expected archived omitted when zero, got %s", s)
	}
}

func TestTimeInfo_ArchivedPresent(t *testing.T) {
	ti := TimeInfo{Created: 1000, Updated: 2000, Archived: 123}
	data, err := json.Marshal(ti)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["archived"] != float64(123) {
		t.Errorf("expected archived=123, got %v", raw["archived"])
	}
}

func TestStore_ArchiveSyncsTimeInfo(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	info, _ := store.Create(CreateInput{ProjectID: "proj-1", Directory: "/tmp"})
	store.Archive(info.ID)

	got, _ := store.Get(info.ID)
	if got.Time.Archived == 0 {
		t.Error("expected Time.Archived to be populated after archive")
	}
	if got.TimeArchived == 0 {
		t.Error("expected TimeArchived to be populated after archive")
	}
	if got.Time.Archived != got.TimeArchived {
		t.Errorf("Time.Archived (%d) should match TimeArchived (%d)", got.Time.Archived, got.TimeArchived)
	}
}

// --- Elision tests ---

func TestElideOldResults_MasksToolResults(t *testing.T) {
	p := &Processor{}
	var messages []Message
	for i := 0; i < 10; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart(fmt.Sprintf("c%d", i), "read", fmt.Sprintf("output%d", i), false),
			},
		})
	}
	p.messages = messages

	p.elideOldResults()

	// First 5 should be masked (10 - preserveRecentOutputs = 5)
	for i := 0; i < 5; i++ {
		if p.messages[i].Parts[0].ToolResult != "[output masked for compaction]" {
			t.Errorf("message %d should be masked, got %q", i, p.messages[i].Parts[0].ToolResult)
		}
	}
	// Last 5 should be preserved
	for i := 5; i < 10; i++ {
		expected := fmt.Sprintf("output%d", i)
		if p.messages[i].Parts[0].ToolResult != expected {
			t.Errorf("message %d should be preserved as %q, got %q", i, expected, p.messages[i].Parts[0].ToolResult)
		}
	}
}

func TestElideOldResults_Idempotent(t *testing.T) {
	p := &Processor{}
	var messages []Message
	for i := 0; i < 10; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart(fmt.Sprintf("c%d", i), "read", fmt.Sprintf("output%d", i), false),
			},
		})
	}
	p.messages = messages

	p.elideOldResults()
	afterFirst := make([]Message, len(p.messages))
	copy(afterFirst, p.messages)

	p.elideOldResults()

	for i := range afterFirst {
		if p.messages[i].Parts[0].ToolResult != afterFirst[i].Parts[0].ToolResult {
			t.Errorf("message %d changed on second call: %q vs %q",
				i, afterFirst[i].Parts[0].ToolResult, p.messages[i].Parts[0].ToolResult)
		}
	}
}

func TestElideOldResults_ResetOnNewProcess(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "ok"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_elision_reset",
		Model:      &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &mockToolExecutor{}, b)

	p.elisionDone = true

	p.Process(context.Background(), "hello")

	if p.elisionDone {
		t.Error("expected elisionDone to be reset after ProcessWithID")
	}
}

func TestCheckCompaction_SoftThreshold(t *testing.T) {
	// Model: context=100000, output=4096
	// compactionOutputReserve keeps the 20k reserve when the window can afford it.
	// threshold = 80000
	// softThreshold = 64000
	// Usage at 70000 => above soft, below hard => elision only
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "response"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 70000, CompletionTokens: 100}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_soft",
		Model:           &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, &stubToolExecutor{}, b)

	// Add some tool results so elision has something to mask
	var messages []Message
	for i := 0; i < 10; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart(fmt.Sprintf("c%d", i), "read", fmt.Sprintf("output%d", i), false),
			},
		})
	}
	p.SetMessages(messages)

	result := p.Process(context.Background(), "test query")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Elision should have fired
	if !p.elisionDone {
		t.Error("expected elisionDone to be true after soft threshold")
	}

	// compactionCount should NOT have incremented (no LLM compaction)
	if p.compactionCount != 0 {
		t.Errorf("expected compactionCount=0, got %d", p.compactionCount)
	}
}

func TestCheckCompaction_HardThreshold_SkipsElision(t *testing.T) {
	// Usage at 85000 (above threshold of 80000) => hard compaction, not elision
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "response"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 85000, CompletionTokens: 100}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_hard",
		Model:           &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, &stubToolExecutor{}, b)

	result := p.Process(context.Background(), "test query")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Elision should NOT have fired (usage >= threshold, not in soft range)
	if p.elisionDone {
		t.Error("expected elisionDone to be false at hard threshold")
	}
}
