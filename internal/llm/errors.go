package llm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// httpStatusError formats a non-200 LLM HTTP response. Auth errors keep the
// literal "HTTP 401" / "HTTP 403" prefix so session.statusFromError can parse them.
func httpStatusError(statusCode int, body string) error {
	switch statusCode {
	case 401:
		return fmt.Errorf("HTTP 401: authentication failed — check your API key or provider credentials (%s)", body)
	case 403:
		return fmt.Errorf("HTTP 403: access denied — verify your API key permissions or model access (%s)", body)
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, body)
	}
}

// dialError wraps a transport failure with host + override + doctor guidance.
func dialError(baseURL string, err error) error {
	host := endpointHost(baseURL)
	kind, hint := classifyDial(err)
	if host == "" {
		return fmt.Errorf("%s: %w — %s; run: tinycode doctor", kind, err, hint)
	}
	return fmt.Errorf("%s connecting to %s: %w — %s; run: tinycode doctor", kind, host, err, hint)
}

func endpointHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return strings.TrimRight(baseURL, "/")
	}
	return u.Host
}

func classifyDial(err error) (kind, hint string) {
	if err == nil {
		return "request failed", "check the provider endpoint"
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		return "timeout", "check the host is reachable or raise the client timeout"
	}
	if isDNS(err) {
		return "DNS lookup failed", "check the hostname or set TINYCODE_OLLAMA_HOST / provider baseURL"
	}
	if isRefused(err) {
		return "connection refused", "start the provider (e.g. ollama serve) or fix the endpoint URL / env override"
	}
	return "request failed", "check the provider endpoint URL and network"
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func isDNS(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

func isRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return true
		}
		if opErr.Err != nil && strings.Contains(strings.ToLower(opErr.Err.Error()), "connection refused") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection refused")
}
