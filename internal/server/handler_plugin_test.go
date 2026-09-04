package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

func testServerWithPlugins(t *testing.T, registry []plugin.RegistryEntry) *Server {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { b.Close() })

	db := testDB(t)
	reg := provider.NewRegistry()
	mgr := plugin.NewManagerWithRegistry(registry)

	srv := New(Config{}, Dependencies{
		Bus:           b,
		DB:            db,
		Registry:      reg,
		PluginManager: mgr,
	})
	return srv
}

func TestPluginList_Empty(t *testing.T) {
	srv := testServerWithPlugins(t, nil)

	req := httptest.NewRequest("GET", "/plugin", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body []plugin.PluginInfo
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 0 {
		t.Errorf("expected empty array, got %d items", len(body))
	}
}

func TestPluginList_NilManager(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: provider.NewRegistry()})

	req := httptest.NewRequest("GET", "/plugin", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body []plugin.PluginInfo
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 0 {
		t.Errorf("expected empty array, got %d items", len(body))
	}
}

func TestPluginRegistry_WithEntries(t *testing.T) {
	entries := []plugin.RegistryEntry{
		{Name: "foo", Description: "Foo plugin", Package: "@tinycode/foo"},
		{Name: "bar", Description: "Bar plugin", Package: "@tinycode/bar"},
	}
	srv := testServerWithPlugins(t, entries)

	req := httptest.NewRequest("GET", "/plugin/registry", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body []plugin.RegistryEntry
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(body))
	}
	if body[0].Name != "foo" {
		t.Errorf("expected foo, got %s", body[0].Name)
	}
}

func TestPluginLoad(t *testing.T) {
	srv := testServerWithPlugins(t, nil)

	req := httptest.NewRequest("POST", "/plugin/load", strings.NewReader(`{"name":"test-plugin"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var info plugin.PluginInfo
	json.NewDecoder(w.Body).Decode(&info)
	if info.Name != "test-plugin" {
		t.Errorf("expected name test-plugin, got %s", info.Name)
	}
	if info.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestPluginLoad_MissingName(t *testing.T) {
	srv := testServerWithPlugins(t, nil)

	req := httptest.NewRequest("POST", "/plugin/load", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPluginUnload(t *testing.T) {
	srv := testServerWithPlugins(t, nil)

	// Load first
	req := httptest.NewRequest("POST", "/plugin/load", strings.NewReader(`{"name":"to-unload"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var loaded plugin.PluginInfo
	json.NewDecoder(w.Body).Decode(&loaded)

	// Unload
	req = httptest.NewRequest("POST", "/plugin/unload", strings.NewReader(`{"id":"`+loaded.ID+`"}`))
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify empty
	req = httptest.NewRequest("GET", "/plugin", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var list []plugin.PluginInfo
	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected 0 plugins after unload, got %d", len(list))
	}
}

func TestPluginUnload_NotFound(t *testing.T) {
	srv := testServerWithPlugins(t, nil)

	req := httptest.NewRequest("POST", "/plugin/unload", strings.NewReader(`{"id":"plg_nonexistent"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
