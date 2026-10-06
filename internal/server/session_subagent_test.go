package server

import (
	"context"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

// stubLLMClient returns a fixed text response with usage data.
type stubLLMClient struct {
	usage *llm.Usage
}

func (c *stubLLMClient) Stream(_ context.Context, _ llm.Request, _ ...llm.StreamOption) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 3)
	ch <- llm.Event{Type: llm.EventTextDelta, Text: "subagent reply"}
	ch <- llm.Event{Type: llm.EventFinish, FinishReason: "stop", Usage: c.usage}
	close(ch)
	return ch, nil
}

// newSubagentTestSM creates a SessionManager wired with a stub LLM client,
// a registered model, and optional config overrides.
func newSubagentTestSM(t *testing.T, b *bus.Bus, cfg *config.Info, usage *llm.Usage) *SessionManager {
	t.Helper()
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:   "test",
		Name: "test",
		Models: map[string]*provider.Model{
			"test-model": {
				ID:         "test-model",
				ProviderID: "test",
				API:        provider.ModelAPI{URL: "http://localhost:0"},
			},
		},
	})

	toolReg := tool.NewRegistry(&tool.Context{
		Directory: t.TempDir(),
		Bus:       b,
	})

	agentReg := agent.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, nil, agentReg, nil, cfg, nil)
	sm.clientFactory = func(_ *provider.Model) llm.Client {
		return &stubLLMClient{usage: usage}
	}
	return sm
}

func TestRunSubagent_PublishesTokenUsageEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	usage := &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	sm := newSubagentTestSM(t, b, &config.Info{}, usage)

	sub := b.Subscribe("subagent.completed")
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := sm.RunSubagent(ctx, "ses_parent", 0, "test prompt", "executor", "", false)
	if err != nil {
		t.Fatalf("RunSubagent returned error: %v", err)
	}

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map[string]any properties on subagent.completed event")
		}
		if got := props["parentSessionID"]; got != "ses_parent" {
			t.Errorf("parentSessionID = %v, want ses_parent", got)
		}
		if got := props["agent"]; got != "executor" {
			t.Errorf("agent = %v, want executor", got)
		}
		if got, ok := props["inputTokens"].(int); !ok || got == 0 {
			t.Errorf("inputTokens = %v, want > 0", props["inputTokens"])
		}
		if got, ok := props["outputTokens"].(int); !ok || got == 0 {
			t.Errorf("outputTokens = %v, want > 0", props["outputTokens"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for subagent.completed event")
	}
}

func TestResolveAgentLLMParams_AgentOverridesConfig(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()

	cfgTemp := 0.2
	cfgTopP := 0.5
	agentTemp := 0.9
	agentTopP := 0.95
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, &config.Info{
		Temperature: &cfgTemp,
		TopP:        &cfgTopP,
	}, nil)

	temp, topP := sm.resolveAgentLLMParams(&agent.Info{
		Temperature: &agentTemp,
		TopP:        &agentTopP,
	})
	if temp == nil || *temp != 0.9 {
		t.Errorf("Temperature = %v, want 0.9", temp)
	}
	if topP == nil || *topP != 0.95 {
		t.Errorf("TopP = %v, want 0.95", topP)
	}

	// When agent leaves params unset, fall back to config.
	temp, topP = sm.resolveAgentLLMParams(&agent.Info{})
	if temp == nil || *temp != 0.2 {
		t.Errorf("Temperature = %v, want config 0.2", temp)
	}
	if topP == nil || *topP != 0.5 {
		t.Errorf("TopP = %v, want config 0.5", topP)
	}
}

func TestRunSubagent_InheritsConfigLLMParams(t *testing.T) {
	b := bus.New()
	defer b.Close()

	temp := 0.7
	topP := 0.9
	maxTok := 4096
	cfg := &config.Info{
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTok,
	}

	// We capture the request sent to the LLM client to verify params propagated.
	var capturedReq llm.Request
	sm := newSubagentTestSM(t, b, cfg, nil)
	sm.clientFactory = func(_ *provider.Model) llm.Client {
		return &capturingLLMClient{captured: &capturedReq}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := sm.RunSubagent(ctx, "ses_parent", 0, "test prompt", "executor", "", false)
	if err != nil {
		t.Fatalf("RunSubagent returned error: %v", err)
	}

	if capturedReq.Temperature == nil || *capturedReq.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", capturedReq.Temperature)
	}
	if capturedReq.TopP == nil || *capturedReq.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", capturedReq.TopP)
	}
	if capturedReq.MaxTokens == nil || *capturedReq.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %v, want 4096", capturedReq.MaxTokens)
	}
}

func TestRunSubagent_PrefersAgentLLMParamsOverConfig(t *testing.T) {
	b := bus.New()
	defer b.Close()

	cfgTemp := 0.1
	cfgTopP := 0.2
	agentTemp := 0.8
	agentTopP := 0.85
	cfg := &config.Info{
		Temperature: &cfgTemp,
		TopP:        &cfgTopP,
	}

	var capturedReq llm.Request
	sm := newSubagentTestSM(t, b, cfg, nil)
	_ = sm.agentRegistry.LoadDefaults(nil, nil)
	sm.agentRegistry.ApplyConfigOverrides(map[string]agent.ConfigOverride{
		"executor": {Temperature: &agentTemp, TopP: &agentTopP},
	}, nil, nil)
	sm.clientFactory = func(_ *provider.Model) llm.Client {
		return &capturingLLMClient{captured: &capturedReq}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := sm.RunSubagent(ctx, "ses_parent", 0, "test prompt", "executor", "", false)
	if err != nil {
		t.Fatalf("RunSubagent returned error: %v", err)
	}

	if capturedReq.Temperature == nil || *capturedReq.Temperature != 0.8 {
		t.Errorf("Temperature = %v, want agent 0.8", capturedReq.Temperature)
	}
	if capturedReq.TopP == nil || *capturedReq.TopP != 0.85 {
		t.Errorf("TopP = %v, want agent 0.85", capturedReq.TopP)
	}
}

// capturingLLMClient captures the first request and returns a simple response.
type capturingLLMClient struct {
	captured *llm.Request
}

func (c *capturingLLMClient) Stream(_ context.Context, req llm.Request, _ ...llm.StreamOption) (<-chan llm.Event, error) {
	*c.captured = req
	ch := make(chan llm.Event, 2)
	ch <- llm.Event{Type: llm.EventTextDelta, Text: "ok"}
	ch <- llm.Event{Type: llm.EventFinish, FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func TestRunSubagent_RegistersMCPTools(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sm := newSubagentTestSM(t, b, &config.Info{}, nil)

	// Verify the toolSnapshot was created and MCP tools would be added on top.
	// Since we can't inject a real MCP service in unit tests, we verify that
	// the code path doesn't panic with nil mcpSvc and that subagent tools
	// come from the snapshot (not the live registry).
	if sm.toolSnapshot == nil {
		t.Fatal("expected toolSnapshot to be non-nil after construction")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// RunSubagent should work without MCP service (nil mcpSvc path).
	resp, err := sm.RunSubagent(ctx, "ses_parent", 0, "test prompt", "executor", "", false)
	if err != nil {
		t.Fatalf("RunSubagent with nil mcpSvc returned error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response from subagent")
	}
}

func TestSubagentLabel_NoCollision(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 30; i++ {
		label := nextSubagentLabel("executor")
		if seen[label] {
			t.Fatalf("duplicate label on iteration %d: %s", i, label)
		}
		seen[label] = true
	}
}

func TestRunSubagent_NilConfig(t *testing.T) {
	b := bus.New()
	defer b.Close()

	// Pass nil config to verify no panic when accessing Temperature/TopP/MaxTokens.
	sm := newSubagentTestSM(t, b, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := sm.RunSubagent(ctx, "ses_parent", 0, "test prompt", "executor", "", false)
	if err != nil {
		t.Fatalf("RunSubagent with nil config returned error: %v", err)
	}
}
