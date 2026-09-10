package tool

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	maxFetchSize = 5 * 1024 * 1024 // 5MB
	fetchTimeout = 30 * time.Second
)

var errSSRFBlocked = errors.New("blocked: URL resolves to private/internal network address")

// ssrfBlockedPrefixes lists the CIDR ranges that must never be reached by webfetch.
var ssrfBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),    // loopback
	netip.MustParsePrefix("10.0.0.0/8"),     // private
	netip.MustParsePrefix("172.16.0.0/12"),  // private
	netip.MustParsePrefix("192.168.0.0/16"), // private
	netip.MustParsePrefix("169.254.0.0/16"), // link-local / cloud metadata
	netip.MustParsePrefix("0.0.0.0/8"),      // unspecified
	netip.MustParsePrefix("::1/128"),        // IPv6 loopback
	netip.MustParsePrefix("fc00::/7"),       // IPv6 private
	netip.MustParsePrefix("fe80::/10"),      // IPv6 link-local
}

// checkSSRF resolves the hostname of u and returns errSSRFBlocked if any
// resolved address falls within a private/loopback/link-local range.
func checkSSRF(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("invalid URL: missing hostname")
	}

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}

	for _, addr := range addrs {
		ip, err := netip.ParseAddr(addr)
		if err != nil {
			continue
		}
		for _, prefix := range ssrfBlockedPrefixes {
			if prefix.Contains(ip) {
				return errSSRFBlocked
			}
		}
	}
	return nil
}

// ssrfSafeClient returns an http.Client that validates each redirect target
// against the SSRF blocklist before following it.
func ssrfSafeClient(ctx context.Context) *http.Client {
	return &http.Client{
		Transport: fetchClient.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return checkSSRF(ctx, req.URL.String())
		},
	}
}

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
	Format  string            `json:"format,omitempty"` // text, markdown, html
}

func WebFetchTool() *Def {
	return &Def{
		ID:          "webfetch",
		Description: "Fetch content from a URL. Returns the response body.",
		Permission:  "webfetch",
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
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: text (default), markdown, or html",
					"enum":        []string{"text", "markdown", "html"},
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

	// SSRF check: block requests to private/internal network addresses.
	if err := checkSSRF(fetchCtx, args.URL); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Fetch error: %v", err), IsError: true}, nil
	}

	var bodyReader io.Reader
	if args.Body != "" {
		bodyReader = strings.NewReader(args.Body)
	}

	req, err := http.NewRequestWithContext(fetchCtx, method, args.URL, bodyReader)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error creating request: %v", err), IsError: true}, nil
	}

	req.Header.Set("User-Agent", "tinycode/1.0 (compatible; fetch tool)")

	switch args.Format {
	case "html":
		req.Header.Set("Accept", "text/html")
	case "markdown":
		req.Header.Set("Accept", "text/plain")
	default:
		req.Header.Set("Accept", "text/plain, text/html")
	}

	for k, v := range args.Headers {
		req.Header.Set(k, v)
	}

	// Use a client with redirect validation to prevent SSRF via redirects.
	client := ssrfSafeClient(fetchCtx)
	resp, err := client.Do(req)
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
