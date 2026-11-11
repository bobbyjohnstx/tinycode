package session

import (
	"context"
	"errors"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func intPtr(v int) *int { return &v }

// vllmModel returns a vLLM provider model.
func vllmModel(id string) *provider.Model {
	return &provider.Model{ID: id, ProviderID: "vllm"}
}

// newBudgetProc builds a Processor whose buildRequest() can run (needs a
// non-nil tool executor).
func newBudgetProc(cfg ProcessorConfig) *Processor {
	return &Processor{config: cfg, tools: &stubToolExecutor{}}
}

func TestBuildRequest_VLLMThinkingTokenBudget_ThinkingLevelWins(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{
		SessionID:               "ses-vllm-budget",
		Model:                   vllmModel("qwen3-32b"),
		ThinkingBudget:          intPtr(4096),
		VLLMThinkingTokenBudget: intPtr(8192),
	})
	req := p.buildRequest()
	if req.ThinkingTokenBudget == nil || *req.ThinkingTokenBudget != 4096 {
		t.Errorf("ThinkingTokenBudget = %v, want 4096 (/thinking level wins)", req.ThinkingTokenBudget)
	}
	if req.ThinkingBudget == nil || *req.ThinkingBudget != 4096 {
		t.Errorf("ThinkingBudget = %v, want 4096 passthrough", req.ThinkingBudget)
	}
}

func TestBuildRequest_VLLMThinkingTokenBudget_DefaultWhenNothingSet(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{SessionID: "ses-vllm-default", Model: vllmModel("qwen3-32b")})
	req := p.buildRequest()
	if req.ThinkingTokenBudget == nil || *req.ThinkingTokenBudget != vllmDefaultThinkingTokenBudget {
		t.Errorf("ThinkingTokenBudget = %v, want default %d", req.ThinkingTokenBudget, vllmDefaultThinkingTokenBudget)
	}
}

func TestBuildRequest_VLLMThinkingTokenBudget_ProviderOption(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{SessionID: "ses-vllm-prov", Model: vllmModel("qwen3-32b"), VLLMThinkingTokenBudget: intPtr(8192)})
	req := p.buildRequest()
	if req.ThinkingTokenBudget == nil || *req.ThinkingTokenBudget != 8192 {
		t.Errorf("ThinkingTokenBudget = %v, want 8192 from provider option", req.ThinkingTokenBudget)
	}
}

func TestBuildRequest_VLLMThinkingTokenBudget_NegativeOmitsField(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{SessionID: "ses-vllm-neg", Model: vllmModel("qwen3-32b"), VLLMThinkingTokenBudget: intPtr(-1)})
	req := p.buildRequest()
	if req.ThinkingTokenBudget != nil {
		t.Errorf("ThinkingTokenBudget = %v, want nil for -1 (no budget)", req.ThinkingTokenBudget)
	}
}

func TestBuildRequest_VLLMThinkingTokenBudget_ClampsToMin(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{SessionID: "ses-vllm-min", Model: vllmModel("qwen3-32b"), ThinkingBudget: intPtr(500)})
	req := p.buildRequest()
	if req.ThinkingTokenBudget == nil || *req.ThinkingTokenBudget != vllmMinThinkingTokenBudget {
		t.Errorf("ThinkingTokenBudget = %v, want clamp to %d", req.ThinkingTokenBudget, vllmMinThinkingTokenBudget)
	}
}

func TestBuildRequest_NonVLLM_NeverSetsThinkingTokenBudget(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{
		SessionID:               "ses-openai",
		Model:                   &provider.Model{ID: "gpt-4", ProviderID: "openai"},
		ThinkingBudget:          intPtr(4096),
		VLLMThinkingTokenBudget: intPtr(8192),
	})
	req := p.buildRequest()
	if req.ThinkingTokenBudget != nil {
		t.Errorf("ThinkingTokenBudget = %v, want nil for non-vLLM", req.ThinkingTokenBudget)
	}
	if req.ThinkingBudget == nil || *req.ThinkingBudget != 4096 {
		t.Errorf("ThinkingBudget = %v, want 4096 passthrough", req.ThinkingBudget)
	}
}

func TestBuildRequest_VLLMThinkingTokenBudget_DisabledOmitsField(t *testing.T) {
	p := newBudgetProc(ProcessorConfig{SessionID: "ses-vllm-dis", Model: vllmModel("qwen3-32b"), ThinkingBudgetDisabled: true})
	p.thinkingBudgetDisabled = true
	req := p.buildRequest()
	if req.ThinkingTokenBudget != nil {
		t.Errorf("ThinkingTokenBudget = %v, want nil when disabled", req.ThinkingTokenBudget)
	}
}

// ---------------------------------------------------------------------------
// 400 compatibility retry: vLLM rejects thinking_token_budget when it has no
// reasoning parser. Retry once without the field and disable it for the
// session. Uses a mock client that captures every request.
// ---------------------------------------------------------------------------

type budgetRetryClient struct {
	responses []mockResponse
	requests  []llm.Request
}

func (c *budgetRetryClient) Stream(ctx context.Context, req llm.Request, opts ...llm.StreamOption) (<-chan llm.Event, error) {
	c.requests = append(c.requests, req)
	if len(c.responses) == 0 {
		return nil, errors.New("no more mock responses")
	}
	resp := c.responses[0]
	c.responses = c.responses[1:]
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

func okText() mockResponse {
	return mockResponse{events: []llm.Event{
		{Type: llm.EventTextDelta, Text: "ok"},
		{Type: llm.EventFinish, FinishReason: "stop"},
	}}
}

func TestCallLLM_VLLM400ThinkingBudget_RetriesWithoutFieldAndDisables(t *testing.T) {
	reject := errors.New("HTTP 400: {\"detail\":\"thinking_token_budget is set but reasoning_config is not configured\"}")
	client := &budgetRetryClient{responses: []mockResponse{{err: reject}, okText()}}

	p := NewProcessor(ProcessorConfig{
		SessionID: "ses-vllm-400",
		Model:     vllmModel("qwen3-32b"),
	}, client, &mockToolExecutor{}, newTestBus(t))
	p.SetMessages([]Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}})

	msg, _, err := p.callLLM(context.Background())
	if err != nil {
		t.Fatalf("callLLM error: %v", err)
	}
	if msg == nil {
		t.Fatal("callLLM returned nil message")
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected 2 requests (retry), got %d", len(client.requests))
	}
	if client.requests[0].ThinkingTokenBudget == nil {
		t.Error("first request should carry ThinkingTokenBudget")
	}
	if client.requests[1].ThinkingTokenBudget != nil {
		t.Error("retry request should not carry ThinkingTokenBudget")
	}
	if !p.ThinkingBudgetDisabled() {
		t.Error("ThinkingBudgetDisabled = false, want true after vLLM 400")
	}
}

func TestCallLLM_VLLM400WithoutBudget_MatchesNoRetry(t *testing.T) {
	// A 400 that does not mention thinking_token_budget must NOT trigger the
	// compat path (and 400 is not retryable).
	reject := errors.New(`HTTP 400: {"detail":"bad request: unknown model"}`)
	client := &budgetRetryClient{responses: []mockResponse{{err: reject}}}

	p := NewProcessor(ProcessorConfig{SessionID: "ses-vllm-400other", Model: vllmModel("qwen3-32b")}, client, &mockToolExecutor{}, newTestBus(t))
	p.SetMessages([]Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}})

	_, _, err := p.callLLM(context.Background())
	if err == nil {
		t.Fatal("expected error for non-budget 400")
	}
	if len(client.requests) != 1 {
		t.Errorf("expected 1 request (no compat retry), got %d", len(client.requests))
	}
	if p.ThinkingBudgetDisabled() {
		t.Error("ThinkingBudgetDisabled = true, want false for unrelated 400")
	}
}

// A 400 that mentions the field but comes from a non-vLLM provider is left
// alone (the compat path is vLLM-specific).
func TestCallLLM_NonVLLM400ThinkingBudget_NoCompatPath(t *testing.T) {
	reject := errors.New(`HTTP 400: {"detail":"thinking_token_budget not supported"}`)
	client := &budgetRetryClient{responses: []mockResponse{{err: reject}}}

	p := NewProcessor(ProcessorConfig{SessionID: "ses-oai-400", Model: &provider.Model{ID: "gpt-4", ProviderID: "openai"}}, client, &mockToolExecutor{}, newTestBus(t))
	p.SetMessages([]Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}})

	_, _, err := p.callLLM(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	// No budget was ever set on a non-vLLM request, so the compat path is
	// skipped and 400 is not retryable -> exactly one attempt.
	if len(client.requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(client.requests))
	}
	if p.ThinkingBudgetDisabled() {
		t.Error("ThinkingBudgetDisabled should stay false for non-vLLM")
	}
}

func newTestBus(t *testing.T) *bus.Bus {
	t.Helper()
	b := bus.New()
	t.Cleanup(b.Close)
	return b
}
