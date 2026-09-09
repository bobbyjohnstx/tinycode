package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWarmupProbe_ToolCallSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "calculator",
									"arguments": `{"expression":"2+2"}`,
								},
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	capable, err := WarmupProbe(t.Context(), nil, srv.URL, "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capable {
		t.Error("expected tool call capable, got false")
	}
}

func TestWarmupProbe_TextOnlyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "2+2 equals 4",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	capable, err := WarmupProbe(t.Context(), nil, srv.URL, "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capable {
		t.Error("expected not capable, got true")
	}
}

func TestWarmupProbe_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	_, err := WarmupProbe(ctx, nil, srv.URL, "test-model")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWarmupProbe_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not loaded", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := WarmupProbe(t.Context(), nil, srv.URL, "test-model")
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestWarmupRequest_Structure(t *testing.T) {
	req := warmupRequest("llama3:latest")
	if req["model"] != "llama3:latest" {
		t.Errorf("expected model llama3:latest, got %v", req["model"])
	}
	tools, ok := req["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatal("expected exactly 1 tool")
	}
	fn, ok := tools[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("expected function in tool")
	}
	if fn["name"] != "calculator" {
		t.Errorf("expected tool name 'calculator', got %v", fn["name"])
	}
}
