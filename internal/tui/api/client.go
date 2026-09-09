package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

// Client wraps net/http for all tinycode server API endpoints.
type Client struct {
	baseURL    string
	directory  string
	http       *http.Client
	sseBackoff time.Duration
}

// New creates an API client targeting the given server base URL and working directory.
func New(baseURL, directory string) *Client {
	return &Client{
		baseURL:   baseURL,
		directory: directory,
		http:      &http.Client{},
	}
}

// CreateSession creates a new session via POST /session.
func (c *Client) CreateSession(input SessionCreateInput) (*session.Info, error) {
	var info session.Info
	if err := c.postJSON("/session?directory="+c.directory, input, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListSessions lists sessions via GET /session.
func (c *Client) ListSessions(limit, offset int) ([]session.Info, error) {
	path := "/session?directory=" + c.directory +
		"&limit=" + strconv.Itoa(limit) +
		"&offset=" + strconv.Itoa(offset)

	var sessions []session.Info
	if err := c.getJSON(path, &sessions); err != nil {
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
	_, err := c.doRequest(http.MethodPost, "/session/"+id+"/abort", nil)
	return err
}

// ListProviders fetches all providers via GET /provider.
func (c *Client) ListProviders() (*ProviderListResponse, error) {
	var resp ProviderListResponse
	if err := c.getJSON("/provider", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListAgents fetches all agents via GET /agent.
func (c *Client) ListAgents() ([]AgentInfo, error) {
	var agents []AgentInfo
	if err := c.getJSON("/agent", &agents); err != nil {
		return nil, err
	}
	return agents, nil
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

// ReplyPermission replies to a permission prompt via POST /session/{sessionID}/permissions/{permissionID}.
func (c *Client) ReplyPermission(sessionID, permissionID, action string) error {
	body := PermissionReplyInput{Action: action}
	return c.postNoResp("/session/"+sessionID+"/permissions/"+permissionID, body)
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
	url := c.baseURL + path

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
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
