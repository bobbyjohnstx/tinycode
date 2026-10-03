package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRespondJSON_SetsContentTypeAndStatus(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("body[key] = %q, want %q", body["key"], "value")
	}
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

func TestRespondJSON_WithStruct(t *testing.T) {
	type resp struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusCreated, resp{Name: "test", Count: 42})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var body resp
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body.Name != "test" || body.Count != 42 {
		t.Errorf("body = %+v, want {Name:test Count:42}", body)
	}
}

func TestRespondError_Format(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("body[error] = %q, want %q", body["error"], "something went wrong")
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	type input struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alice","age":30}`))
	var v input
	if err := decodeJSON(w, req, &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Name != "alice" || v.Age != 30 {
		t.Errorf("decoded = %+v, want {Name:alice Age:30}", v)
	}
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not json`))
	var v map[string]string
	if err := decodeJSON(w, req, &v); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeJSON_OversizedBody(t *testing.T) {
	// Create a body larger than 1 MB
	body := strings.Repeat("x", 1<<20+1)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"data":"`+body+`"}`))
	var v map[string]string
	if err := decodeJSON(w, req, &v); err == nil {
		t.Error("expected error for oversized body")
	}
}

func TestRequestDirectory_PathTraversal(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "subdir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		query    string
		header   string
		wantDir  string
		wantFall bool // true if we expect fallback to root
	}{
		{
			name:    "no override returns fallback",
			wantDir: root,
		},
		{
			name:    "valid subdirectory via query param",
			query:   sub,
			wantDir: sub,
		},
		{
			name:     "traversal via query param returns fallback",
			query:    filepath.Join(root, "..", "..", "etc"),
			wantFall: true,
		},
		{
			name:     "absolute path outside root returns fallback",
			query:    "/etc",
			wantFall: true,
		},
		{
			name:    "valid subdirectory via header",
			header:  sub,
			wantDir: sub,
		},
		{
			name:     "traversal via header returns fallback",
			header:   filepath.Join(root, "..", "..", "etc"),
			wantFall: true,
		},
		{
			name:    "root itself is allowed",
			query:   root,
			wantDir: root,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/"
			if tt.query != "" {
				url = "/?directory=" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.header != "" {
				req.Header.Set("x-tinycode-directory", tt.header)
			}
			got := requestDirectory(req, root)

			if tt.wantFall {
				if got != root {
					t.Errorf("expected fallback to root %q, got %q", root, got)
				}
			} else {
				want, _ := filepath.Abs(tt.wantDir)
				if got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
		})
	}
}
