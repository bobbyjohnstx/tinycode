package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOAuthFlow_StartAuth_GeneratesState(t *testing.T) {
	// Use a temp dir for OAuth state persistence.
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	flow := NewOAuthFlow()
	defer flow.Close()

	cfg := &OAuthConfig{
		ClientID: "test-client",
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: "https://auth.example.com/token",
		Scopes:   []string{"read", "write"},
	}

	authURL, state, err := flow.StartAuth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartAuth failed: %v", err)
	}

	// State should be a 64-char hex string (32 bytes).
	if len(state) != 64 {
		t.Errorf("state length = %d, want 64", len(state))
	}

	// Auth URL should contain PKCE challenge params.
	if authURL == "" {
		t.Fatal("authURL is empty")
	}
	for _, param := range []string{"code_challenge=", "code_challenge_method=S256", "state=", "client_id=test-client", "scope=read+write"} {
		if !containsParam(authURL, param) {
			t.Errorf("authURL missing param %q: %s", param, authURL)
		}
	}

	// State should be persisted.
	stored, err := loadOAuthState("test-client")
	if err != nil {
		t.Fatalf("loadOAuthState failed: %v", err)
	}
	if stored == nil {
		t.Fatal("stored state is nil")
	}
	if stored.State != state {
		t.Errorf("stored state = %q, want %q", stored.State, state)
	}
	if stored.CodeVerifier == "" {
		t.Error("stored CodeVerifier is empty")
	}
}

func containsParam(url, param string) bool {
	return param != "" && strings.Contains(url, param)
}

func TestOAuthFlow_HandleCallback_ValidState(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	state := "test-state-123"
	ch := make(chan oauthCallback, 1)

	flow.mu.Lock()
	flow.pending[state] = ch
	flow.mu.Unlock()

	// Simulate a callback request.
	req := httptest.NewRequest("GET", "/mcp/oauth/callback?code=auth-code-456&state="+state, nil)
	w := httptest.NewRecorder()

	flow.handleCallback(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	select {
	case cb := <-ch:
		if cb.code != "auth-code-456" {
			t.Errorf("code = %q, want %q", cb.code, "auth-code-456")
		}
		if cb.state != state {
			t.Errorf("state = %q, want %q", cb.state, state)
		}
	default:
		t.Fatal("no callback received on channel")
	}
}

func TestOAuthFlow_HandleCallback_InvalidState(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	// No pending state registered — should reject.
	req := httptest.NewRequest("GET", "/mcp/oauth/callback?code=some-code&state=unknown-state", nil)
	w := httptest.NewRecorder()

	flow.handleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestOAuthFlow_HandleCallback_MissingParams(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	// No code or state — should return 400.
	req := httptest.NewRequest("GET", "/mcp/oauth/callback", nil)
	w := httptest.NewRecorder()

	flow.handleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExchangeCode_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// Pre-save a code verifier.
	if err := saveOAuthState("test-client", &OAuthState{
		CodeVerifier: "test-verifier",
		State:        "test-state",
	}); err != nil {
		t.Fatalf("saveOAuthState: %v", err)
	}

	// Mock token endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.FormValue("code"); got != "auth-code" {
			t.Errorf("code = %q, want auth-code", got)
		}
		if got := r.FormValue("code_verifier"); got != "test-verifier" {
			t.Errorf("code_verifier = %q, want test-verifier", got)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token-123",
			"refresh_token": "refresh-token-456",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	flow := NewOAuthFlow()
	defer flow.Close()

	cfg := &OAuthConfig{
		ClientID: "test-client",
		TokenURL: srv.URL,
	}

	tokens, err := flow.ExchangeCode(context.Background(), cfg, "auth-code")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}

	if tokens.AccessToken != "access-token-123" {
		t.Errorf("AccessToken = %q, want access-token-123", tokens.AccessToken)
	}
	if tokens.RefreshToken != "refresh-token-456" {
		t.Errorf("RefreshToken = %q, want refresh-token-456", tokens.RefreshToken)
	}
	if tokens.ExpiresAt == 0 {
		t.Error("ExpiresAt should be set")
	}
}

func TestExchangeCode_ErrorResponse(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	defer srv.Close()

	flow := NewOAuthFlow()
	defer flow.Close()

	cfg := &OAuthConfig{
		ClientID: "test-client",
		TokenURL: srv.URL,
	}

	_, err := flow.ExchangeCode(context.Background(), cfg, "bad-code")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("error = %q, want to contain 'status 400'", err.Error())
	}
}

func TestExchangeCode_MissingTokenURL(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	cfg := &OAuthConfig{
		ClientID: "test-client",
		TokenURL: "",
	}

	_, err := flow.ExchangeCode(context.Background(), cfg, "code")
	if err == nil {
		t.Fatal("expected error for missing token URL")
	}
}

func TestSaveAndLoadOAuthState_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	original := &OAuthState{
		Tokens: &OAuthTokens{
			AccessToken:  "at-123",
			RefreshToken: "rt-456",
			ExpiresAt:    time.Now().Unix() + 3600,
		},
		CodeVerifier: "verifier-789",
		State:        "state-abc",
	}

	if err := saveOAuthState("roundtrip-client", original); err != nil {
		t.Fatalf("saveOAuthState: %v", err)
	}

	// Verify file permissions.
	path := filepath.Join(tmpDir, "tinycode", "mcp-auth.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 600", perm)
	}

	loaded, err := loadOAuthState("roundtrip-client")
	if err != nil {
		t.Fatalf("loadOAuthState: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded state is nil")
	}
	if loaded.Tokens == nil {
		t.Fatal("loaded tokens is nil")
	}
	if loaded.Tokens.AccessToken != original.Tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.Tokens.AccessToken, original.Tokens.AccessToken)
	}
	if loaded.Tokens.RefreshToken != original.Tokens.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.Tokens.RefreshToken, original.Tokens.RefreshToken)
	}
	if loaded.CodeVerifier != original.CodeVerifier {
		t.Errorf("CodeVerifier = %q, want %q", loaded.CodeVerifier, original.CodeVerifier)
	}
	if loaded.State != original.State {
		t.Errorf("State = %q, want %q", loaded.State, original.State)
	}
}

func TestSaveOAuthState_MultipleClients(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	if err := saveOAuthState("client-a", &OAuthState{State: "state-a"}); err != nil {
		t.Fatalf("saveOAuthState client-a: %v", err)
	}
	if err := saveOAuthState("client-b", &OAuthState{State: "state-b"}); err != nil {
		t.Fatalf("saveOAuthState client-b: %v", err)
	}

	a, _ := loadOAuthState("client-a")
	b, _ := loadOAuthState("client-b")

	if a == nil || a.State != "state-a" {
		t.Errorf("client-a state = %v, want state-a", a)
	}
	if b == nil || b.State != "state-b" {
		t.Errorf("client-b state = %v, want state-b", b)
	}
}

func TestEnsureCallbackServer_Idempotent(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	cfg := &OAuthConfig{
		ClientID:    "test-client",
		CallbackURL: "http://127.0.0.1:0/mcp/oauth/callback",
	}

	// Use port 0 to let OS assign a free port. But the implementation
	// parses the port from CallbackURL, so use a specific free port.
	// Instead, test with default behavior: first call starts server,
	// second call is a no-op.

	// We can't easily test with default port (19876) as it may be in use.
	// Instead, test the idempotency of the running flag.
	flow.mu.Lock()
	flow.running = true
	flow.mu.Unlock()

	// Second call should return nil without starting anything new.
	err := flow.ensureCallbackServer(cfg)
	if err != nil {
		t.Fatalf("ensureCallbackServer (idempotent) returned error: %v", err)
	}

	// Verify still marked as running.
	flow.mu.Lock()
	running := flow.running
	flow.mu.Unlock()
	if !running {
		t.Error("expected running to remain true")
	}
}

func TestOAuthFlow_StartAuth_NilConfig(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	_, _, err := flow.StartAuth(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestOAuthFlow_WaitForCallback_ReceivesCode(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	state := "test-wait-state"

	// Send callback after WaitForCallback registers its pending channel.
	go func() {
		for {
			flow.mu.Lock()
			ch, ok := flow.pending[state]
			flow.mu.Unlock()
			if ok {
				ch <- oauthCallback{code: "callback-code-789", state: state}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	code, err := flow.WaitForCallback(ctx, state)
	if err != nil {
		t.Fatalf("WaitForCallback: %v", err)
	}
	if code != "callback-code-789" {
		t.Errorf("code = %q, want callback-code-789", code)
	}
}

func TestOAuthFlow_WaitForCallback_CancelledContext(t *testing.T) {
	flow := NewOAuthFlow()
	defer flow.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := flow.WaitForCallback(ctx, "cancelled-state")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}
