package redhat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAPIClient_Defaults(t *testing.T) {
	c := NewAPIClient(APIClientConfig{BaseURL: "http://example.com"})
	if c.maxRetries != 1 {
		t.Errorf("maxRetries = %d, want 1", c.maxRetries)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.httpClient.Timeout)
	}
}

func TestNewAPIClient_CustomValues(t *testing.T) {
	c := NewAPIClient(APIClientConfig{
		BaseURL:    "http://example.com",
		MaxRetries: 5,
		Timeout:    10 * time.Second,
	})
	if c.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", c.maxRetries)
	}
	if c.httpClient.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", c.httpClient.Timeout)
	}
}

func TestNewAPIClient_TrimsTrailingSlash(t *testing.T) {
	c := NewAPIClient(APIClientConfig{BaseURL: "http://example.com///"})
	if c.baseURL != "http://example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://example.com")
	}
}

func TestNewAPIClient_ZeroRetries(t *testing.T) {
	c := NewAPIClient(APIClientConfig{BaseURL: "http://example.com", MaxRetries: 0})
	if c.maxRetries != 1 {
		t.Errorf("maxRetries = %d, want 1 (default for <=0)", c.maxRetries)
	}
}

func TestAPIClient_Get_QueryParams(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	resp, err := c.Get(context.Background(), "/api/data", map[string]string{"foo": "bar", "baz": "qux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/data" {
		t.Errorf("path = %q, want /api/data", gotPath)
	}
	if gotQuery == "" {
		t.Error("expected query string, got empty")
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}

func TestAPIClient_Get_NoQueryParams(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	_, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRawQuery != "" {
		t.Errorf("expected no query string, got %q", gotRawQuery)
	}
}

func TestAPIClient_Post_JSONBody(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	payload := map[string]string{"name": "test"}
	resp, err := c.Post(context.Background(), "/items", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	var parsed map[string]string
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if parsed["name"] != "test" {
		t.Errorf("body name = %q, want test", parsed["name"])
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}

func TestAPIClient_Put_JSONBody(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"updated":true}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	_, err := c.Put(context.Background(), "/items/1", map[string]string{"name": "updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
}

func TestAPIClient_Delete(t *testing.T) {
	var gotMethod string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	_, err := c.Delete(context.Background(), "/items/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotContentType != "" {
		t.Errorf("Content-Type should be empty for DELETE (no body), got %q", gotContentType)
	}
}

func TestAPIClient_AuthTokenInjection(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{
		BaseURL: srv.URL,
		TokenFn: func(ctx context.Context) (string, error) {
			return "my-secret-token", nil
		},
	})
	_, err := c.Get(context.Background(), "/secure", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

func TestAPIClient_EmptyTokenSkipsHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{
		BaseURL: srv.URL,
		TokenFn: func(ctx context.Context) (string, error) {
			return "", nil
		},
	})
	_, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header when token is empty, got %q", gotAuth)
	}
}

func TestAPIClient_NoTokenFnSkipsHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	_, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header when tokenFn is nil, got %q", gotAuth)
	}
}

func TestAPIClient_CustomHeaders(t *testing.T) {
	var gotCustom, gotAnother string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustom = r.Header.Get("X-Custom")
		gotAnother = r.Header.Get("X-Another")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{
		BaseURL: srv.URL,
		Headers: map[string]string{
			"X-Custom":  "value1",
			"X-Another": "value2",
		},
	})
	_, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCustom != "value1" {
		t.Errorf("X-Custom = %q, want value1", gotCustom)
	}
	if gotAnother != "value2" {
		t.Errorf("X-Another = %q, want value2", gotAnother)
	}
}

func TestAPIClient_RetryOn401(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`unauthorized`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{
		BaseURL:    srv.URL,
		MaxRetries: 1,
	})
	resp, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts = %d, want 2 (initial + 1 retry)", got)
	}
}

func TestAPIClient_AllRetriesExhausted401(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`unauthorized`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{
		BaseURL:    srv.URL,
		MaxRetries: 2,
	})
	resp, err := c.Get(context.Background(), "/test", nil)
	// After exhausting retries on 401, it falls through to the status check.
	// 401 is not in [200,300) so it returns an error.
	if err == nil {
		t.Fatal("expected error for exhausted 401 retries")
	}
	if resp != nil {
		t.Errorf("expected nil response on error, got status %d", resp.Status)
	}
	// 1 initial + 2 retries = 3 total attempts
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestAPIClient_ErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{"bad request", http.StatusBadRequest, `{"error":"invalid"}`},
		{"not found", http.StatusNotFound, `not found`},
		{"server error", http.StatusInternalServerError, `internal error`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
			_, err := c.Get(context.Background(), "/test", nil)
			if err == nil {
				t.Fatal("expected error for non-2xx status")
			}
			expected := fmt.Sprintf("HTTP %d: %s", tt.statusCode, tt.body)
			if err.Error() != expected {
				t.Errorf("error = %q, want %q", err.Error(), expected)
			}
		})
	}
}

func TestAPIClient_TokenFnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{
		BaseURL: srv.URL,
		TokenFn: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("token provider failed")
		},
	})
	_, err := c.Get(context.Background(), "/test", nil)
	if err == nil {
		t.Fatal("expected error when TokenFn fails")
	}
	if got := err.Error(); got != "getting auth token: token provider failed" {
		t.Errorf("error = %q, want %q", got, "getting auth token: token provider failed")
	}
}

func TestDecodeResponse_Success(t *testing.T) {
	resp := &APIResponse{
		Data:   json.RawMessage(`{"name":"alice","age":30}`),
		Status: 200,
	}
	type person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	result, err := DecodeResponse[person](resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "alice" {
		t.Errorf("Name = %q, want alice", result.Name)
	}
	if result.Age != 30 {
		t.Errorf("Age = %d, want 30", result.Age)
	}
}

func TestDecodeResponse_Error(t *testing.T) {
	resp := &APIResponse{
		Data:   json.RawMessage(`not-json`),
		Status: 200,
	}
	type dummy struct{ X int }
	_, err := DecodeResponse[dummy](resp)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodeResponse_Slice(t *testing.T) {
	resp := &APIResponse{
		Data:   json.RawMessage(`[1,2,3]`),
		Status: 200,
	}
	result, err := DecodeResponse[[]int](resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 || result[0] != 1 || result[2] != 3 {
		t.Errorf("result = %v, want [1 2 3]", result)
	}
}

func TestAPIClient_GetNoContentType(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	_, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "" {
		t.Errorf("GET should not set Content-Type, got %q", gotContentType)
	}
}

func TestAPIClient_ResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "abc-123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	resp, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Headers.Get("X-Request-Id") != "abc-123" {
		t.Errorf("X-Request-Id = %q, want abc-123", resp.Headers.Get("X-Request-Id"))
	}
}

func TestAPIResponse_DataPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value","nested":{"a":1}}`))
	}))
	defer srv.Close()

	c := NewAPIClient(APIClientConfig{BaseURL: srv.URL})
	resp, err := c.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		t.Fatalf("failed to unmarshal Data: %v", err)
	}
	if _, ok := raw["nested"]; !ok {
		t.Error("expected nested key in response data")
	}
}
