package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/lsp"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func TestHandleLSP_NoManager(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: provider.NewRegistry()})

	req := httptest.NewRequest("GET", "/lsp", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false when manager missing", body["enabled"])
	}
}

func TestHandleLSP_WithManager(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	mgr := lsp.NewManager(t.TempDir(), nil)
	t.Cleanup(mgr.Close)

	srv := New(Config{}, Dependencies{
		Bus:        b,
		DB:         db,
		Registry:   provider.NewRegistry(),
		LSPManager: mgr,
	})

	req := httptest.NewRequest("GET", "/lsp", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Enabled   bool     `json:"enabled"`
		Languages []string `json:"languages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled {
		t.Error("expected enabled=true")
	}
	if body.Languages == nil {
		t.Error("languages should be a non-null array")
	}
}

func TestHandleLSP_Disabled(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	enabled := false
	mgr := lsp.NewManager(t.TempDir(), &lsp.Config{Enabled: &enabled})
	t.Cleanup(mgr.Close)

	srv := New(Config{}, Dependencies{
		Bus:        b,
		DB:         db,
		Registry:   provider.NewRegistry(),
		LSPManager: mgr,
	})

	req := httptest.NewRequest("GET", "/lsp", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false", body["enabled"])
	}
}
