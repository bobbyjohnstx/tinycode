package redhat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSSOURL     = "https://sso.redhat.com"
	DefaultAPIBaseURL = "https://console.redhat.com"
	tokenPath         = "/auth/realms/redhat-external/protocol/openid-connect/token"
	expiryBuffer      = 60 * time.Second
)

type ConsoleAuthConfig struct {
	OfflineToken string
	ClientID     string
	SSOURL       string
	APIBaseURL   string
}

const DefaultClientID = "cloud-services"

type ConsoleAuthClient struct {
	ssoURL       string
	clientID     string
	offlineToken string
	mu           sync.Mutex
	accessToken  string
	expiresAt    time.Time
	httpClient   *http.Client
}

func NewConsoleAuthClient(cfg ConsoleAuthConfig) *ConsoleAuthClient {
	ssoURL := cfg.SSOURL
	if ssoURL == "" {
		ssoURL = DefaultSSOURL
	}
	clientID := cfg.ClientID
	if clientID == "" {
		clientID = DefaultClientID
	}
	return &ConsoleAuthClient{
		ssoURL:       ssoURL,
		clientID:     clientID,
		offlineToken: cfg.OfflineToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *ConsoleAuthClient) IsConfigured() bool {
	return c.offlineToken != ""
}

func (c *ConsoleAuthClient) GetAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.expiresAt.Add(-expiryBuffer)) {
		return c.accessToken, nil
	}

	token, expiresIn, err := c.exchangeToken(ctx)
	if err != nil {
		return "", err
	}

	c.accessToken = token
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return c.accessToken, nil
}

func (c *ConsoleAuthClient) exchangeToken(ctx context.Context) (string, int, error) {
	reqURL := c.ssoURL + tokenPath
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.clientID},
		"refresh_token": {c.offlineToken},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("console SSO token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, fmt.Errorf("parsing token response: %w", err)
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

func NewConsoleAPIClient(cfg ConsoleAuthConfig, servicePath string, authClient *ConsoleAuthClient) *APIClient {
	apiBaseURL := cfg.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = DefaultAPIBaseURL
	}
	if authClient == nil {
		authClient = NewConsoleAuthClient(cfg)
	}
	return NewAPIClient(APIClientConfig{
		BaseURL: apiBaseURL + servicePath,
		TokenFn: authClient.GetAccessToken,
	})
}
