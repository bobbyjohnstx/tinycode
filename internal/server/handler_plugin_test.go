package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

func testServerWithPlugins(t *testing.T) *Server {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { b.Close() })

	db := testDB(t)
	reg := provider.NewRegistry()
	mgr := plugin.NewManager(slog.Default())

	srv := New(Config{}, Dependencies{
		Bus:           b,
		DB:            db,
		Registry:      reg,
		PluginManager: mgr,
	})
	return srv
}

func TestPluginList_Empty(t *testing.T) {
	srv := testServerWithPlugins(t)

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

func TestPluginRegistry(t *testing.T) {
	srv := testServerWithPlugins(t)

	req := httptest.NewRequest("GET", "/plugin/registry", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body []plugin.RegistryEntry
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) == 0 {
		t.Error("expected non-empty registry")
	}
}

func TestPluginLoad_MissingName(t *testing.T) {
	srv := testServerWithPlugins(t)

	req := httptest.NewRequest("POST", "/plugin/load", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPluginLoad_UnknownPlugin(t *testing.T) {
	srv := testServerWithPlugins(t)

	req := httptest.NewRequest("POST", "/plugin/load", strings.NewReader(`{"name":"nonexistent-plugin"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unresolvable plugin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPluginUnload_NotFound(t *testing.T) {
	srv := testServerWithPlugins(t)

	req := httptest.NewRequest("POST", "/plugin/unload", strings.NewReader(`{"id":"plg_nonexistent"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPluginLoad_NilManager(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: provider.NewRegistry()})

	req := httptest.NewRequest("POST", "/plugin/load", strings.NewReader(`{"name":"test"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestPluginUnload_NilManager(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: provider.NewRegistry()})

	req := httptest.NewRequest("POST", "/plugin/unload", strings.NewReader(`{"id":"plg_test"}`))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
