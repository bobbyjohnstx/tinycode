package console_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/server/console"
)

func TestHome_RendersOpsHTML(t *testing.T) {
	h := console.New(console.Deps{
		Version:   "test",
		Directory: "/tmp/proj",
		Registry:  provider.NewRegistry(),
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "tinycode ops") {
		t.Fatalf("expected ops brand in HTML, got %q", body)
	}
	if !strings.Contains(body, "/doctor") {
		t.Fatalf("expected nav link to doctor, got %q", body)
	}
}

func TestDoctor_RendersChecks(t *testing.T) {
	h := console.New(console.Deps{
		Version:  "test",
		Registry: provider.NewRegistry(),
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/doctor", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Database") {
		t.Fatalf("expected Database check, got %q", body)
	}
	if !strings.Contains(body, "Providers") {
		t.Fatalf("expected Providers check, got %q", body)
	}
}

func TestSessionDelete_RequiresConfirm(t *testing.T) {
	h := console.New(console.Deps{})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/sessions/abc/delete", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestProviders_EmptyTable(t *testing.T) {
	h := console.New(console.Deps{Registry: provider.NewRegistry()})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No providers") {
		t.Fatalf("expected empty providers message, got %q", rec.Body.String())
	}
}
