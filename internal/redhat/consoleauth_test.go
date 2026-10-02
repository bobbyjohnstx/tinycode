package redhat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewConsoleAuthClient_DefaultSSOURL(t *testing.T) {
	client := NewConsoleAuthClient(ConsoleAuthConfig{
		OfflineToken: "tok-123",
	})
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.ssoURL != DefaultSSOURL {
		t.Errorf("got ssoURL %q, want %q", client.ssoURL, DefaultSSOURL)
	}
}

func TestNewConsoleAuthClient_CustomSSOURL(t *testing.T) {
	client := NewConsoleAuthClient(ConsoleAuthConfig{
		OfflineToken: "tok-123",
		SSOURL:       "https://custom-sso.example.com",
	})
	if client.ssoURL != "https://custom-sso.example.com" {
		t.Errorf("got ssoURL %q, want %q", client.ssoURL, "https://custom-sso.example.com")
	}
}

func TestConsoleAuthClient_IsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{"configured", "tok-123", true},
		{"not configured", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewConsoleAuthClient(ConsoleAuthConfig{OfflineToken: tt.token})
			if got := client.IsConfigured(); got != tt.expected {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func newSSOServer(t *testing.T, callCount *atomic.Int32, accessToken string, expiresIn int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)

		if r.URL.Path != tokenPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("got grant_type %q, want %q", r.FormValue("grant_type"), "refresh_token")
		}
		if r.FormValue("client_id") != "cloud-services" {
			t.Errorf("got client_id %q, want %q", r.FormValue("client_id"), "cloud-services")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": accessToken,
			"expires_in":   expiresIn,
		})
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestConsoleAuthClient_ExchangeToken(t *testing.T) {
	var callCount atomic.Int32
	ts := newSSOServer(t, &callCount, "access-tok-1", 3600)

	client := NewConsoleAuthClient(ConsoleAuthConfig{SSOURL: ts.URL, OfflineToken: "offline-tok"})

	token, expiresIn, err := client.exchangeToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "access-tok-1" {
		t.Errorf("got token %q, want %q", token, "access-tok-1")
	}
	if expiresIn != 3600 {
		t.Errorf("got expiresIn %d, want %d", expiresIn, 3600)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 call, got %d", callCount.Load())
	}
}

func TestConsoleAuthClient_ExchangeToken_FormData(t *testing.T) {
	var gotRefreshToken string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotRefreshToken = r.FormValue("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"expires_in":   300,
		})
	}))
	defer ts.Close()

	client := NewConsoleAuthClient(ConsoleAuthConfig{SSOURL: ts.URL, OfflineToken: "my-offline-token"})

	_, _, err := client.exchangeToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRefreshToken != "my-offline-token" {
		t.Errorf("got refresh_token %q, want %q", gotRefreshToken, "my-offline-token")
	}
}

func TestConsoleAuthClient_ExchangeToken_ErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer ts.Close()

	client := NewConsoleAuthClient(ConsoleAuthConfig{SSOURL: ts.URL, OfflineToken: "bad-token"})

	_, _, err := client.exchangeToken(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestConsoleAuthClient_GetAccessToken_CachesToken(t *testing.T) {
	var callCount atomic.Int32
	ts := newSSOServer(t, &callCount, "cached-tok", 3600)

	client := NewConsoleAuthClient(ConsoleAuthConfig{SSOURL: ts.URL, OfflineToken: "offline-tok"})

	ctx := context.Background()

	tok1, err := client.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if tok1 != "cached-tok" {
		t.Errorf("first call: got %q, want %q", tok1, "cached-tok")
	}

	tok2, err := client.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if tok2 != "cached-tok" {
		t.Errorf("second call: got %q, want %q", tok2, "cached-tok")
	}

	if callCount.Load() != 1 {
		t.Errorf("expected 1 server call (cached), got %d", callCount.Load())
	}
}

func TestConsoleAuthClient_GetAccessToken_ReexchangesOnExpiry(t *testing.T) {
	var callCount atomic.Int32
	ts := newSSOServer(t, &callCount, "new-tok", 3600)

	client := NewConsoleAuthClient(ConsoleAuthConfig{SSOURL: ts.URL, OfflineToken: "offline-tok"})
	client.accessToken = "old-tok"
	client.expiresAt = time.Now().Add(-1 * time.Second)

	tok, err := client.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "new-tok" {
		t.Errorf("got %q, want %q", tok, "new-tok")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 server call for re-exchange, got %d", callCount.Load())
	}
}

func TestConsoleAuthClient_GetAccessToken_ExpiryBufferTriggersRefresh(t *testing.T) {
	var callCount atomic.Int32
	ts := newSSOServer(t, &callCount, "refreshed-tok", 3600)

	client := NewConsoleAuthClient(ConsoleAuthConfig{SSOURL: ts.URL, OfflineToken: "offline-tok"})
	client.accessToken = "about-to-expire"
	client.expiresAt = time.Now().Add(30 * time.Second)

	tok, err := client.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "refreshed-tok" {
		t.Errorf("got %q, want %q (should have refreshed due to 60s buffer)", tok, "refreshed-tok")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 server call, got %d", callCount.Load())
	}
}

func TestNewConsoleAPIClient_DefaultBaseURL(t *testing.T) {
	cfg := ConsoleAuthConfig{OfflineToken: "tok"}
	client := NewConsoleAPIClient(cfg, "/api/v1", nil)
	if client == nil {
		t.Fatal("expected non-nil APIClient")
	}
	if client.baseURL != DefaultAPIBaseURL+"/api/v1" {
		t.Errorf("got baseURL %q, want %q", client.baseURL, DefaultAPIBaseURL+"/api/v1")
	}
}

func TestNewConsoleAPIClient_CustomBaseURL(t *testing.T) {
	cfg := ConsoleAuthConfig{
		OfflineToken: "tok",
		APIBaseURL:   "https://custom.example.com",
	}
	client := NewConsoleAPIClient(cfg, "/svc", nil)
	if client.baseURL != "https://custom.example.com/svc" {
		t.Errorf("got baseURL %q, want %q", client.baseURL, "https://custom.example.com/svc")
	}
}

func TestNewConsoleAPIClient_WithExistingAuthClient(t *testing.T) {
	auth := NewConsoleAuthClient(ConsoleAuthConfig{OfflineToken: "tok"})
	cfg := ConsoleAuthConfig{OfflineToken: "tok"}
	client := NewConsoleAPIClient(cfg, "/api", auth)
	if client == nil {
		t.Fatal("expected non-nil APIClient")
	}
	if client.tokenFn == nil {
		t.Error("expected tokenFn to be set from auth client")
	}
}
