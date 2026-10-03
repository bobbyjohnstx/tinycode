package provider

import (
	"testing"
)

// --- Issue #89: IsOverflow anchored HTTP status matching ---

func TestIsOverflow_HTTPFormat(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"HTTP 400: invalid request body", false},
		{"HTTP 400: context_length_exceeded", true},
		{"HTTP 413: request entity too large", true},
	}
	for _, tt := range tests {
		got := IsOverflow(tt.msg)
		if got != tt.want {
			t.Errorf("IsOverflow(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsOverflow_ContextLengthExceeded(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"context_length_exceeded", true},
		{"context length exceeded", true},
		{"This model's maximum context length is 8192 tokens", true},
		{"prompt is too long", true},
	}
	for _, tt := range tests {
		got := IsOverflow(tt.msg)
		if got != tt.want {
			t.Errorf("IsOverflow(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsOverflow_NegativeNonMatches(t *testing.T) {
	tests := []struct {
		msg string
	}{
		{"HTTP 500: internal server error"},
		{"HTTP 429: too many requests"},
		{"connection refused"},
		{"some random error"},
	}
	for _, tt := range tests {
		got := IsOverflow(tt.msg)
		if got {
			t.Errorf("IsOverflow(%q) = true, want false", tt.msg)
		}
	}
}

