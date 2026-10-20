package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeCatalogIntoRegistry_ExactID(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Info{
		ID:   "openrouter",
		Name: "OpenRouter",
		Models: map[string]*Model{
			"openrouter/test": {
				ID:         "openrouter/test",
				ProviderID: "openrouter",
				Name:       "test",
				Limit:      ModelLimit{Context: 1000, Output: 100},
			},
		},
	})

	cat := catalogFile{
		"openrouter": {
			ID: "openrouter",
			Models: map[string]catalogModel{
				"openrouter/test": {
					ID:          "openrouter/test",
					Temperature: true,
					ToolCall:    true,
					Limit:       &catalogLimit{Context: 128000, Output: 8192},
					Cost:        &catalogCost{Input: 1, Output: 2},
				},
				"missing-model": {ID: "missing-model", Limit: &catalogLimit{Context: 99, Output: 9}},
			},
		},
	}

	n := MergeCatalogIntoRegistry(reg, cat)
	if n != 1 {
		t.Fatalf("merged = %d, want 1", n)
	}
	m, err := reg.GetModel("openrouter", "openrouter/test")
	if err != nil {
		t.Fatal(err)
	}
	if m.Limit.Context != 128000 || m.Limit.Output != 8192 {
		t.Fatalf("limit = %+v", m.Limit)
	}
	if m.Cost.Input != 1 || !m.Capabilities.ToolCall {
		t.Fatalf("cost/caps = %+v %+v", m.Cost, m.Capabilities)
	}
}

func TestLoadCatalog_FetchAPIJSON_FailOpen(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(catalogFile{
			"p": {ID: "p", Models: map[string]catalogModel{"m": {ID: "m", Limit: &catalogLimit{Context: 42, Output: 7}}}},
		})
	}))
	t.Cleanup(ts.Close)

	t.Setenv("TINYCODE_MODELS_URL", ts.URL)
	t.Setenv("TINYCODE_MODELS_PATH", "")
	t.Setenv("TINYCODE_DISABLE_MODELS_FETCH", "")
	t.Setenv("TINYCODE_DATA_DIR", t.TempDir())

	cat, err := LoadCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cat["p"].Models["m"].Limit.Context != 42 {
		t.Fatalf("catalog = %+v", cat)
	}

	// 500 → fail-open error (no cache yet)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(bad.Close)
	t.Setenv("TINYCODE_MODELS_URL", bad.URL)
	t.Setenv("TINYCODE_DATA_DIR", t.TempDir())
	_, err = LoadCatalog(context.Background())
	if err == nil {
		t.Fatal("expected error on 500 with no cache")
	}
}

func TestLoadCatalog_FromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cat.json")
	data, _ := json.Marshal(catalogFile{"x": {ID: "x", Models: map[string]catalogModel{}}})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TINYCODE_MODELS_PATH", path)
	t.Setenv("TINYCODE_MODELS_URL", "http://example.invalid")
	cat, err := LoadCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat["x"]; !ok {
		t.Fatal("expected path catalog")
	}
}
