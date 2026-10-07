package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

// switchingToolExecutor is a mockToolExecutor that also implements
// AgentSwitcher, simulating the plan_enter/plan_exit tool wiring in
// internal/tool.Registry.
type switchingToolExecutor struct {
	mockToolExecutor
	pendingTarget string
	hasPending    bool
}

func (s *switchingToolExecutor) Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error) {
	switch name {
	case "plan_enter":
		s.hasPending = true
		s.pendingTarget = "plan"
		return "Switched to plan mode.", false, nil
	case "plan_exit":
		s.hasPending = true
		s.pendingTarget = "build"
		return "Switched to build mode.", false, nil
	default:
		return s.mockToolExecutor.Execute(ctx, name, args, sessionID)
	}
}

func (s *switchingToolExecutor) TakePendingAgentSwitch() (string, bool) {
	if !s.hasPending {
		return "", false
	}
	s.hasPending = false
	return s.pendingTarget, true
}

func TestProcessor_AppliesAgentSwitchOnApprovedPlanEnter(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventToolCallBegin, ToolCallID: "call_1", ToolName: "plan_enter"},
				{Type: llm.EventToolCallDelta, ToolCallID: "call_1", ToolCallArgs: `{}`},
				{Type: llm.EventToolCallEnd, ToolCallID: "call_1", ToolName: "plan_enter", ToolCallArgs: `{}`},
				{Type: llm.EventFinish, FinishReason: "tool_calls"},
			}},
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "Now in plan mode."},
				{Type: llm.EventFinish, FinishReason: "stop"},
			}},
		},
	}

	tools := &switchingToolExecutor{}

	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.agent.switched")
	defer sub.Unsubscribe()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_plan_switch",
		Agent:           "build",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, tools, b)

	result := p.Process(context.Background(), "please plan this out")

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Agent != "plan" {
		t.Errorf("expected ProcessResult.Agent %q, got %q", "plan", result.Agent)
	}

	select {
	case evt := <-sub.C:
		payload, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any payload, got %T", evt.Properties)
		}
		if payload["agent"] != "plan" {
			t.Errorf("expected switched event agent %q, got %v", "plan", payload["agent"])
		}
	default:
		t.Fatal("expected a session.agent.switched event to be published")
	}
}

func TestProcessor_NoAgentSwitchWithoutPendingRequest(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventToolCallBegin, ToolCallID: "call_1", ToolName: "read"},
				{Type: llm.EventToolCallDelta, ToolCallID: "call_1", ToolCallArgs: `{"file_path":"test.txt"}`},
				{Type: llm.EventToolCallEnd, ToolCallID: "call_1", ToolName: "read", ToolCallArgs: `{"file_path":"test.txt"}`},
				{Type: llm.EventFinish, FinishReason: "tool_calls"},
			}},
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "done"},
				{Type: llm.EventFinish, FinishReason: "stop"},
			}},
		},
	}

	tools := &switchingToolExecutor{
		mockToolExecutor: mockToolExecutor{results: map[string]string{"read": "contents"}},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_no_switch",
		Agent:           "build",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, tools, b)

	result := p.Process(context.Background(), "read a file")

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Agent != "build" {
		t.Errorf("expected agent to remain %q, got %q", "build", result.Agent)
	}
}

func TestProcessor_PlainMockExecutorHasNoAgentSwitch(t *testing.T) {
	// mockToolExecutor does not implement AgentSwitcher; applyPendingAgentSwitch
	// must be a no-op rather than panicking via a failed type assertion.
	p := &Processor{
		config: ProcessorConfig{SessionID: "ses_plain", Agent: "build"},
		tools:  &mockToolExecutor{},
		bus:    bus.New(),
	}
	defer p.bus.Close()

	if got := p.applyPendingAgentSwitch(); got != "" {
		t.Errorf("expected no-op switch, got %q", got)
	}
}
