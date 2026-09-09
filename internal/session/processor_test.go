package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

type stubToolExecutor struct{}

func (s *stubToolExecutor) Execute(_ context.Context, _ string, _ json.RawMessage, _ string) (string, bool, error) {
	return "", false, nil
}

func (s *stubToolExecutor) ToolDefs(_ []string) []llm.Tool {
	return nil
}

func TestBuildRequest_ParallelToolResults(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID:    "ses-test",
			SystemPrompt: "You are a test assistant.",
			Model:        &provider.Model{ID: "test-model"},
		},
		tools: &stubToolExecutor{},
		bus:   bus.New(),
	}

	p.messages = []Message{
		{
			ID:        "msg-1",
			SessionID: "ses-test",
			Role:      RoleUser,
			Parts:     []Part{TextPart("Do three things")},
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-2",
			SessionID: "ses-test",
			Role:      RoleAssistant,
			Parts: []Part{
				ToolCallPart("call-1", "read", `{"path":"a.go"}`),
				ToolCallPart("call-2", "read", `{"path":"b.go"}`),
				ToolCallPart("call-3", "read", `{"path":"c.go"}`),
			},
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-3",
			SessionID: "ses-test",
			Role:      RoleTool,
			Parts: []Part{
				ToolResultPart("call-1", "read", "contents of a.go", false),
				ToolResultPart("call-2", "read", "contents of b.go", false),
				ToolResultPart("call-3", "read", "contents of c.go", false),
			},
			CreatedAt: time.Now(),
		},
	}

	req := p.buildRequest()

	// Expected messages: system + user + assistant + 3 tool results = 6
	if len(req.Messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(req.Messages))
	}

	// Verify system message
	if req.Messages[0].Role != "system" {
		t.Errorf("expected message[0] role 'system', got %q", req.Messages[0].Role)
	}

	// Verify user message
	if req.Messages[1].Role != "user" {
		t.Errorf("expected message[1] role 'user', got %q", req.Messages[1].Role)
	}

	// Verify assistant message with 3 tool calls
	assistantMsg := req.Messages[2]
	if assistantMsg.Role != "assistant" {
		t.Errorf("expected message[2] role 'assistant', got %q", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) != 3 {
		t.Fatalf("expected 3 tool calls on assistant message, got %d", len(assistantMsg.ToolCalls))
	}

	// Verify each tool result is a separate message with correct ToolCallID
	expectedToolIDs := []string{"call-1", "call-2", "call-3"}
	expectedContents := []string{"contents of a.go", "contents of b.go", "contents of c.go"}

	for i, idx := range []int{3, 4, 5} {
		msg := req.Messages[idx]
		if msg.Role != "tool" {
			t.Errorf("expected message[%d] role 'tool', got %q", idx, msg.Role)
		}
		if msg.ToolCallID != expectedToolIDs[i] {
			t.Errorf("expected message[%d] ToolCallID %q, got %q", idx, expectedToolIDs[i], msg.ToolCallID)
		}
		content, ok := msg.Content.(string)
		if !ok {
			t.Errorf("expected message[%d] Content to be string, got %T", idx, msg.Content)
			continue
		}
		if content != expectedContents[i] {
			t.Errorf("expected message[%d] Content %q, got %q", idx, expectedContents[i], content)
		}
	}
}

func TestBuildRequest_SingleToolResult(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Model:     &provider.Model{ID: "test-model"},
		},
		tools: &stubToolExecutor{},
		bus:   bus.New(),
	}

	p.messages = []Message{
		{
			ID:        "msg-1",
			SessionID: "ses-test",
			Role:      RoleUser,
			Parts:     []Part{TextPart("Do one thing")},
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-2",
			SessionID: "ses-test",
			Role:      RoleAssistant,
			Parts: []Part{
				ToolCallPart("call-1", "read", `{"path":"a.go"}`),
			},
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-3",
			SessionID: "ses-test",
			Role:      RoleTool,
			Parts: []Part{
				ToolResultPart("call-1", "read", "file contents", false),
			},
			CreatedAt: time.Now(),
		},
	}

	req := p.buildRequest()

	// Expected: user + assistant + 1 tool result = 3 (no system prompt)
	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(req.Messages))
	}

	toolMsg := req.Messages[2]
	if toolMsg.Role != "tool" {
		t.Errorf("expected role 'tool', got %q", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "call-1" {
		t.Errorf("expected ToolCallID 'call-1', got %q", toolMsg.ToolCallID)
	}
}

// Verify assistant messages preserve multiple tool calls in a single message.
func TestBuildRequest_AssistantToolCalls(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Model:     &provider.Model{ID: "test-model"},
		},
		tools: &stubToolExecutor{},
		bus:   bus.New(),
	}

	p.messages = []Message{
		{
			ID:        "msg-1",
			SessionID: "ses-test",
			Role:      RoleUser,
			Parts:     []Part{TextPart("Hello")},
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-2",
			SessionID: "ses-test",
			Role:      RoleAssistant,
			Parts: []Part{
				ToolCallPart("tc-a", "bash", `{"cmd":"ls"}`),
				ToolCallPart("tc-b", "read", `{"path":"x"}`),
			},
			CreatedAt: time.Now(),
		},
	}

	req := p.buildRequest()

	// user + assistant = 2
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	assistant := req.Messages[1]
	if len(assistant.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].ID != "tc-a" {
		t.Errorf("expected first tool call ID 'tc-a', got %q", assistant.ToolCalls[0].ID)
	}
	if assistant.ToolCalls[1].ID != "tc-b" {
		t.Errorf("expected second tool call ID 'tc-b', got %q", assistant.ToolCalls[1].ID)
	}
}

// filteringToolExecutor returns different tool sets based on agentPerms.
type filteringToolExecutor struct{}

func (f *filteringToolExecutor) Execute(_ context.Context, _ string, _ json.RawMessage, _ string) (string, bool, error) {
	return "", false, nil
}

func (f *filteringToolExecutor) ToolDefs(agentPerms []string) []llm.Tool {
	allTools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read"}},
		{Type: "function", Function: llm.ToolFunction{Name: "write"}},
		{Type: "function", Function: llm.ToolFunction{Name: "edit"}},
		{Type: "function", Function: llm.ToolFunction{Name: "grep"}},
		{Type: "function", Function: llm.ToolFunction{Name: "bash"}},
	}
	if len(agentPerms) == 0 {
		return allTools
	}
	allowed := make(map[string]bool, len(agentPerms))
	for _, p := range agentPerms {
		allowed[p] = true
	}
	var filtered []llm.Tool
	for _, t := range allTools {
		if allowed[t.Function.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func TestBuildRequest_AgentPermsFilter(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID:    "ses-perms",
			Model:        &provider.Model{ID: "test-model"},
			SystemPrompt: "Test",
			AgentPerms:   []string{"read", "grep"},
		},
		tools: &filteringToolExecutor{},
		bus:   bus.New(),
	}

	p.messages = []Message{
		{
			ID:        "msg-1",
			SessionID: "ses-perms",
			Role:      RoleUser,
			Parts:     []Part{TextPart("Hello")},
			CreatedAt: time.Now(),
		},
	}

	req := p.buildRequest()

	if len(req.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(req.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range req.Tools {
		toolNames[tool.Function.Name] = true
	}

	if !toolNames["read"] {
		t.Error("expected 'read' tool to be present")
	}
	if !toolNames["grep"] {
		t.Error("expected 'grep' tool to be present")
	}
	if toolNames["write"] {
		t.Error("'write' tool should not be present")
	}
	if toolNames["edit"] {
		t.Error("'edit' tool should not be present")
	}
	if toolNames["bash"] {
		t.Error("'bash' tool should not be present")
	}
}

func TestBuildRequest_NoAgentPermsReturnsAll(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-no-perms",
			Model:     &provider.Model{ID: "test-model"},
		},
		tools: &filteringToolExecutor{},
		bus:   bus.New(),
	}

	p.messages = []Message{
		{
			ID:        "msg-1",
			SessionID: "ses-no-perms",
			Role:      RoleUser,
			Parts:     []Part{TextPart("Hello")},
			CreatedAt: time.Now(),
		},
	}

	req := p.buildRequest()

	if len(req.Tools) != 5 {
		t.Fatalf("expected 5 tools (all), got %d", len(req.Tools))
	}
}

func TestBuildRequest_TemperatureTopPMaxTokens(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTok := 4096
	p := &Processor{
		config: ProcessorConfig{
			SessionID:   "ses-params",
			Model:       &provider.Model{ID: "test-model"},
			Temperature: &temp,
			TopP:        &topP,
			MaxTokens:   &maxTok,
		},
		tools: &stubToolExecutor{},
		bus:   bus.New(),
	}

	p.messages = []Message{
		{ID: "msg-1", SessionID: "ses-params", Role: RoleUser, Parts: []Part{TextPart("Hello")}, CreatedAt: time.Now()},
	}

	req := p.buildRequest()

	if req.Temperature == nil || *req.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", req.Temperature)
	}
	if req.TopP == nil || *req.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %v", req.TopP)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 4096 {
		t.Errorf("expected max_tokens 4096, got %v", req.MaxTokens)
	}
}

// --- Issue #111: Doom-loop detection ---

func TestIsDoomLoop_Detected(t *testing.T) {
	sigs := []toolCallSignature{
		{Name: "read", Args: `{"path":"a.go"}`},
		{Name: "read", Args: `{"path":"a.go"}`},
		{Name: "read", Args: `{"path":"a.go"}`},
	}
	if !isDoomLoop(sigs, 3) {
		t.Error("expected doom loop to be detected")
	}
}

func TestIsDoomLoop_NotDetected_DifferentCalls(t *testing.T) {
	sigs := []toolCallSignature{
		{Name: "read", Args: `{"path":"a.go"}`},
		{Name: "read", Args: `{"path":"b.go"}`},
		{Name: "read", Args: `{"path":"a.go"}`},
	}
	if isDoomLoop(sigs, 3) {
		t.Error("expected no doom loop with different args")
	}
}

func TestIsDoomLoop_NotDetected_TooFew(t *testing.T) {
	sigs := []toolCallSignature{
		{Name: "read", Args: `{"path":"a.go"}`},
		{Name: "read", Args: `{"path":"a.go"}`},
	}
	if isDoomLoop(sigs, 3) {
		t.Error("expected no doom loop with fewer calls than threshold")
	}
}

func TestIsDoomLoop_DifferentNames(t *testing.T) {
	sigs := []toolCallSignature{
		{Name: "read", Args: `{"path":"a.go"}`},
		{Name: "write", Args: `{"path":"a.go"}`},
		{Name: "read", Args: `{"path":"a.go"}`},
	}
	if isDoomLoop(sigs, 3) {
		t.Error("expected no doom loop with different tool names")
	}
}

// --- Issue #111: Auto-continue ---

func TestProcessor_AutoContinue(t *testing.T) {
	// Scenario: tool call -> text response (triggers auto-continue) -> final text
	client := &mockLLMClient{
		responses: []mockResponse{
			// First call: tool call
			{events: []llm.Event{
				{Type: llm.EventToolCallBegin, ToolCallID: "call_1", ToolName: "read"},
				{Type: llm.EventToolCallEnd, ToolCallID: "call_1", ToolName: "read", ToolCallArgs: `{"path":"a.go"}`},
				{Type: llm.EventFinish, FinishReason: "tool_calls"},
			}},
			// Second call: text only (triggers auto-continue since prior had tool calls)
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "I read the file."},
				{Type: llm.EventFinish, FinishReason: "stop"},
			}},
			// Third call (after nudge): final text
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "All done."},
				{Type: llm.EventFinish, FinishReason: "stop"},
			}},
		},
	}

	tools := &mockToolExecutor{
		results: map[string]string{"read": "file contents"},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_autocont",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: 1, // allow 1 auto-continue
	}, client, tools, b)

	result := p.Process(context.Background(), "read file")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Expect: user, assistant(tool), tool-result, assistant(text), user(nudge), assistant(final)
	if len(result.Messages) < 6 {
		t.Fatalf("expected at least 6 messages with auto-continue, got %d", len(result.Messages))
	}

	// Find the nudge message
	foundNudge := false
	for _, msg := range result.Messages {
		if msg.Role == RoleUser {
			for _, part := range msg.Parts {
				if part.Type == PartText && part.Text == "Continue with your next step." {
					foundNudge = true
				}
			}
		}
	}
	if !foundNudge {
		t.Error("expected auto-continue nudge message")
	}
}

// --- Issue #114: External directory detection ---

func TestIsInsideDirectory(t *testing.T) {
	tests := []struct {
		path    string
		dir     string
		inside  bool
	}{
		{"/project/src/main.go", "/project", true},
		{"/project/src/../src/main.go", "/project", true},
		{"/other/file.go", "/project", false},
		{"/project", "/project", true},
		{"/projectx/file.go", "/project", false},
	}

	for _, tt := range tests {
		got := isInsideDirectory(tt.path, tt.dir)
		if got != tt.inside {
			t.Errorf("isInsideDirectory(%q, %q) = %v, want %v", tt.path, tt.dir, got, tt.inside)
		}
	}
}

func TestExtractPathsFromArgs(t *testing.T) {
	args := `{"file_path": "/tmp/test.go", "other": "value"}`
	paths := extractPathsFromArgs(args)
	if len(paths) != 1 || paths[0] != "/tmp/test.go" {
		t.Errorf("expected [/tmp/test.go], got %v", paths)
	}

	args2 := `{"path": "/tmp/a.go", "directory": "/tmp/dir"}`
	paths2 := extractPathsFromArgs(args2)
	if len(paths2) != 2 {
		t.Errorf("expected 2 paths, got %d: %v", len(paths2), paths2)
	}

	empty := extractPathsFromArgs(`{"command": "ls"}`)
	if len(empty) != 0 {
		t.Errorf("expected no paths, got %v", empty)
	}
}

// --- Issue #128: Proactive overflow detection ---

func TestProcessor_ProactiveCompaction(t *testing.T) {
	// Simulate high token usage that should trigger proactive compaction.
	// The model has context=100000, output=4096.
	// Reserve = max(20000, 4096) = 20000, threshold = 80000.
	// We'll report usage of 85000 input tokens.
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
		SessionID:       "ses_overflow",
		Model:           &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, &stubToolExecutor{}, b)

	// The proactive compaction will try to compact, but with only 1 message
	// it will be a no-op (FindPreserveBoundary returns 0). That's fine - we
	// just verify no crash and the process completes.
	result := p.Process(context.Background(), "test query")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestProcessor_BelowThreshold_NoCompaction(t *testing.T) {
	// Usage below threshold should not trigger compaction.
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "response"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 50000, CompletionTokens: 100}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_no_overflow",
		Model:           &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, &stubToolExecutor{}, b)

	result := p.Process(context.Background(), "test query")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if p.compactionCount != 0 {
		t.Errorf("expected no compaction, but compactionCount=%d", p.compactionCount)
	}
}
