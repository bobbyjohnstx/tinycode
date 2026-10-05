package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func TestHandleAuthPut_StoresCredentials(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("PUT", "/auth/openai", strings.NewReader(`{"apiKey":"sk-test"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	creds, ok := srv.credentials.Get("openai")
	if !ok {
		t.Fatal("expected credentials to be stored")
	}
	if creds["apiKey"] != "sk-test" {
		t.Errorf("expected apiKey sk-test, got %q", creds["apiKey"])
	}
}

func TestHandleAuthPut_OpenRouterSetsEnvWithoutDiscovery(t *testing.T) {
	srv, _ := testServer(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	req := httptest.NewRequest("PUT", "/auth/openrouter", strings.NewReader(`{"apiKey":"or-key-123"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "or-key-123" {
		t.Errorf("expected OPENROUTER_API_KEY=or-key-123, got %q", got)
	}
}

func TestHandleAuthPut_OpenRouterTriggersDiscovery(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	disc := provider.NewDiscovery(reg, b)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer or-discover-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":             "openrouter/test-model",
					"name":           "Test Model",
					"context_length": 8192,
					"pricing": map[string]string{
						"prompt":     "0.000001",
						"completion": "0.000002",
					},
					"supported_parameters": []string{"tools"},
				},
			},
		})
	}))
	t.Cleanup(ts.Close)

	prev := provider.SetOpenRouterModelsURLForTest(ts.URL)
	t.Cleanup(func() { provider.SetOpenRouterModelsURLForTest(prev) })

	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg, Discovery: disc})
	t.Setenv("OPENROUTER_API_KEY", "")

	req := httptest.NewRequest("PUT", "/auth/OpenRouter", strings.NewReader(`{"api_key":"or-discover-key"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "or-discover-key" {
		t.Errorf("expected env key set, got %q", got)
	}

	info, err := reg.GetProvider("openrouter")
	if err != nil {
		t.Fatalf("expected openrouter provider registered via discovery: %v", err)
	}
	if _, ok := info.Models["openrouter/test-model"]; !ok {
		t.Fatal("expected discovered model to be registered")
	}
}

func TestHandleAuthPut_OpenRouterDiscoveryFailure(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	disc := provider.NewDiscovery(reg, b)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	prev := provider.SetOpenRouterModelsURLForTest(ts.URL)
	t.Cleanup(func() { provider.SetOpenRouterModelsURLForTest(prev) })

	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg, Discovery: disc})

	req := httptest.NewRequest("PUT", "/auth/openrouter", strings.NewReader(`{"apiKey":"bad"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}
