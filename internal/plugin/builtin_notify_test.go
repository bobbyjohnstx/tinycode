package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNotifyTool_UrgencyParameter(t *testing.T) {
	p := NewNotifyPlugin()
	tools := p.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	props, ok := tools[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	urgencyProp, ok := props["urgency"].(map[string]any)
	if !ok {
		t.Fatal("expected urgency property")
	}
	if urgencyProp["type"] != "string" {
		t.Errorf("urgency type = %v, want string", urgencyProp["type"])
	}
	enumVal, ok := urgencyProp["enum"].([]string)
	if !ok {
		t.Fatal("expected enum []string")
	}
	if len(enumVal) != 3 {
		t.Errorf("urgency enum count = %d, want 3", len(enumVal))
	}
}

func TestNotifyTool_RequiresFields(t *testing.T) {
	p := NewNotifyPlugin()
	tool := p.Tools()[0]

	// Empty title should fail.
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"","message":"hi"}`))
	if err == nil {
		t.Error("expected error for empty title")
	}

	// Empty message should fail.
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"title":"hi","message":""}`))
	if err == nil {
		t.Error("expected error for empty message")
	}

	// Invalid JSON should fail.
	_, err = tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRateLimiting(t *testing.T) {
	// Reset rate limiter state.
	notifyMu.Lock()
	lastNotifyTime = time.Time{}
	notifyMu.Unlock()

	// First call should go through (may fail on the command itself, but won't be rate-limited).
	result1, _ := SendNotification(context.Background(), "Test", "First", "normal")
	if result1 == "Notification rate-limited (max 1 per 5 seconds)" {
		t.Error("first notification should not be rate-limited")
	}

	// Second call within 5s should be rate-limited.
	result2, err := SendNotification(context.Background(), "Test", "Second", "normal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2 != "Notification rate-limited (max 1 per 5 seconds)" {
		t.Errorf("second notification should be rate-limited, got: %q", result2)
	}
}

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello",
			expected: `"hello"`,
		},
		{
			name:     "double quotes",
			input:    `say "hi"`,
			expected: `"say \"hi\""`,
		},
		{
			name:     "backslash",
			input:    `path\to\file`,
			expected: `"path\\to\\file"`,
		},
		{
			name:     "single quotes",
			input:    "it's done",
			expected: `"it'\''s done"`,
		},
		{
			name:     "newlines stripped",
			input:    "line1\nline2\rline3",
			expected: `"line1 line2line3"`,
		},
		{
			name:     "combined special chars",
			input:    "say \"it's\"\nnew\\line",
			expected: `"say \"it'\''s\" new\\line"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeAppleScript(tt.input)
			if got != tt.expected {
				t.Errorf("escapeAppleScript(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsWSL(t *testing.T) {
	// On non-Linux or standard Linux, isWSL should return false.
	// We cannot mock /proc/version easily, but we can verify the function
	// does not panic and returns a bool.
	result := isWSL()
	// On macOS or non-WSL Linux, this should be false.
	if result {
		t.Log("isWSL returned true — running in WSL environment")
	}
}

func TestNotifyPlugin_ID(t *testing.T) {
	p := NewNotifyPlugin()
	if p.ID() != "notify" {
		t.Errorf("ID() = %q, want %q", p.ID(), "notify")
	}
}

func TestNotifyPlugin_HooksEmpty(t *testing.T) {
	p := NewNotifyPlugin()
	hooks := p.Hooks()
	if hooks.SessionStart != nil || hooks.SessionEnd != nil || hooks.Dispose != nil {
		t.Error("expected all hooks to be nil")
	}
}
