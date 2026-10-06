package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestDiscoverOllama_DoesNotWarmupEveryModel(t *testing.T) {
	var mu sync.Mutex
	var completions int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModel{
					{Name: "llama3:latest"},
					{Name: "qwen:7b"},
				},
			})
		case "/v1/chat/completions":
			mu.Lock()
			completions++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverOllama(t.Context(), srv.URL)

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	got := completions
	mu.Unlock()
	if got != 0 {
		t.Fatalf("discovery sent %d warmup completions, want 0", got)
	}
}

func TestDiscoveryWarmup_ProbesSelectedOllamaModel(t *testing.T) {
	probed := make(chan string, 4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModel{{Name: "llama3:latest"}, {Name: "qwen:7b"}},
			})
		case "/v1/chat/completions":
			var body struct {
				Model string `json:"model"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			probed <- body.Model
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"calculator"}}]}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverOllama(t.Context(), srv.URL)

	selected, err := reg.GetModel("ollama", "llama3:latest")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	d.Warmup(t.Context(), selected)
	d.Warmup(t.Context(), selected)

	select {
	case modelID := <-probed:
		if modelID != "llama3:latest" {
			t.Fatalf("probed model %q, want llama3:latest", modelID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected one warmup probe for the selected model")
	}

	select {
	case extra := <-probed:
		t.Fatalf("warmup probed again: %s", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDiscoveryWarmup_RetriesAfterTransientFailure(t *testing.T) {
	var hits atomic.Int32
	probed := make(chan struct{}, 4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModel{{Name: "llama3:latest"}},
			})
		case "/v1/chat/completions":
			n := hits.Add(1)
			probed <- struct{}{}
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"calculator"}}]}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverOllama(t.Context(), srv.URL)

	selected, err := reg.GetModel("ollama", "llama3:latest")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}

	d.Warmup(t.Context(), selected)
	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first warmup probe")
	}
	// Allow failure path to clear warmingModels.
	time.Sleep(50 * time.Millisecond)

	if !selected.Capabilities.ToolCall {
		t.Fatal("transient failure must not permanently disable ToolCall")
	}
	d.warmedMu.Lock()
	marked := d.warmedModels[selected.ID]
	d.warmedMu.Unlock()
	if marked {
		t.Fatal("failed probe must leave model unmarked for retry")
	}

	d.Warmup(t.Context(), selected)
	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected retry warmup probe")
	}
	time.Sleep(50 * time.Millisecond)

	updated, err := reg.GetModel("ollama", "llama3:latest")
	if err != nil {
		t.Fatalf("model not found after retry: %v", err)
	}
	if !updated.Capabilities.ToolCall {
		t.Fatal("expected ToolCall still true after successful retry")
	}
}
