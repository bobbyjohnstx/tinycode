package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestDiscoverLMStudio_RegistersModelsFromAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		resp := vllmModelsResponse{
			Data: []vllmModel{
				{ID: "ornith-1.0-9b-mlx"},
				{ID: "llama-3.1-8b"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	p, err := reg.GetProvider("lm-studio")
	if err != nil {
		t.Fatalf("lm-studio provider not registered: %v", err)
	}
	if p.Name != "LM Studio" {
		t.Errorf("expected provider name 'LM Studio', got %q", p.Name)
	}
	if len(p.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(p.Models))
	}
}

func TestDiscoverLMStudio_SetsDefaultContextLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vllmModelsResponse{
			Data: []vllmModel{{ID: "test-model"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	m, err := reg.GetModel("lm-studio", "test-model")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	if m.Limit.Context != 8192 {
		t.Errorf("expected default context 8192, got %d", m.Limit.Context)
	}
	if m.Limit.Output != 4096 {
		t.Errorf("expected default output 4096, got %d", m.Limit.Output)
	}
}

func TestDiscoverLMStudio_UsesMaxModelLen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vllmModelsResponse{
			Data: []vllmModel{{ID: "loaded-model", MaxModelLen: 32768}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	m, err := reg.GetModel("lm-studio", "loaded-model")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	if m.Limit.Context != 32768 {
		t.Errorf("expected context 32768 from max_model_len, got %d", m.Limit.Context)
	}
	if m.Limit.Output != 4096 {
		t.Errorf("expected output 4096, got %d", m.Limit.Output)
	}
}

func TestDiscoverLMStudio_SetsCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vllmModelsResponse{
			Data: []vllmModel{{ID: "test-model"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	m, err := reg.GetModel("lm-studio", "test-model")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	if !m.Capabilities.ToolCall {
		t.Error("expected ToolCall capability to be true")
	}
	if !m.Capabilities.Temperature {
		t.Error("expected Temperature capability to be true")
	}
	if !m.Capabilities.Input.Text {
		t.Error("expected text input capability to be true")
	}
}

func TestDiscoverLMStudio_PublishesDiscoveredEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vllmModelsResponse{
			Data: []vllmModel{{ID: "model-a"}, {ID: "model-b"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.discovered")
	defer sub.Unsubscribe()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map properties")
		}
		if props["providerID"] != "lm-studio" {
			t.Errorf("expected providerID 'lm-studio', got %v", props["providerID"])
		}
		if count, ok := props["modelCount"].(int); !ok || count != 2 {
			t.Errorf("expected modelCount 2, got %v", props["modelCount"])
		}
	default:
		t.Fatal("expected provider.discovered event to be published")
	}
}

func TestDiscoverLMStudio_HandlesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	if reg.Has("lm-studio") {
		t.Error("expected lm-studio provider NOT to be registered after HTTP error")
	}
	if reg.Failures("lm-studio") != 1 {
		t.Errorf("expected 1 failure recorded, got %d", reg.Failures("lm-studio"))
	}
}

func TestDiscoverLMStudio_HandlesInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverLMStudio(t.Context(), srv.URL)

	if reg.Has("lm-studio") {
		t.Error("expected lm-studio provider NOT to be registered after invalid JSON")
	}
	if reg.Failures("lm-studio") != 1 {
		t.Errorf("expected 1 failure recorded, got %d", reg.Failures("lm-studio"))
	}
}
