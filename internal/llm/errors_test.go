package llm

import (
	"strings"
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
