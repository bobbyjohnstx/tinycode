package llm

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestHTTPStatusError_AuthMessages(t *testing.T) {
	t.Run("401", func(t *testing.T) {
		err := httpStatusError(401, `{"error":"invalid api key"}`)
		msg := err.Error()
		if !strings.HasPrefix(msg, "HTTP 401") {
			t.Errorf("expected HTTP 401 prefix, got %q", msg)
		}
		if !strings.Contains(msg, "API key") {
			t.Errorf("expected actionable API key hint, got %q", msg)
		}
	})
	t.Run("403", func(t *testing.T) {
		err := httpStatusError(403, `{"error":"forbidden"}`)
		msg := err.Error()
		if !strings.HasPrefix(msg, "HTTP 403") {
			t.Errorf("expected HTTP 403 prefix, got %q", msg)
		}
		if !strings.Contains(msg, "API key") {
			t.Errorf("expected actionable API key hint, got %q", msg)
		}
	})
}

func TestHTTPStatusError_OtherStatus(t *testing.T) {
	err := httpStatusError(500, "internal error")
	if err.Error() != "HTTP 500: internal error" {
		t.Errorf("unexpected message: %q", err.Error())
	}
}

func TestDialError_ConnectionRefused(t *testing.T) {
	err := dialError("http://127.0.0.1:11434", &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: syscall.ECONNREFUSED,
	})
	msg := err.Error()
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("expected refused classification, got %q", msg)
	}
	if !strings.Contains(msg, "127.0.0.1:11434") {
		t.Errorf("expected host in message, got %q", msg)
	}
	if !strings.Contains(msg, "tinycode doctor") {
		t.Errorf("expected doctor hint, got %q", msg)
	}
	if !strings.Contains(msg, "TINYCODE_OLLAMA_HOST") && !strings.Contains(msg, "endpoint") {
		t.Errorf("expected override/endpoint hint, got %q", msg)
	}
}

func TestDialError_Timeout(t *testing.T) {
	err := dialError("http://example.com:443", context.DeadlineExceeded)
	msg := err.Error()
	if !strings.Contains(msg, "timeout") {
		t.Errorf("expected timeout classification, got %q", msg)
	}
	if !strings.Contains(msg, "example.com:443") {
		t.Errorf("expected host, got %q", msg)
	}
	if !strings.Contains(msg, "tinycode doctor") {
		t.Errorf("expected doctor hint, got %q", msg)
	}
}

func TestDialError_DNS(t *testing.T) {
	err := dialError("http://no-such-host.invalid", &net.DNSError{
		Err:  "no such host",
		Name: "no-such-host.invalid",
	})
	msg := err.Error()
	if !strings.Contains(msg, "DNS") {
		t.Errorf("expected DNS classification, got %q", msg)
	}
	if !errors.As(err, new(*net.DNSError)) && !strings.Contains(msg, "no-such-host") {
		t.Errorf("expected hostname retained, got %q", msg)
	}
}
