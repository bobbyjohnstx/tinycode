package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func TestExtractToolCalls_FiltersOnlyToolCallParts(t *testing.T) {
	msg := &Message{
		Parts: []Part{
			TextPart("some text"),
			ToolCallPart("call-1", "read", `{"path":"a.go"}`),
			ReasoningPart("thinking..."),
			ToolCallPart("call-2", "write", `{"path":"b.go"}`),
			TextPart("more text"),
			ToolResultPart("call-0", "bash", "output", false),
		},
	}

	calls := extractToolCalls(msg)

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].ToolCallID != "call-1" {
		t.Errorf("first call ID = %q, want %q", calls[0].ToolCallID, "call-1")
	}
	if calls[0].ToolName != "read" {
		t.Errorf("first call name = %q, want %q", calls[0].ToolName, "read")
	}
	if calls[1].ToolCallID != "call-2" {
		t.Errorf("second call ID = %q, want %q", calls[1].ToolCallID, "call-2")
	}
	if calls[1].ToolName != "write" {
		t.Errorf("second call name = %q, want %q", calls[1].ToolName, "write")
	}
}

func TestExtractToolCalls_EmptyMessage(t *testing.T) {
	msg := &Message{Parts: []Part{}}
	calls := extractToolCalls(msg)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for empty message, got %d", len(calls))
	}
}

func TestExtractToolCalls_NoToolCalls(t *testing.T) {
	msg := &Message{
		Parts: []Part{
			TextPart("just text"),
			ReasoningPart("just reasoning"),
		},
	}
	calls := extractToolCalls(msg)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls when no tool-call parts, got %d", len(calls))
	}
}

func TestApplyPendingAgentSwitch_NoSwitcher_ReturnsEmpty(t *testing.T) {
	// mockToolExecutor does not implement AgentSwitcher.
	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-1", Agent: "build"},
		tools:  &mockToolExecutor{},
		bus:    bus.New(),
	}
	defer p.bus.Close()

	got := p.applyPendingAgentSwitch()
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestApplyPendingAgentSwitch_NoPending_ReturnsEmpty(t *testing.T) {
	tools := &switchingToolExecutor{
		mockToolExecutor: mockToolExecutor{results: map[string]string{}},
	}

	b := bus.New()
	defer b.Close()

	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-2", Agent: "build"},
		tools:  tools,
		bus:    b,
	}

	got := p.applyPendingAgentSwitch()
	if got != "" {
		t.Errorf("expected empty string when no pending switch, got %q", got)
	}
}

func TestApplyPendingAgentSwitch_PlanEnter_ReturnsNewAgent(t *testing.T) {
	tools := &switchingToolExecutor{}

	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.agent.switched")
	defer sub.Unsubscribe()

	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-3", Agent: "build"},
		tools:  tools,
		bus:    b,
	}

	// Simulate plan_enter having set a pending switch.
	tools.hasPending = true
	tools.pendingTarget = "plan"

	got := p.applyPendingAgentSwitch()
	if got != "plan" {
		t.Errorf("expected %q, got %q", "plan", got)
	}

	// Verify the config was updated.
	if p.config.Agent != "plan" {
		t.Errorf("config.Agent = %q, want %q", p.config.Agent, "plan")
	}

	// Verify bus event.
	select {
	case evt := <-sub.C:
		payload := evt.Properties.(map[string]any)
		if payload["agent"] != "plan" {
			t.Errorf("event agent = %v, want %q", payload["agent"], "plan")
		}
	default:
		t.Error("expected session.agent.switched event")
	}
}

func TestApplyPendingAgentSwitch_PlanExit_ReturnsNewAgent(t *testing.T) {
	tools := &switchingToolExecutor{}

	b := bus.New()
	defer b.Close()

	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-4", Agent: "plan"},
		tools:  tools,
		bus:    b,
	}

	tools.hasPending = true
	tools.pendingTarget = "build"

	got := p.applyPendingAgentSwitch()
	if got != "build" {
		t.Errorf("expected %q, got %q", "build", got)
	}
	if p.config.Agent != "build" {
		t.Errorf("config.Agent = %q, want %q", p.config.Agent, "build")
	}
}

func TestApplyPendingAgentSwitch_SameAgent_ReturnsEmpty(t *testing.T) {
	tools := &switchingToolExecutor{}

	b := bus.New()
	defer b.Close()

	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-5", Agent: "plan"},
		tools:  tools,
		bus:    b,
	}

	// Pending switch to the same agent.
	tools.hasPending = true
	tools.pendingTarget = "plan"

	got := p.applyPendingAgentSwitch()
	if got != "" {
		t.Errorf("expected empty string for same-agent switch, got %q", got)
	}
}

func TestExecuteTools_CancelledContext_AbandonsPending(t *testing.T) {
	// Create a tool executor that blocks until context is cancelled.
	blocker := &blockingToolExecutor{
		block: make(chan struct{}),
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-cancel",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, blocker, b)

	toolCalls := []Part{
		ToolCallPart("call-1", "slow_tool", `{}`),
		ToolCallPart("call-2", "slow_tool", `{}`),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	results, allFailed := p.executeTools(ctx, toolCalls)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// At least one should be "aborted".
	abortedCount := 0
	for _, r := range results {
		if r.ToolResult == "aborted" {
			abortedCount++
		}
	}
	if abortedCount == 0 {
		t.Error("expected at least one aborted tool result")
	}
	_ = allFailed // may or may not be true depending on timing
}

func TestExecuteTools_ErrorPath_ReturnsErrorResult(t *testing.T) {
	tools := &errorToolExecutor{
		err: "permission denied",
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-err",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, tools, b)

	toolCalls := []Part{
		ToolCallPart("call-1", "read", `{"path":"secret.txt"}`),
	}

	results, allFailed := p.executeTools(context.Background(), toolCalls)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !allFailed {
		t.Error("expected allFailed=true when tool returns error")
	}
	if results[0].ToolError != true {
		t.Error("expected ToolError=true for error result")
	}
	if results[0].ToolResult != "permission denied" {
		t.Errorf("ToolResult = %q, want %q", results[0].ToolResult, "permission denied")
	}
}

func TestExecuteTools_SuccessPath_ReturnsOutput(t *testing.T) {
	tools := &mockToolExecutor{
		results: map[string]string{"read": "file contents here"},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-success",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, tools, b)

	toolCalls := []Part{
		ToolCallPart("call-1", "read", `{"path":"a.go"}`),
	}

	results, allFailed := p.executeTools(context.Background(), toolCalls)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if allFailed {
		t.Error("expected allFailed=false for successful tool")
	}
	if results[0].ToolResult != "file contents here" {
		t.Errorf("ToolResult = %q, want %q", results[0].ToolResult, "file contents here")
	}
	if results[0].ToolCallID != "call-1" {
		t.Errorf("ToolCallID = %q, want %q", results[0].ToolCallID, "call-1")
	}
}

func TestExecuteTools_MultipleTools_PreservesOrder(t *testing.T) {
	tools := &mockToolExecutor{
		results: map[string]string{
			"read":  "content-a",
			"write": "content-b",
			"bash":  "content-c",
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-order",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, tools, b)

	toolCalls := []Part{
		ToolCallPart("call-1", "read", `{}`),
		ToolCallPart("call-2", "write", `{}`),
		ToolCallPart("call-3", "bash", `{}`),
	}

	results, _ := p.executeTools(context.Background(), toolCalls)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results should be in the same order as toolCalls.
	expectedIDs := []string{"call-1", "call-2", "call-3"}
	expectedResults := []string{"content-a", "content-b", "content-c"}

	for i, r := range results {
		if r.ToolCallID != expectedIDs[i] {
			t.Errorf("result[%d] ToolCallID = %q, want %q", i, r.ToolCallID, expectedIDs[i])
		}
		if r.ToolResult != expectedResults[i] {
			t.Errorf("result[%d] ToolResult = %q, want %q", i, r.ToolResult, expectedResults[i])
		}
	}
}

func TestPublishToolResult_NilBus_NoPanic(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-nilbus"},
		bus:    nil,
	}

	// Should not panic with nil bus.
	p.publishToolResult(
		ToolCallPart("call-1", "read", `{}`),
		"output",
		false,
	)
}

func TestPublishToolResult_PublishesEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.tool.result")
	defer sub.Unsubscribe()

	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-pub"},
		bus:    b,
	}

	call := ToolCallPart("call-1", "read", `{"path":"test.go"}`)
	p.publishToolResult(call, "file contents", false)

	select {
	case evt := <-sub.C:
		payload := evt.Properties.(map[string]any)
		if payload["sessionID"] != "ses-pub" {
			t.Errorf("sessionID = %v, want %q", payload["sessionID"], "ses-pub")
		}
		if payload["toolCallID"] != "call-1" {
			t.Errorf("toolCallID = %v, want %q", payload["toolCallID"], "call-1")
		}
		if payload["toolName"] != "read" {
			t.Errorf("toolName = %v, want %q", payload["toolName"], "read")
		}
		if payload["output"] != "file contents" {
			t.Errorf("output = %v, want %q", payload["output"], "file contents")
		}
		if payload["isError"] != false {
			t.Errorf("isError = %v, want false", payload["isError"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session.tool.result event")
	}
}

func TestPublishToolResult_ErrorResult(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.tool.result")
	defer sub.Unsubscribe()

	p := &Processor{
		config: ProcessorConfig{SessionID: "ses-pub-err"},
		bus:    b,
	}

	call := ToolCallPart("call-err", "bash", `{"command":"rm -rf /"}`)
	p.publishToolResult(call, "permission denied", true)

	select {
	case evt := <-sub.C:
		payload := evt.Properties.(map[string]any)
		if payload["isError"] != true {
			t.Errorf("isError = %v, want true", payload["isError"])
		}
		if payload["output"] != "permission denied" {
			t.Errorf("output = %v, want %q", payload["output"], "permission denied")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

// --- Test helper types ---

// blockingToolExecutor blocks until its channel is closed or context is cancelled.
type blockingToolExecutor struct {
	block chan struct{}
}

func (b *blockingToolExecutor) Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error) {
	select {
	case <-b.block:
		return "done", false, nil
	case <-ctx.Done():
		return "", false, ctx.Err()
	}
}

func (b *blockingToolExecutor) ToolDefs(_ []string) []llm.Tool {
	return nil
}

// errorToolExecutor always returns an error from Execute.
type errorToolExecutor struct {
	err string
}

func (e *errorToolExecutor) Execute(_ context.Context, _ string, _ json.RawMessage, _ string) (string, bool, error) {
	return e.err, true, nil
}

func (e *errorToolExecutor) ToolDefs(_ []string) []llm.Tool {
	return nil
}
