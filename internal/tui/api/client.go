package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

const defaultHTTPTimeout = 60 * time.Second

// Client wraps net/http for all tinycode server API endpoints.
type Client struct {
	baseURL    string
	directory  string
	token      string
	http       *http.Client
	sseHTTP    *http.Client
	sseBackoff time.Duration
}

// New creates an API client targeting the given server base URL and working directory.
// An optional token enables Bearer authentication on all requests.
// The REST client uses a 60s timeout; SSE uses a separate client with no timeout.
func New(baseURL, directory, token string) *Client {
	rest := &http.Client{Timeout: defaultHTTPTimeout}
	return &Client{
		baseURL:   baseURL,
		directory: directory,
		token:     token,
		http:      rest,
		sseHTTP:   &http.Client{Transport: rest.Transport},
	}
}

// CreateSession creates a new session via POST /session.
func (c *Client) CreateSession(input SessionCreateInput) (*session.Info, error) {
	var info session.Info
	q := url.Values{}
	q.Set("directory", c.directory)
	if err := c.postJSON("/session?"+q.Encode(), input, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListSessions lists sessions via GET /session.
func (c *Client) ListSessions(limit, offset int) ([]session.Info, error) {
	q := url.Values{}
	q.Set("directory", c.directory)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))

	var sessions []session.Info
	if err := c.getJSON("/session?"+q.Encode(), &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetSession fetches a single session via GET /session/{id}.
func (c *Client) GetSession(id string) (*session.Info, error) {
	var info session.Info
	if err := c.getJSON("/session/"+id, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// UpdateSessionTitle renames a session via PATCH /session/{id}.
func (c *Client) UpdateSessionTitle(id, title string) error {
	data, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return err
	}
	body, err := c.doRequest(http.MethodPatch, "/session/"+id, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if body != nil {
		body.Close()
	}
	return nil
}

// DeleteSession deletes a session via DELETE /session/{id}.
func (c *Client) DeleteSession(id string) error {
	return c.doNoBody(http.MethodDelete, "/session/"+id)
}

// SendPrompt sends a prompt asynchronously via POST /session/{id}/prompt_async.
// Returns nil on 204 No Content.
func (c *Client) SendPrompt(sessionID string, input PromptInput) error {
	return c.postNoResp("/session/"+sessionID+"/prompt_async", input)
}

// AbortSession aborts a running session via POST /session/{id}/abort.
func (c *Client) AbortSession(id string) error {
	return c.doNoBody(http.MethodPost, "/session/"+id+"/abort")
}

// ListProviders fetches all providers via GET /provider.
func (c *Client) ListProviders() (*ProviderListResponse, error) {
	var resp ProviderListResponse
	if err := c.getJSON("/provider", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetProviderAuth stores credentials for a provider via PUT /auth/{providerID}.
func (c *Client) SetProviderAuth(providerID string, creds map[string]string) error {
	return c.putNoResp("/auth/"+providerID, creds)
}

// ListAgents fetches all agents via GET /agent.
func (c *Client) ListAgents() ([]AgentInfo, error) {
	var agents []AgentInfo
	if err := c.getJSON("/agent", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// ListAllAgents fetches all agents including disabled ones via GET /agent?include=disabled.
func (c *Client) ListAllAgents() ([]AgentInfo, error) {
	var agents []AgentInfo
	if err := c.getJSON("/agent?include=disabled", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// PatchConfig sends a partial config update via PATCH /config.
func (c *Client) PatchConfig(body map[string]any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	respBody, err := c.doRequest(http.MethodPatch, "/config", bytes.NewReader(data))
	if err != nil {
		return err
	}
	if respBody != nil {
		respBody.Close()
	}
	return nil
}

// ListCommands fetches all commands via GET /command.
func (c *Client) ListCommands() ([]CommandInfo, error) {
	var commands []CommandInfo
	if err := c.getJSON("/command", &commands); err != nil {
		return nil, err
	}
	return commands, nil
}

// ListMessages fetches messages for a session via GET /session/{id}/message.
func (c *Client) ListMessages(sessionID string) ([]map[string]any, error) {
	var messages []map[string]any
	if err := c.getJSON("/session/"+sessionID+"/message", &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// GetMCPStatus fetches MCP server status via GET /mcp/status.
func (c *Client) GetMCPStatus() (map[string]map[string]any, error) {
	var status map[string]map[string]any
	if err := c.getJSON("/mcp/status", &status); err != nil {
		return nil, err
	}
	return status, nil
}

// ReconnectMCP requests reconnection of a named MCP server via POST /mcp/{name}/reconnect.
func (c *Client) ReconnectMCP(name string) error {
	return c.postNoResp("/mcp/"+name+"/reconnect", nil)
}

// ListPlugins retrieves loaded plugins via GET /plugin.
func (c *Client) ListPlugins() ([]PluginInfo, error) {
	var plugins []PluginInfo
	if err := c.getJSON("/plugin", &plugins); err != nil {
		return nil, err
	}
	return plugins, nil
}

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BalanceResponse is the response from GET /provider/{id}/balance.
type BalanceResponse struct {
	Remaining *float64 `json:"remaining"`
	Usage     *float64 `json:"usage"`
	Provider  string   `json:"provider"`
}

// GetProviderBalance fetches the balance for a provider via GET /provider/{id}/balance.
func (c *Client) GetProviderBalance(providerID string) (*BalanceResponse, error) {
	var resp BalanceResponse
	if err := c.getJSON("/provider/"+providerID+"/balance", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SummarizeSession triggers manual context compaction via POST /session/{id}/summarize.
// The Go server currently returns 501; this surfaces a clear error rather than claiming success.
func (c *Client) SummarizeSession(id string) error {
	err := c.doNoBody(http.MethodPost, "/session/"+id+"/summarize")
	if err != nil && (strings.Contains(err.Error(), "status 501") || strings.Contains(err.Error(), "Not Implemented")) {
		return fmt.Errorf("summarize not implemented")
	}
	return err
}

// ArchiveSession archives a session via POST /session/{id}/archive.
func (c *Client) ArchiveSession(id string) error {
	return c.doNoBody(http.MethodPost, "/session/"+id+"/archive")
}

// RevertSession triggers a revert (undo) for the session via POST /session/{id}/revert.
func (c *Client) RevertSession(id string) error {
	return c.doNoBody(http.MethodPost, "/session/"+id+"/revert")
}

// UnrevertSession restores previously reverted changes via POST /session/{id}/unrevert.
func (c *Client) UnrevertSession(id string) error {
	return c.doNoBody(http.MethodPost, "/session/"+id+"/unrevert")
}

// ReplyPermission replies to a permission prompt via POST /session/{sessionID}/permissions/{permissionID}.
// reply should be once|always|reject (SDK contract). Action is also sent for older servers.
func (c *Client) ReplyPermission(sessionID, permissionID, reply string) error {
	body := PermissionReplyInput{Reply: reply, Action: reply}
	return c.postNoResp("/session/"+sessionID+"/permissions/"+permissionID, body)
}

// BranchSession branches a session via POST /session/{id}/fork.
// It creates a new session with ParentID set to the source session and copies all messages.
func (c *Client) BranchSession(sessionID, title string) (*session.Info, error) {
	var info session.Info
	body := map[string]string{"title": title}
	if err := c.postJSON("/session/"+sessionID+"/fork", body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// RewindSession rewinds a session to a specific message via POST /session/{id}/rewind.
// All messages after the target message are deleted.
func (c *Client) RewindSession(sessionID, messageID string) error {
	body := map[string]string{"messageID": messageID}
	return c.postNoResp("/session/"+sessionID+"/rewind", body)
}

// Btw sends a side question via POST /session/{id}/btw and returns the answer.
func (c *Client) Btw(sessionID, question string) (string, error) {
	var resp struct {
		Answer string `json:"answer"`
	}
	if err := c.postJSON("/session/"+sessionID+"/btw", map[string]string{"question": question}, &resp); err != nil {
		return "", err
	}
	return resp.Answer, nil
}

// getJSON performs a GET and decodes the JSON response into dest.
func (c *Client) getJSON(path string, dest any) error {
	body, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(dest)
}

// postJSON performs a POST with a JSON body and decodes the response into dest.
func (c *Client) postJSON(path string, payload, dest any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	body, err := c.doRequest(http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(dest)
}

// postNoResp performs a POST with a JSON body, ignoring the response body.
func (c *Client) postNoResp(path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	body, err := c.doRequest(http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if body != nil {
		body.Close()
	}
	return nil
}

// putNoResp performs a PUT with a JSON body, ignoring the response body.
func (c *Client) putNoResp(path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	body, err := c.doRequest(http.MethodPut, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if body != nil {
		body.Close()
	}
	return nil
}

// doNoBody performs a request without a request body, ignoring the response body.
func (c *Client) doNoBody(method, path string) error {
	body, err := c.doRequest(method, path, nil)
	if err != nil {
		return err
	}
	if body != nil {
		body.Close()
	}
	return nil
}

// doRequest executes an HTTP request and returns the response body.
// It returns an error for non-2xx status codes.
func (c *Client) doRequest(method, path string, reqBody io.Reader) (io.ReadCloser, error) {
	return c.doRequestContext(context.Background(), method, path, reqBody)
}

// doRequestContext executes an HTTP request with the given context and returns the response body.
func (c *Client) doRequestContext(ctx context.Context, method, path string, reqBody io.Reader) (io.ReadCloser, error) {
	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&apiErr) == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("%s %s: %s", method, path, apiErr.Error)
		}
		return nil, fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}

	return resp.Body, nil
}
