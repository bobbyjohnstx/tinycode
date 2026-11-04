package session

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func TestCompactionOutputReserve(t *testing.T) {
	tests := []struct {
		name        string
		contextLen  int
		outputLimit int
		want        int
	}{
		{name: "large window keeps 20k", contextLen: 100000, outputLimit: 4096, want: 20000},
		{name: "32k window keeps 20k", contextLen: 32768, outputLimit: 4096, want: 20000},
		{name: "output limit above 20k is kept when it fits", contextLen: 128000, outputLimit: 64000, want: 64000},
		{name: "8k window uses a fifth with a 2048 floor", contextLen: 8192, outputLimit: 4096, want: 2048},
		{name: "4k window uses the 2048 floor", contextLen: 4096, outputLimit: 2048, want: 2048},
		{name: "tiny window stays under the context", contextLen: 1500, outputLimit: 512, want: 750},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactionOutputReserve(tt.contextLen, tt.outputLimit)
			if got != tt.want {
				t.Errorf("compactionOutputReserve(%d, %d) = %d, want %d", tt.contextLen, tt.outputLimit, got, tt.want)
			}
		})
	}
}

func TestCompact_CircuitBreakerReturnsError(t *testing.T) {
	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_breaker",
		Model:      &provider.Model{ID: "local", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction: DefaultCompactionConfig(),
	}, &mockLLMClient{}, &stubToolExecutor{}, b)

	p.compactionCount = compactionCircuitBreakerThreshold

	_, err := p.compact(context.Background(), false)
	if err == nil {
		t.Fatal("expected error from circuit breaker, got nil")
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}
}

func TestCheckCompaction_SmallContextNoElision(t *testing.T) {
	// Elision is disabled — soft threshold no longer masks old tool results.
	// context 8192, reserve 2048, threshold 6144.
	// 5000 input tokens is below the compaction threshold.
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "response"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 5000, CompletionTokens: 100}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_small",
		Model:           &provider.Model{ID: "local", Limit: provider.ModelLimit{Context: 8192, Output: 4096}},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, &stubToolExecutor{}, b)

	var messages []Message
	for i := 0; i < 10; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart("c", "read", "output", false),
			},
		})
	}
	p.SetMessages(messages)

	result := p.Process(context.Background(), "test query")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if p.elisionDone {
		t.Error("elision should be disabled — soft threshold no longer masks results")
	}
	if p.compactionCount != 0 {
		t.Errorf("expected compactionCount=0, got %d", p.compactionCount)
	}
}

func TestCompactBreakerTripped_Threshold(t *testing.T) {
	if CompactBreakerTripped(compactionCircuitBreakerThreshold - 1) {
		t.Fatal("below threshold must not trip")
	}
	if !CompactBreakerTripped(compactionCircuitBreakerThreshold) {
		t.Fatal("at threshold must trip")
	}
}

func TestCompact_DurableFailureBudgetAcrossProcessors(t *testing.T) {
	// Simulate SessionManager seeding the next prompt from prior failures.
	p1 := NewProcessor(ProcessorConfig{
		SessionID:          "ses-durable",
		Model:              &provider.Model{ID: "m", Limit: provider.ModelLimit{Context: 8192}},
		Compaction:         DefaultCompactionConfig(),
		CompactionFailures: compactionCircuitBreakerThreshold,
	}, &mockLLMClient{}, &stubToolExecutor{}, bus.New())
	ok, err := p1.Compact(context.Background())
	if err == nil || ok {
		t.Fatal("expected circuit breaker on seeded failures")
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Fatalf("got %v", err)
	}
	if !CompactBreakerTripped(p1.CompactionFailures()) {
		t.Fatal("failures should remain tripped for next prompt")
	}
}

// ---------------------------------------------------------------------------
// compact() integration: tool pair preservation
// ---------------------------------------------------------------------------

func TestCompact_PreservedMessagesNeverOrphanToolResults(t *testing.T) {
	// Build a history with tool pairs, compact it with force=true, then
	// verify no tool_result message lacks a preceding tool_use message.
	b := bus.New()
	defer b.Close()

	// Mock client returns a summary for the compaction request.
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "Summary of the conversation."},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50}},
			}},
		},
	}

	p := NewProcessor(ProcessorConfig{
		SessionID: "ses_orphan_check",
		Model:     &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction: CompactionConfig{
			MaskObservations: true,
			MinPreserve:      100,  // Small so more gets compacted
			MaxPreserve:      200,
			MaxMessages:      80,
		},
	}, client, &stubToolExecutor{}, b)

	// Build 3 tool-use/result pairs plus a final assistant text response.
	// Use long-enough text that the estimator will compact the older messages.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart(strings.Repeat("question ", 200))}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c1", "read", `{"path":"a.go"}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c1", "read", strings.Repeat("content ", 200), false)}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c2", "edit", `{"path":"b.go"}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c2", "edit", strings.Repeat("result ", 200), false)}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c3", "bash", `{"command":"ls"}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c3", "bash", "file list output", false)}},
		{Role: RoleAssistant, Parts: []Part{TextPart("All done.")}},
	}
	p.SetMessages(messages)

	compacted, err := p.compact(context.Background(), true)
	if err != nil {
		t.Fatalf("compact returned error: %v", err)
	}
	if !compacted {
		t.Fatal("expected compaction to proceed")
	}

	// Check that no tool_result message is orphaned (missing a preceding tool_use).
	result := p.Messages()
	toolCallIDs := make(map[string]bool)
	for _, msg := range result {
		for _, part := range msg.Parts {
			if part.Type == PartToolCall {
				toolCallIDs[part.ToolCallID] = true
			}
			if part.Type == PartToolResult {
				if !toolCallIDs[part.ToolCallID] {
					t.Errorf("orphaned tool_result found: ToolCallID=%q has no preceding tool_use", part.ToolCallID)
				}
			}
		}
	}
}

func TestCompact_ForceWithSmallMessageSet(t *testing.T) {
	// Compact with force=true on a small set (4 messages including tool pairs).
	// The estimator would normally skip this, but force=true should proceed.
	b := bus.New()
	defer b.Close()

	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "Forced summary."},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 50, CompletionTokens: 20}},
			}},
		},
	}

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_force_small",
		Model:      &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, b)

	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("do something")}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c1", "read", `{"path":"x.go"}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c1", "read", "content of x.go", false)}},
		{Role: RoleAssistant, Parts: []Part{TextPart("Here is the result.")}},
	}
	p.SetMessages(messages)

	compacted, err := p.compact(context.Background(), true)
	if err != nil {
		t.Fatalf("compact(force=true) returned error: %v", err)
	}
	if !compacted {
		t.Fatal("expected forced compaction to proceed on small message set")
	}

	// After compaction, first message should be the summary.
	result := p.Messages()
	if len(result) == 0 {
		t.Fatal("expected at least one message after compaction")
	}
	if result[0].Role != RoleUser {
		t.Errorf("first message after compact should be user (summary), got %s", result[0].Role)
	}
	if !strings.Contains(result[0].Parts[0].Text, "Forced summary") {
		t.Errorf("expected summary text, got %q", result[0].Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// compactOrStop backoff behavior
// ---------------------------------------------------------------------------

func TestCompactOrStop_ReturnNilWhenNotCompacted(t *testing.T) {
	// When compact returns (false, nil) because there is nothing to compact,
	// compactOrStop should return nil, not an error.
	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_no_compact",
		Model:      &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction: DefaultCompactionConfig(),
	}, &mockLLMClient{}, &stubToolExecutor{}, b)

	// Only 2 messages: too small for compaction, even with force.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hi")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("hello")}},
	}
	p.SetMessages(messages)

	err := p.compactOrStop(context.Background(), "proactive")
	if err != nil {
		t.Errorf("compactOrStop should return nil when <=2 messages, got: %v", err)
	}
}

func TestCompactOrStop_ReturnNilWhenMessageCountUnchanged(t *testing.T) {
	// When compact returns (true, nil) but message count is unchanged,
	// compactOrStop should return nil (back off instead of stopping).
	b := bus.New()
	defer b.Close()

	// The mock returns a summary. After compact, the processor replaces
	// compacted messages with summary + preserved. We need the count to stay
	// the same or increase. Build messages where compact preserves all but one,
	// then the summary replaces the compacted portion with 1 message => same count.
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "Summary."},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 50, CompletionTokens: 20}},
			}},
		},
	}

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_no_progress",
		Model:      &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, b)

	// 3 messages: compaction replaces first message with summary => still 3
	// (summary + 2 preserved). Actually with force=true and preserveIdx computed
	// to len-2, it compacts 1 msg into summary => summary + 2 preserved = 3.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("first")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("response")}},
		{Role: RoleUser, Parts: []Part{TextPart("follow up")}},
	}
	p.SetMessages(messages)

	err := p.compactOrStop(context.Background(), "proactive")
	if err != nil {
		t.Errorf("compactOrStop should return nil when count unchanged, got: %v", err)
	}
}

func TestCompactOrStop_PropagatesCompactError(t *testing.T) {
	// When compact returns an error, compactOrStop should propagate it.
	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:          "ses_error",
		Model:              &provider.Model{ID: "test-model", Limit: provider.ModelLimit{Context: 100000, Output: 4096}},
		Compaction:         DefaultCompactionConfig(),
		CompactionFailures: compactionCircuitBreakerThreshold, // Will trip the breaker
	}, &mockLLMClient{}, &stubToolExecutor{}, b)

	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("world")}},
		{Role: RoleUser, Parts: []Part{TextPart("more")}},
	}
	p.SetMessages(messages)

	err := p.compactOrStop(context.Background(), "proactive")
	if err == nil {
		t.Fatal("compactOrStop should propagate compact error")
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}
}
