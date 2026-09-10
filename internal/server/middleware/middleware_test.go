package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultCORSConfig(t *testing.T) {
	cfg := DefaultCORSConfig()

	if cfg.AllowOriginFunc == nil {
		t.Error("AllowOriginFunc should be set")
	}
	if len(cfg.AllowOrigins) != 0 {
		t.Errorf("AllowOrigins = %v, want empty (localhost-only via AllowOriginFunc)", cfg.AllowOrigins)
	}

	expectedMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	if len(cfg.AllowMethods) != len(expectedMethods) {
		t.Fatalf("AllowMethods length = %d, want %d", len(cfg.AllowMethods), len(expectedMethods))
	}
	for i, m := range expectedMethods {
		if cfg.AllowMethods[i] != m {
			t.Errorf("AllowMethods[%d] = %q, want %q", i, cfg.AllowMethods[i], m)
		}
	}

	expectedHeaders := []string{"Content-Type", "Authorization", "X-Request-ID"}
	if len(cfg.AllowHeaders) != len(expectedHeaders) {
		t.Fatalf("AllowHeaders length = %d, want %d", len(cfg.AllowHeaders), len(expectedHeaders))
	}
	for i, h := range expectedHeaders {
		if cfg.AllowHeaders[i] != h {
			t.Errorf("AllowHeaders[%d] = %q, want %q", i, cfg.AllowHeaders[i], h)
		}
	}

	if cfg.MaxAge != 86400 {
		t.Errorf("MaxAge = %d, want 86400", cfg.MaxAge)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	handler := CORS(DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("Access-Control-Allow-Origin should be empty, got %q", v)
	}
}

func TestCORS_DefaultAllowsLocalhost(t *testing.T) {
	handler := CORS(DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "http://localhost:3000" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", v, "http://localhost:3000")
	}
	if v := rec.Header().Get("Access-Control-Allow-Credentials"); v != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", v, "true")
	}
	if v := rec.Header().Get("Access-Control-Allow-Methods"); v == "" {
		t.Error("Access-Control-Allow-Methods should be set")
	}
	if v := rec.Header().Get("Access-Control-Allow-Headers"); v == "" {
		t.Error("Access-Control-Allow-Headers should be set")
	}
}

func TestCORS_DefaultAllows127001(t *testing.T) {
	handler := CORS(DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://127.0.0.1:4096")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "http://127.0.0.1:4096" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", v, "http://127.0.0.1:4096")
	}
}

func TestCORS_DefaultRejectsExternalOrigin(t *testing.T) {
	handler := CORS(DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("Access-Control-Allow-Origin should be empty for external origin, got %q", v)
	}
}

func TestCORS_WildcardAllowsAnyOrigin(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST"},
		AllowHeaders: []string{"Content-Type"},
	}
	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", v, "https://example.com")
	}
}

func TestCORS_SpecificOrigin_Matching(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{"https://allowed.com", "https://also-allowed.com"},
		AllowMethods: []string{"GET", "POST"},
		AllowHeaders: []string{"Content-Type"},
		MaxAge:       3600,
	}
	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://allowed.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "https://allowed.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", v, "https://allowed.com")
	}
}

func TestCORS_SpecificOrigin_NotMatching(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{"https://allowed.com"},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"Content-Type"},
	}
	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://denied.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("Access-Control-Allow-Origin should be empty for denied origin, got %q", v)
	}
}

func TestCORS_OptionsPreflight(t *testing.T) {
	cfg := DefaultCORSConfig()
	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if v := rec.Header().Get("Access-Control-Max-Age"); v != "86400" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", v, "86400")
	}
	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "http://localhost:3000" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", v, "http://localhost:3000")
	}
}

func TestCORS_OptionsNoMaxAge(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{},
		MaxAge:       0,
	}
	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if v := rec.Header().Get("Access-Control-Max-Age"); v != "" {
		t.Errorf("Access-Control-Max-Age should be empty when MaxAge=0, got %q", v)
	}
}

func TestCORS_NonOptionsCallsNextHandler(t *testing.T) {
	called := false
	handler := CORS(DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler should be called for non-OPTIONS request")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := SecurityHeaders(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"X-XSS-Protection":      "1; mode=block",
		"Referrer-Policy":       "strict-origin-when-cross-origin",
	}

	for header, want := range expected {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSecurityHeaders_CallsNext(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("hello"))
	})
	handler := SecurityHeaders(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler should be called")
	}
	if rec.Body.String() != "hello" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "hello")
	}
}
