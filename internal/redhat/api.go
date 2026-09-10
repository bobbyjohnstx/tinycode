package redhat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type APIClientConfig struct {
	BaseURL    string
	TokenFn    func(ctx context.Context) (string, error)
	Headers    map[string]string
	MaxRetries int
	Timeout    time.Duration
}

type APIResponse struct {
	Data    json.RawMessage
	Status  int
	Headers http.Header
}

type APIClient struct {
	baseURL    string
	tokenFn    func(ctx context.Context) (string, error)
	headers    map[string]string
	maxRetries int
	httpClient *http.Client
}

func NewAPIClient(cfg APIClientConfig) *APIClient {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &APIClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		tokenFn:    cfg.TokenFn,
		headers:    cfg.Headers,
		maxRetries: maxRetries,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *APIClient) Get(ctx context.Context, path string, query map[string]string) (*APIResponse, error) {
	return c.request(ctx, http.MethodGet, path, query, nil)
}

func (c *APIClient) Post(ctx context.Context, path string, body any) (*APIResponse, error) {
	return c.request(ctx, http.MethodPost, path, nil, body)
}

func (c *APIClient) Put(ctx context.Context, path string, body any) (*APIResponse, error) {
	return c.request(ctx, http.MethodPut, path, nil, body)
}

func (c *APIClient) Delete(ctx context.Context, path string) (*APIResponse, error) {
	return c.request(ctx, http.MethodDelete, path, nil, nil)
}

func (c *APIClient) request(ctx context.Context, method, path string, query map[string]string, body any) (*APIResponse, error) {
	reqURL := c.baseURL + path
	if len(query) > 0 {
		params := url.Values{}
		for k, v := range query {
			params.Set(k, v)
		}
		reqURL += "?" + params.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	var resp *http.Response
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		for k, v := range c.headers {
			req.Header.Set(k, v)
		}

		if c.tokenFn != nil {
			token, err := c.tokenFn(ctx)
			if err != nil {
				return nil, fmt.Errorf("getting auth token: %w", err)
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			if attempt == c.maxRetries {
				return nil, fmt.Errorf("request failed after %d attempt(s): %w", attempt+1, err)
			}
			continue
		}

		if resp.StatusCode != http.StatusUnauthorized {
			break
		}
		resp.Body.Close()
	}

	if resp == nil {
		return nil, fmt.Errorf("no response after %d attempts", c.maxRetries+1)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return &APIResponse{
		Data:    json.RawMessage(respBody),
		Status:  resp.StatusCode,
		Headers: resp.Header,
	}, nil
}

func DecodeResponse[T any](resp *APIResponse) (T, error) {
	var result T
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return result, fmt.Errorf("decoding response: %w", err)
	}
	return result, nil
}
