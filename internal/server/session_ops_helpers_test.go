package server

import "testing"

func TestAutoTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal text",
			input: "Fix the authentication bug",
			want:  "Fix the authentication bug",
		},
		{
			name:  "long text truncates at word boundary",
			input: "This is a very long prompt that exceeds the sixty character limit and should be truncated at a word boundary",
			want:  "This is a very long prompt that exceeds the sixty character...",
		},
		{
			name:  "slash command stripped",
			input: "/ask explore review this project",
			want:  "explore review this project",
		},
		{
			name:  "slash swarm stripped",
			input: "/swarm build the feature",
			want:  "build the feature",
		},
		{
			name:  "multi-line collapsed",
			input: "line1\nline2\nline3",
			want:  "line1 line2 line3",
		},
		{
			name:  "short text preserved",
			input: "hi",
			want:  "hi",
		},
		{
			name:  "whitespace only returns empty",
			input: "   ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := autoTitle(tt.input)
			if got != tt.want {
				t.Errorf("autoTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsDefaultTitle(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		{"New Session", true},
		{"New session - 2026-09-22", true},
		{"", true},
		{"Child session - abc123", true},
		{"My real title", false},
		{"Fix the bug", false},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := isDefaultTitle(tt.title)
			if got != tt.want {
				t.Errorf("isDefaultTitle(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}
