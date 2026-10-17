package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
	for i, r := range results {
		if r.Type == "" {
			t.Errorf("result %d has empty type; abort should wait for workers or fill aborted", i)
		}
	}
	if !allFailed {
		t.Error("expected allFailed=true when tools are cancelled")
	}
}

func TestExecuteTools_CapsConcurrentCalls(t *testing.T) {
	exec := &countingToolExecutor{
		started: make(chan struct{}, 32),
		release: make(chan struct{}),
	}
	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-cap",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, exec, b)

	const calls = maxConcurrentTools + 4
	toolCalls := make([]Part, calls)
	for i := range toolCalls {
		toolCalls[i] = ToolCallPart("call-"+string(rune('a'+i)), "slow", `{}`)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		for i := 0; i < maxConcurrentTools; i++ {
			select {
			case <-exec.started:
			case <-ctx.Done():
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
		exec.mu.Lock()
		exec.observed = exec.current
		exec.observedMax = exec.maxSeen
		exec.mu.Unlock()
		close(exec.release)
	}()

	results, allFailed := p.executeTools(ctx, toolCalls)
	if allFailed {
		t.Fatal("expected capped tools to finish")
	}
	if len(results) != calls {
		t.Fatalf("results = %d, want %d", len(results), calls)
	}
	if exec.observed != maxConcurrentTools {
		t.Errorf("in flight when the cap filled = %d, want %d", exec.observed, maxConcurrentTools)
	}
	if exec.observedMax > maxConcurrentTools {
		t.Errorf("max in flight = %d, want at most %d", exec.observedMax, maxConcurrentTools)
	}
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

func TestExecuteTools_Panic_PublishesOneBusEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("session.tool.result")
	defer sub.Unsubscribe()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-panic-bus",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, &panicToolExecutor{}, b)

	toolCalls := []Part{
		ToolCallPart("call-1", "read", `{"path":"a.go"}`),
	}

	results, allFailed := p.executeTools(context.Background(), toolCalls)
	if !allFailed {
		t.Error("expected allFailed=true for panicking tool")
	}
	if len(results) != 1 || !results[0].ToolError {
		t.Fatalf("expected one failed tool result, got %+v", results)
	}
	if results[0].ToolResult != "tool panicked: unexpected nil pointer in read" {
		t.Errorf("ToolResult = %q, want exact panic message", results[0].ToolResult)
	}

	var events int
	deadline := time.After(2 * time.Second)
collect:
	for {
		select {
		case evt := <-sub.C:
			events++
			payload, ok := evt.Properties.(map[string]any)
			if !ok {
				t.Fatalf("unexpected payload type %T", evt.Properties)
			}
			if payload["isError"] != true {
				t.Errorf("isError = %v, want true", payload["isError"])
			}
			// Drain any extras briefly; exactly one publish is required.
			select {
			case <-sub.C:
				events++
			case <-time.After(50 * time.Millisecond):
				break collect
			}
		case <-deadline:
			t.Fatal("timed out waiting for session.tool.result event")
		}
	}
	if events != 1 {
		t.Fatalf("session.tool.result events = %d, want 1", events)
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

func TestExecuteTools_SerializesQuestionByCallIndex(t *testing.T) {
	exec := &orderingToolExecutor{
		block: map[string]chan struct{}{
			"q0": make(chan struct{}),
			"q2": make(chan struct{}),
		},
		started: make(map[string]chan struct{}),
	}
	for _, name := range []string{"q0", "bash1", "q2"} {
		exec.started[name] = make(chan struct{}, 1)
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-q-serial",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, exec, b)

	toolCalls := []Part{
		ToolCallPart("call-0", "question", `{"question":"first","_tag":"q0"}`),
		ToolCallPart("call-1", "bash", `{"command":"true","_tag":"bash1"}`),
		ToolCallPart("call-2", "question", `{"question":"second","_tag":"q2"}`),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.executeTools(ctx, toolCalls)
		close(done)
	}()

	// q0 and bash may start; q2 must not start until q0 finishes.
	select {
	case <-exec.started["q0"]:
	case <-time.After(2 * time.Second):
		t.Fatal("q0 did not start")
	}
	select {
	case <-exec.started["bash1"]:
	case <-time.After(2 * time.Second):
		t.Fatal("bash should start while q0 is still running")
	}
	select {
	case <-exec.started["q2"]:
		t.Fatal("q2 started before q0 finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(exec.block["q0"])
	select {
	case <-exec.started["q2"]:
	case <-time.After(2 * time.Second):
		t.Fatal("q2 did not start after q0 finished")
	}
	close(exec.block["q2"])

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("executeTools did not finish")
	}

	if exec.startAt("q2").Before(exec.finishAt("q0")) {
		t.Fatalf("q2 started at %v before q0 finished at %v", exec.startAt("q2"), exec.finishAt("q0"))
	}
}

func TestExecuteTools_QuestionGateDoesNotStarveNonQuestions(t *testing.T) {
	// With gate-before-sem: many queued questions waiting on q0 must not
	// occupy concurrency slots and block a parallel non-question tool.
	const waitingQuestions = maxConcurrentTools // would fill the sem if held during gate wait

	exec := &orderingToolExecutor{
		block: map[string]chan struct{}{
			"q0":   make(chan struct{}),
			"bash": make(chan struct{}),
		},
		started: map[string]chan struct{}{
			"q0":   make(chan struct{}, 1),
			"bash": make(chan struct{}, 1),
		},
	}
	for i := 1; i <= waitingQuestions; i++ {
		tag := fmt.Sprintf("qw%d", i)
		exec.block[tag] = make(chan struct{})
		exec.started[tag] = make(chan struct{}, 1)
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-q-starve",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, exec, b)

	toolCalls := make([]Part, 0, waitingQuestions+2)
	toolCalls = append(toolCalls, ToolCallPart("call-q0", "question", `{"question":"first","_tag":"q0"}`))
	for i := 1; i <= waitingQuestions; i++ {
		tag := fmt.Sprintf("qw%d", i)
		toolCalls = append(toolCalls, ToolCallPart(
			fmt.Sprintf("call-qw%d", i),
			"question",
			fmt.Sprintf(`{"question":"wait","_tag":"%s"}`, tag),
		))
	}
	toolCalls = append(toolCalls, ToolCallPart("call-bash", "bash", `{"command":"true","_tag":"bash"}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.executeTools(ctx, toolCalls)
		close(done)
	}()

	select {
	case <-exec.started["q0"]:
	case <-time.After(2 * time.Second):
		t.Fatal("q0 did not start")
	}
	select {
	case <-exec.started["bash"]:
	case <-time.After(2 * time.Second):
		t.Fatal("bash should start while later questions wait on q0 (gate before sem)")
	}

	close(exec.block["q0"])
	close(exec.block["bash"])
	for i := 1; i <= waitingQuestions; i++ {
		close(exec.block[fmt.Sprintf("qw%d", i)])
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("executeTools did not finish")
	}
}

func TestExecuteTools_NonQuestionsRemainConcurrent(t *testing.T) {
	exec := &countingToolExecutor{
		started: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses-nq-conc",
		Model:           &provider.Model{ID: "test-model"},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, &mockLLMClient{}, exec, b)

	toolCalls := []Part{
		ToolCallPart("call-a", "read", `{}`),
		ToolCallPart("call-b", "bash", `{}`),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		for i := 0; i < 2; i++ {
			select {
			case <-exec.started:
			case <-ctx.Done():
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
		exec.mu.Lock()
		exec.observedMax = exec.maxSeen
		exec.mu.Unlock()
		close(exec.release)
	}()

	_, allFailed := p.executeTools(ctx, toolCalls)
	if allFailed {
		t.Fatal("expected non-question tools to finish")
	}
	if exec.observedMax < 2 {
		t.Fatalf("max in flight = %d, want concurrent (>=2)", exec.observedMax)
	}
}

// orderingToolExecutor records start/finish times keyed by a _tag in args JSON.
type orderingToolExecutor struct {
	mu       sync.Mutex
	block    map[string]chan struct{}
	started  map[string]chan struct{}
	starts   map[string]time.Time
	finishes map[string]time.Time
}

func (o *orderingToolExecutor) tag(args json.RawMessage) string {
	var parsed struct {
		Tag string `json:"_tag"`
	}
	_ = json.Unmarshal(args, &parsed)
	return parsed.Tag
}

func (o *orderingToolExecutor) Execute(ctx context.Context, name string, args json.RawMessage, _ string) (string, bool, error) {
	tag := o.tag(args)
	if tag == "" {
		tag = name
	}
	o.mu.Lock()
	if o.starts == nil {
		o.starts = make(map[string]time.Time)
		o.finishes = make(map[string]time.Time)
	}
	o.starts[tag] = time.Now()
	o.mu.Unlock()

	if ch, ok := o.started[tag]; ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}

	if block, ok := o.block[tag]; ok {
		select {
		case <-block:
		case <-ctx.Done():
			return "", true, ctx.Err()
		}
	}

	o.mu.Lock()
	o.finishes[tag] = time.Now()
	o.mu.Unlock()
	return "ok", false, nil
}

func (o *orderingToolExecutor) ToolDefs(_ []string) []llm.Tool { return nil }

func (o *orderingToolExecutor) startAt(tag string) time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.starts[tag]
}

func (o *orderingToolExecutor) finishAt(tag string) time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.finishes[tag]
}

// --- Test helper types ---

// countingToolExecutor records how many calls run at once and blocks until release is closed.
type countingToolExecutor struct {
	mu          sync.Mutex
	current     int
	maxSeen     int
	observed    int
	observedMax int
	started     chan struct{}
	release     chan struct{}
}

func (c *countingToolExecutor) Execute(ctx context.Context, _ string, _ json.RawMessage, _ string) (string, bool, error) {
	c.mu.Lock()
	c.current++
	if c.current > c.maxSeen {
		c.maxSeen = c.current
	}
	c.mu.Unlock()

	select {
	case c.started <- struct{}{}:
	case <-ctx.Done():
		return "", true, ctx.Err()
	}

	select {
	case <-c.release:
	case <-ctx.Done():
		return "", true, ctx.Err()
	}

	c.mu.Lock()
	c.current--
	c.mu.Unlock()
	return "ok", false, nil
}

func (c *countingToolExecutor) ToolDefs(_ []string) []llm.Tool {
	return nil
}

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
