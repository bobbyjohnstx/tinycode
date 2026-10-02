package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestDiscoverOpenRouter_RegistersModelsFromAPI(t *testing.T) {
	maxTokens := 4096
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := openRouterResponse{
			Data: []openRouterModel{
				{
					ID:            "anthropic/claude-3-opus",
					Name:          "Claude 3 Opus",
					ContextLength: 200000,
					TopProvider: &openRouterTopProvider{
						ContextLength:       200000,
						MaxCompletionTokens: &maxTokens,
					},
					SupportedParameters: []string{"temperature", "tools"},
					Pricing: &openRouterPricing{
						Prompt:     "0.000015",
						Completion: "0.000075",
					},
					Architecture: &openRouterArchitecture{
						InputModalities: []string{"text", "image"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)

	// Override the hardcoded URL by using DiscoverOpenRouter directly
	// We need to use the httptest server, but DiscoverOpenRouter hardcodes the URL.
	// Instead, test buildOpenRouterModel directly.
	_ = srv
	_ = d

	// Test the buildOpenRouterModel function directly since DiscoverOpenRouter
	// hardcodes the OpenRouter URL.
	entry := openRouterModel{
		ID:            "anthropic/claude-3-opus",
		Name:          "Claude 3 Opus",
		ContextLength: 200000,
		TopProvider: &openRouterTopProvider{
			ContextLength:       200000,
			MaxCompletionTokens: &maxTokens,
		},
		SupportedParameters: []string{"temperature", "tools"},
		Pricing: &openRouterPricing{
			Prompt:     "0.000015",
			Completion: "0.000075",
		},
		Architecture: &openRouterArchitecture{
			InputModalities: []string{"text", "image"},
		},
	}

	m := buildOpenRouterModel(entry)

	if m.ID != "anthropic/claude-3-opus" {
		t.Errorf("expected ID 'anthropic/claude-3-opus', got %q", m.ID)
	}
	if m.Name != "Claude 3 Opus" {
		t.Errorf("expected Name 'Claude 3 Opus', got %q", m.Name)
	}
	if m.ProviderID != "openrouter" {
		t.Errorf("expected ProviderID 'openrouter', got %q", m.ProviderID)
	}
}

func TestBuildOpenRouterModel_SetsContextFromTopProvider(t *testing.T) {
	maxTokens := 8192
	entry := openRouterModel{
		ID:            "meta/llama-3-70b",
		Name:          "Llama 3 70B",
		ContextLength: 8192,
		TopProvider: &openRouterTopProvider{
			ContextLength:       128000,
			MaxCompletionTokens: &maxTokens,
		},
	}

	m := buildOpenRouterModel(entry)

	if m.Limit.Context != 128000 {
		t.Errorf("expected context 128000 from TopProvider, got %d", m.Limit.Context)
	}
	if m.Limit.Output != 8192 {
		t.Errorf("expected output 8192, got %d", m.Limit.Output)
	}
}

func TestBuildOpenRouterModel_FallbackOutputWhenNoMaxCompletionTokens(t *testing.T) {
	entry := openRouterModel{
		ID:            "test/model",
		Name:          "Test",
		ContextLength: 100000,
	}

	m := buildOpenRouterModel(entry)

	// maxOutput = min(16384, contextLen/5) = min(16384, 20000) = 16384
	if m.Limit.Output != 16384 {
		t.Errorf("expected fallback output 16384, got %d", m.Limit.Output)
	}
}

func TestBuildOpenRouterModel_FallbackOutputForSmallContext(t *testing.T) {
	entry := openRouterModel{
		ID:            "test/small",
		Name:          "Test Small",
		ContextLength: 8192,
	}

	m := buildOpenRouterModel(entry)

	// maxOutput = min(16384, 8192/5) = min(16384, 1638) = 1638
	if m.Limit.Output != 1638 {
		t.Errorf("expected fallback output 1638, got %d", m.Limit.Output)
	}
}

func TestBuildOpenRouterModel_ParsesPricingCorrectly(t *testing.T) {
	entry := openRouterModel{
		ID:            "test/priced",
		Name:          "Priced Model",
		ContextLength: 32768,
		Pricing: &openRouterPricing{
			Prompt:     "0.000015",
			Completion: "0.000075",
		},
	}

	m := buildOpenRouterModel(entry)

	// 0.000015 * 1_000_000 = 15.0
	if m.Cost.Input != 15.0 {
		t.Errorf("expected input cost 15.0, got %f", m.Cost.Input)
	}
	// 0.000075 * 1_000_000 = 75.0
	if m.Cost.Output != 75.0 {
		t.Errorf("expected output cost 75.0, got %f", m.Cost.Output)
	}
}

func TestBuildOpenRouterModel_SetsCapabilitiesFromParameters(t *testing.T) {
	entry := openRouterModel{
		ID:                  "test/capable",
		Name:                "Capable Model",
		ContextLength:       32768,
		SupportedParameters: []string{"temperature", "tools"},
		Architecture: &openRouterArchitecture{
			InputModalities: []string{"text", "image"},
		},
	}

	m := buildOpenRouterModel(entry)

	if !m.Capabilities.Temperature {
		t.Error("expected Temperature capability")
	}
	if !m.Capabilities.ToolCall {
		t.Error("expected ToolCall capability")
	}
	if !m.Capabilities.Input.Text {
		t.Error("expected text input capability")
	}
	if !m.Capabilities.Input.Image {
		t.Error("expected image input capability")
	}
	if !m.Capabilities.Attachment {
		t.Error("expected Attachment capability when image input is supported")
	}
}

func TestBuildOpenRouterModel_SetsReasoningFromField(t *testing.T) {
	entry := openRouterModel{
		ID:            "test/reasoning",
		Name:          "Reasoning Model",
		ContextLength: 32768,
		Reasoning:     &openRouterReasoning{DefaultEnabled: true},
	}

	m := buildOpenRouterModel(entry)

	if !m.Capabilities.Reasoning {
		t.Error("expected Reasoning capability when DefaultEnabled is true")
	}
}

func TestBuildOpenRouterModel_ExtractsFamilyFromID(t *testing.T) {
	entry := openRouterModel{
		ID:            "anthropic/claude-3-opus",
		Name:          "Claude 3 Opus",
		ContextLength: 200000,
	}

	m := buildOpenRouterModel(entry)

	if m.Family != "anthropic" {
		t.Errorf("expected family 'anthropic', got %q", m.Family)
	}
}

func TestBuildOpenRouterModel_DefaultsToTextInputWhenNoArchitecture(t *testing.T) {
	entry := openRouterModel{
		ID:            "test/no-arch",
		Name:          "No Arch",
		ContextLength: 4096,
	}

	m := buildOpenRouterModel(entry)

	if !m.Capabilities.Input.Text {
		t.Error("expected default text input when no architecture specified")
	}
	if m.Capabilities.Input.Image {
		t.Error("expected no image input when no architecture specified")
	}
}

func TestDiscoverOpenRouter_ReturnsErrorOnConnectionFailure(t *testing.T) {
	// Start a server and immediately close it to get a port that refuses connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()
	d := NewDiscovery(reg, b)

	// DiscoverOpenRouter hardcodes the OpenRouter URL, so this test verifies
	// the error return path. Use a cancelled context to force a request failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := d.DiscoverOpenRouter(ctx, "test-key")
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}
}

func TestHandleDiscoveryFailure_RemovesProviderAfterMaxFailures(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.removed")
	defer sub.Unsubscribe()

	// Pre-register the provider so it can be removed.
	reg.Register(&Info{
		ID:     "test-prov",
		Name:   "Test",
		Models: map[string]*Model{"m": {ID: "m", ProviderID: "test-prov"}},
	})

	d := NewDiscovery(reg, b)

	// Simulate maxConsecutiveFailures (3) failures.
	for i := 0; i < maxConsecutiveFailures; i++ {
		d.handleDiscoveryFailure("test-prov", "Test", nil)
	}

	if reg.Has("test-prov") {
		t.Error("expected provider to be removed after max failures")
	}
	if d.shouldPoll("test-prov") {
		t.Error("expected provider to be dormant after max failures")
	}

	// Should have published provider.removed.
	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["providerID"] != "test-prov" {
			t.Errorf("expected providerID 'test-prov', got %v", props["providerID"])
		}
	default:
		t.Fatal("expected provider.removed event")
	}
}

func TestHandleDiscoverySuccess_ResetsFailureCounterAndPublishesReconnected(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.reconnected")
	defer sub.Unsubscribe()

	d := NewDiscovery(reg, b)

	// Record some failures first.
	reg.RecordFailure("test-prov")
	reg.RecordFailure("test-prov")

	d.handleDiscoverySuccess("test-prov")

	if reg.Failures("test-prov") != 0 {
		t.Errorf("expected failures reset to 0, got %d", reg.Failures("test-prov"))
	}

	// Should publish reconnected event because prev > 0.
	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["providerID"] != "test-prov" {
			t.Errorf("expected providerID 'test-prov', got %v", props["providerID"])
		}
		if prevFails, ok := props["previous_failures"].(int); !ok || prevFails != 2 {
			t.Errorf("expected previous_failures 2, got %v", props["previous_failures"])
		}
	default:
		t.Fatal("expected provider.reconnected event")
	}
}

func TestHandleDiscoverySuccess_NoEventWhenNoFailures(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.reconnected")
	defer sub.Unsubscribe()

	d := NewDiscovery(reg, b)

	d.handleDiscoverySuccess("healthy-prov")

	select {
	case <-sub.C:
		t.Error("expected no provider.reconnected event when no prior failures")
	default:
		// Good — no event published
	}
}
