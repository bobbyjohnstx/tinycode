package static

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_DevDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>dev</html>"), 0o644)

	assetsDir := filepath.Join(dir, "assets")
	os.MkdirAll(assetsDir, 0o755)
	os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('dev')"), 0o644)

	handler := Handler(dir)

	t.Run("serves index.html at root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
		}
	})

	t.Run("serves asset with correct content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/javascript" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/javascript")
		}
	})

	t.Run("asset cache headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Errorf("Cache-Control = %q, want immutable cache header", cc)
		}
	})

	t.Run("SPA fallback for unknown paths", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some/deep/route", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 (SPA fallback)", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
		}
	})
}

func TestHandler_EmptyDevDir(t *testing.T) {
	handler := Handler("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Falls back to embedded FS — may be 404 if no dist/ embedded
	// Just verify it doesn't panic
	if rec.Code == 0 {
		t.Error("expected non-zero status code")
	}
}

func TestHandler_InvalidDevDir(t *testing.T) {
	handler := Handler("/nonexistent/path/that/does/not/exist")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Falls back to embedded FS — verify no panic
	if rec.Code == 0 {
		t.Error("expected non-zero status code")
	}
}

func TestContentTypes(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".js", "application/javascript"},
		{".css", "text/css"},
		{".html", "text/html; charset=utf-8"},
		{".json", "application/json"},
		{".svg", "image/svg+xml"},
		{".png", "image/png"},
		{".woff2", "font/woff2"},
		{".wasm", "application/wasm"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got, ok := contentTypes[tt.ext]
			if !ok {
				t.Fatalf("no content type for %q", tt.ext)
			}
			if got != tt.expected {
				t.Errorf("contentTypes[%q] = %q, want %q", tt.ext, got, tt.expected)
			}
		})
	}
}
