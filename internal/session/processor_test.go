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

