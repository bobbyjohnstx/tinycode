package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestProviderRemovedAfterConsecutiveFailures(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	// Pre-register ollama so we can verify removal
	reg.Register(&Info{
		ID:   "ollama",
		Name: "Ollama",
		Models: map[string]*Model{
			"llama3": {ID: "llama3", ProviderID: "ollama"},
		},
	})

	// Server that always fails
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewDiscovery(reg, b)

	// Fail 3 times — should trigger removal
	for i := 0; i < maxConsecutiveFailures; i++ {
		d.discoverOllama(t.Context(), srv.URL)
	}

	if _, err := reg.GetProvider("ollama"); err == nil {
		t.Fatal("expected ollama to be removed after consecutive failures")
	}

	// Failure counter should also be cleaned up
	if got := reg.Failures("ollama"); got != 0 {
		t.Errorf("expected failure count 0 after removal, got %d", got)
	}
}

func TestProviderRemovedEventPublished(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.removed")
	defer sub.Unsubscribe()

	reg.Register(&Info{
		ID:     "vllm",
		Name:   "vLLM",
		Models: make(map[string]*Model),
	})

	// Server that always returns connection refused (use closed server)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately so connections fail

	d := NewDiscovery(reg, b)

	for i := 0; i < maxConsecutiveFailures; i++ {
		d.discoverVLLM(t.Context(), srv.URL)
	}

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map properties")
		}
		if props["providerID"] != "vllm" {
			t.Errorf("expected providerID 'vllm', got %v", props["providerID"])
		}
		if props["reason"] != "consecutive_failures" {
			t.Errorf("expected reason 'consecutive_failures', got %v", props["reason"])
		}
	default:
		t.Fatal("expected provider.removed event")
	}
}

func TestFailureCounterResetOnSuccess(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n <= 2 {
			// First 2 requests fail
			http.Error(w, "temporarily down", http.StatusInternalServerError)
			return
		}
		// Third request succeeds
		resp := ollamaTagsResponse{
			Models: []ollamaModel{
				{Name: "llama3:latest", Details: &ollamaModelDetails{ContextLength: 8192}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewDiscovery(reg, b)

	// Fail twice
	d.discoverOllama(t.Context(), srv.URL)
	d.discoverOllama(t.Context(), srv.URL)
	if got := reg.Failures("ollama"); got != 2 {
		t.Fatalf("expected 2 failures, got %d", got)
	}

	// Succeed — should reset
	d.discoverOllama(t.Context(), srv.URL)
	if got := reg.Failures("ollama"); got != 0 {
		t.Errorf("expected 0 failures after success, got %d", got)
	}

	// Verify provider is registered with the model
	if _, err := reg.GetModel("ollama", "llama3:latest"); err != nil {
		t.Errorf("expected model to be registered: %v", err)
	}
}

func TestReconnectedEventPublished(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.reconnected")
	defer sub.Unsubscribe()

	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n <= 2 {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		resp := ollamaTagsResponse{
			Models: []ollamaModel{
				{Name: "llama3:latest"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewDiscovery(reg, b)

	// Fail twice, then succeed
	d.discoverOllama(t.Context(), srv.URL)
	d.discoverOllama(t.Context(), srv.URL)
	d.discoverOllama(t.Context(), srv.URL)

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map properties")
		}
		if props["providerID"] != "ollama" {
			t.Errorf("expected providerID 'ollama', got %v", props["providerID"])
		}
		if props["previous_failures"] != 2 {
			t.Errorf("expected previous_failures 2, got %v", props["previous_failures"])
		}
	default:
		t.Fatal("expected provider.reconnected event")
	}
}

func TestNoReconnectedEventWhenNoFailures(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("provider.reconnected")
	defer sub.Unsubscribe()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaTagsResponse{
			Models: []ollamaModel{{Name: "llama3:latest"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewDiscovery(reg, b)
	d.discoverOllama(t.Context(), srv.URL)

	select {
	case evt := <-sub.C:
		t.Fatalf("unexpected reconnected event: %+v", evt)
	default:
		// expected — no event
	}
}

func TestModelDiffDetection(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	// Pre-register with model-a and model-b
	reg.Register(&Info{
		ID:   "ollama",
		Name: "Ollama",
		Models: map[string]*Model{
			"model-a": {ID: "model-a", ProviderID: "ollama"},
			"model-b": {ID: "model-b", ProviderID: "ollama"},
		},
	})

	// Server returns model-b and model-c (model-a removed, model-c added)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaTagsResponse{
			Models: []ollamaModel{
				{Name: "model-b"},
				{Name: "model-c"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewDiscovery(reg, b)
	d.discoverOllama(t.Context(), srv.URL)

	// After discovery, registry should have model-b and model-c
	p, err := reg.GetProvider("ollama")
	if err != nil {
		t.Fatalf("provider not found: %v", err)
	}
	if _, ok := p.Models["model-a"]; ok {
		t.Error("model-a should have been removed")
	}
	if _, ok := p.Models["model-b"]; !ok {
		t.Error("model-b should still exist")
	}
	if _, ok := p.Models["model-c"]; !ok {
		t.Error("model-c should have been added")
	}
}

func TestLMStudioRemovedAfterFailures(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	reg.Register(&Info{
		ID:     "lm-studio",
		Name:   "LM Studio",
		Models: make(map[string]*Model),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	d := NewDiscovery(reg, b)

	for i := 0; i < maxConsecutiveFailures; i++ {
		d.discoverLMStudio(t.Context(), srv.URL)
	}

	if _, err := reg.GetProvider("lm-studio"); err == nil {
		t.Fatal("expected lm-studio to be removed after consecutive failures")
	}
}
