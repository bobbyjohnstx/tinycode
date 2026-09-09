package tool

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	maxFetchSize    = 512 * 1024 // 512KB
	fetchTimeout    = 30 * time.Second
)

var fetchClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:  10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	},
}

type webfetchArgs struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

func WebFetchTool() *Def {
	return &Def{
		ID:          "webfetch",
		Description: "Fetch content from a URL. Returns the response body.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method (default: GET)",
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "Additional headers to send",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Request body for POST/PUT",
				},
			},
			"required": []string{"url"},
		},
		Execute: executeWebFetch,
	}
}

func executeWebFetch(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args webfetchArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	method := "GET"
	if args.Method != "" {
		method = strings.ToUpper(args.Method)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	var bodyReader io.Reader
	if args.Body != "" {
		bodyReader = strings.NewReader(args.Body)
	}

	req, err := http.NewRequestWithContext(fetchCtx, method, args.URL, bodyReader)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error creating request: %v", err), IsError: true}, nil
	}

	for k, v := range args.Headers {
		req.Header.Set(k, v)
	}

	resp, err := fetchClient.Do(req)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Fetch error: %v", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error reading response: %v", err), IsError: true}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("HTTP %d %s\n\n", resp.StatusCode, resp.Status))
	sb.Write(body)

	if resp.StatusCode >= 400 {
		return &ExecuteResult{Output: sb.String(), IsError: true}, nil
	}

	return &ExecuteResult{Output: sb.String()}, nil
}
