package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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
