package redhat

import (
	"testing"
	"time"
)

func TestParseDuration_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h", 1 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseDuration(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, d, tt.expected)
			}
		})
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	invalid := []string{"", "abc", "5x", "m5", "5 m", "-1h", "1.5h"}
	for _, input := range invalid {
		t.Run(input, func(t *testing.T) {
			_, err := ParseDuration(input)
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}
