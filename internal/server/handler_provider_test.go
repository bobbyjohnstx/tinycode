package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func TestHandleProviderList_RedactsAPIKey(t *testing.T) {
	srv, _ := testServer(t)
	srv.deps.Registry.Register(&provider.Info{
		ID:   "openrouter",
		Name: "OpenRouter",
		Options: map[string]any{
			"apiKey":        "sk-secret",
			"api_key":       "sk-secret-alt",
			"Authorization": "Bearer sk-secret",
			"baseURL":       "https://openrouter.ai/api",
		},
		Models: map[string]*provider.Model{
			"m": {ID: "m", ProviderID: "openrouter", Name: "m"},
		},
	})

	req := httptest.NewRequest("GET", "/provider", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "sk-secret") {
		t.Fatalf("response leaked secret: %s", body)
	}

	var resp struct {
		All []map[string]any `json:"all"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, p := range resp.All {
		if p["id"] != "openrouter" {
			continue
		}
		found = true
		opts, _ := p["options"].(map[string]any)
		if _, ok := opts["apiKey"]; ok {
			t.Error("apiKey present in list response")
		}
		if _, ok := opts["api_key"]; ok {
			t.Error("api_key present in list response")
		}
		if _, ok := opts["Authorization"]; ok {
			t.Error("Authorization present in list response")
		}
		if opts["baseURL"] != "https://openrouter.ai/api" {
			t.Errorf("baseURL = %v, want preserved", opts["baseURL"])
		}
	}
	if !found {
		t.Fatal("openrouter provider missing from list")
	}

	// Registry must still hold the key for balance/auth.
	info, err := srv.deps.Registry.GetProvider("openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if info.Options["apiKey"] != "sk-secret" {
		t.Errorf("registry apiKey mutated, got %v", info.Options["apiKey"])
	}
}

func TestHandleProviderGet_RedactsAPIKey(t *testing.T) {
	srv, _ := testServer(t)
	srv.deps.Registry.Register(&provider.Info{
		ID:      "openrouter",
		Name:    "OpenRouter",
		Options: map[string]any{"apiKey": "sk-get-secret"},
		Models:  map[string]*provider.Model{},
	})

	req := httptest.NewRequest("GET", "/provider/openrouter", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "sk-get-secret") {
		t.Fatalf("get leaked secret: %s", w.Body.String())
	}
}

func TestHandleAuthDelete_ClearsOptionsAndOpenRouterEnv(t *testing.T) {
	srv, _ := testServer(t)
	t.Setenv("OPENROUTER_API_KEY", "or-to-clear")

	srv.deps.Registry.Register(&provider.Info{
		ID:      "openrouter",
		Name:    "OpenRouter",
		Options: map[string]any{"apiKey": "or-to-clear", "baseURL": "https://openrouter.ai/api"},
		Models:  map[string]*provider.Model{},
	})
	srv.credentials.Set("openrouter", map[string]string{"apiKey": "or-to-clear"})

	req := httptest.NewRequest("DELETE", "/auth/openrouter", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	if _, ok := srv.credentials.Get("openrouter"); ok {
		t.Fatal("expected credentials deleted")
	}
	info, err := srv.deps.Registry.GetProvider("openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := info.Options["apiKey"]; ok {
		t.Error("expected apiKey removed from Options")
	}
	if info.Options["baseURL"] != "https://openrouter.ai/api" {
		t.Error("expected non-secret options preserved")
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "" {
		t.Errorf("expected OPENROUTER_API_KEY unset, got %q", got)
	}
}
