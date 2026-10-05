package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	defaultCallbackPort = 19876
	oauthCallbackPath   = "/mcp/oauth/callback"
	callbackTimeout     = 5 * time.Minute
)

type OAuthTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

type OAuthState struct {
	Tokens       *OAuthTokens `json:"tokens,omitempty"`
	CodeVerifier string       `json:"codeVerifier,omitempty"`
	State        string       `json:"oauthState,omitempty"`
}

type OAuthFlow struct {
	mu       sync.Mutex
	pending  map[string]chan oauthCallback
	server   *http.Server
	listener net.Listener
	running  bool
}

type oauthCallback struct {
	code  string
	state string
	err   error
}

func NewOAuthFlow() *OAuthFlow {
	return &OAuthFlow{
		pending: make(map[string]chan oauthCallback),
	}
}

func (f *OAuthFlow) StartAuth(ctx context.Context, cfg *OAuthConfig) (authURL string, state string, err error) {
	if cfg == nil {
		return "", "", fmt.Errorf("OAuth config is nil")
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generating state: %w", err)
	}
	state = hex.EncodeToString(stateBytes)

	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", fmt.Errorf("generating code verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	if err := f.ensureCallbackServer(cfg); err != nil {
		return "", "", fmt.Errorf("starting callback server: %w", err)
	}

	callbackURL := cfg.CallbackURL
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://127.0.0.1:%d%s", defaultCallbackPort, oauthCallbackPath)
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {callbackURL},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if len(cfg.Scopes) > 0 {
		params.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	authURL = cfg.AuthURL + "?" + params.Encode()

	if err := saveOAuthState(cfg.ClientID, &OAuthState{
		CodeVerifier: verifier,
		State:        state,
	}); err != nil {
		slog.Warn("failed to save OAuth state", "error", err)
	}

	return authURL, state, nil
}

func (f *OAuthFlow) WaitForCallback(ctx context.Context, state string) (string, error) {
	ch := make(chan oauthCallback, 1)

	f.mu.Lock()
	f.pending[state] = ch
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		delete(f.pending, state)
		f.mu.Unlock()
	}()

	select {
	case result := <-ch:
		if result.err != nil {
			return "", result.err
		}
		return result.code, nil
	case <-time.After(callbackTimeout):
		return "", fmt.Errorf("OAuth callback timeout after %v", callbackTimeout)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (f *OAuthFlow) ExchangeCode(ctx context.Context, cfg *OAuthConfig, code string) (*OAuthTokens, error) {
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("token URL not configured")
	}

	stored, _ := loadOAuthState(cfg.ClientID)
	verifier := ""
	if stored != nil {
		verifier = stored.CodeVerifier
	}

	callbackURL := cfg.CallbackURL
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://127.0.0.1:%d%s", defaultCallbackPort, oauthCallbackPath)
	}

	params := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {callbackURL},
		"client_id":    {cfg.ClientID},
	}
	if verifier != "" {
		params.Set("code_verifier", verifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange: status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token,omitempty"`
		ExpiresIn    int64  `json:"expires_in,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
	}
	if tokenResp.ExpiresIn > 0 {
		tokens.ExpiresAt = time.Now().Unix() + tokenResp.ExpiresIn
	}

	if err := saveOAuthState(cfg.ClientID, &OAuthState{Tokens: tokens}); err != nil {
		slog.Warn("failed to save OAuth tokens", "error", err)
	}

	return tokens, nil
}

func (f *OAuthFlow) ensureCallbackServer(cfg *OAuthConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return nil
	}

	port := defaultCallbackPort
	if cfg.CallbackURL != "" {
		if u, err := url.Parse(cfg.CallbackURL); err == nil && u.Port() != "" {
			if n, scanErr := fmt.Sscanf(u.Port(), "%d", &port); scanErr != nil || n != 1 {
				return fmt.Errorf("malformed port in callback URL: %s", u.Port())
			}
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("binding callback port %d: %w", port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(oauthCallbackPath, f.handleCallback)

	srv := &http.Server{Handler: mux}
	f.server = srv
	f.listener = listener
	f.running = true

	safego.Go(func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("OAuth callback server error", "error", err)
		}
	})

	return nil
}

func (f *OAuthFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "<html><body><h2>Error</h2><p>Missing code or state parameter.</p></body></html>")
		return
	}

	f.mu.Lock()
	ch, ok := f.pending[state]
	f.mu.Unlock()

	if !ok {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "<html><body><h2>Error</h2><p>Unknown or expired OAuth state.</p></body></html>")
		return
	}

	ch <- oauthCallback{code: code, state: state}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<html><body><h2>Success</h2><p>Authorization complete. You can close this tab.</p></body></html>")
}

func (f *OAuthFlow) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = f.server.Shutdown(ctx)
		f.running = false
	}
}

func oauthStorePath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "tinycode", "mcp-auth.json")
}

type OAuthConfig = config.MCPOAuthConfig

func saveOAuthState(clientID string, state *OAuthState) error {
	path := oauthStorePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	store := make(map[string]*OAuthState)
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &store)
	}

	store[clientID] = state

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".mcp-auth-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, path)
}

func loadOAuthState(clientID string) (*OAuthState, error) {
	data, err := os.ReadFile(oauthStorePath())
	if err != nil {
		return nil, err
	}

	store := make(map[string]*OAuthState)
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}

	return store[clientID], nil
}
