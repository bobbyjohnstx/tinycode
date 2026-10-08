package tui

import (
	"strings"
	"testing"
)

func TestSanitizeFilename_BasicAlphanumeric(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase preserved", "hello-world", "hello-world"},
		{"spaces become dashes", "hello world", "hello-world"},
		{"underscores become dashes", "hello_world", "hello-world"},
		{"uppercase lowered", "Hello World", "hello-world"},
		{"special chars removed", "hello@world#123!", "helloworld123"},
		{"consecutive dashes collapsed", "hello---world", "hello-world"},
		{"leading dash trimmed", "-hello", "hello"},
		{"trailing dash trimmed", "hello-", "hello"},
		{"mixed special chars", "Fix bug: parse JSON (v2)", "fix-bug-parse-json-v2"},
		{"empty input", "", ""},
		{"only special chars", "@#$%^&*()", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename_TruncatesLongInput(t *testing.T) {
	input := strings.Repeat("a", 100)
	got := sanitizeFilename(input)
	if len(got) > 40 {
		t.Errorf("expected max 40 chars, got %d", len(got))
	}
}

func TestFormatPartExport_TextPart(t *testing.T) {
	got := formatPartExport(PartView{Type: "text", Text: "Hello world"})
	if got != "Hello world\n\n" {
		t.Errorf("expected 'Hello world\\n\\n', got %q", got)
	}
}

func TestFormatPartExport_EmptyText(t *testing.T) {
	got := formatPartExport(PartView{Type: "text", Text: ""})
	if got != "" {
		t.Errorf("expected empty for empty text part, got %q", got)
	}
}

func TestFormatPartExport_ReasoningPart(t *testing.T) {
	got := formatPartExport(PartView{Type: "reasoning", Text: "Let me think..."})
	if !strings.Contains(got, "_Thinking:_") {
		t.Error("expected reasoning header")
	}
	if !strings.Contains(got, "Let me think...") {
		t.Error("expected reasoning text")
	}
}

func TestFormatPartExport_ReasoningEmpty(t *testing.T) {
	got := formatPartExport(PartView{Type: "reasoning", Text: ""})
	if got != "" {
		t.Errorf("expected empty for empty reasoning, got %q", got)
	}
}

func TestFormatPartExport_ToolCallWithArgs(t *testing.T) {
	got := formatPartExport(PartView{
		Type:     "tool-call",
		ToolName: "bash",
		ToolArgs: `{"command":"ls -la"}`,
	})
	if !strings.Contains(got, "**Tool: bash**") {
		t.Error("expected tool name header")
	}
	if !strings.Contains(got, "**Input:**") {
		t.Error("expected input label")
	}
	if !strings.Contains(got, `{"command":"ls -la"}`) {
		t.Error("expected tool args in code block")
	}
}

func TestFormatPartExport_ToolCallNoArgs(t *testing.T) {
	got := formatPartExport(PartView{
		Type:     "tool-call",
		ToolName: "read",
	})
	if !strings.Contains(got, "**Tool: read**") {
		t.Error("expected tool name header")
	}
	if strings.Contains(got, "**Input:**") {
		t.Error("expected no input section when args empty")
	}
}

func TestFormatPartExport_ToolCallNoName(t *testing.T) {
	got := formatPartExport(PartView{Type: "tool-call"})
	if !strings.Contains(got, "**Tool: unknown**") {
		t.Error("expected fallback tool name 'unknown'")
	}
}

func TestFormatPartExport_ToolResult(t *testing.T) {
	got := formatPartExport(PartView{Type: "tool-result", Text: "file.go\nmain.go"})
	if !strings.Contains(got, "**Output:**") {
		t.Error("expected output header")
	}
	if !strings.Contains(got, "file.go\nmain.go") {
		t.Error("expected result text in code block")
	}
}

func TestFormatPartExport_ToolResultEmpty(t *testing.T) {
	got := formatPartExport(PartView{Type: "tool-result", Text: ""})
	if got != "" {
		t.Errorf("expected empty for empty tool result, got %q", got)
	}
}

func TestFormatPartExport_ToolResultTruncated(t *testing.T) {
	// Build a result with more than 50 lines.
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, "line content")
	}
	text := strings.Join(lines, "\n")
	got := formatPartExport(PartView{Type: "tool-result", Text: text})
	if !strings.Contains(got, "... (10 more lines)") {
		t.Error("expected truncation notice for long tool result")
	}
}

func TestFormatPartExport_UnknownType(t *testing.T) {
	got := formatPartExport(PartView{Type: "image", Text: "data"})
	if got != "" {
		t.Errorf("expected empty for unknown type, got %q", got)
	}
}

func TestFormatMessageExport_UserRole(t *testing.T) {
	got := formatMessageExport(MessageView{
		Info:  MessageInfo{Role: "user"},
		Parts: []PartView{{Type: "text", Text: "Hello"}},
	})
	if !strings.HasPrefix(got, "## User\n\n") {
		t.Errorf("expected user header, got %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Error("expected message text")
	}
}

func TestFormatMessageExport_AssistantDefaultAgent(t *testing.T) {
	got := formatMessageExport(MessageView{
		Info:  MessageInfo{Role: "assistant"},
		Parts: []PartView{{Type: "text", Text: "Hi"}},
	})
	if !strings.Contains(got, "## Assistant (Build)") {
		t.Errorf("expected default agent 'Build' in header, got %q", got)
	}
}

func TestFormatMessageExport_AssistantCustomAgent(t *testing.T) {
	got := formatMessageExport(MessageView{
		Info:  MessageInfo{Role: "assistant", Agent: "architect", ModelID: "gpt-4"},
		Parts: []PartView{{Type: "text", Text: "Review"}},
	})
	if !strings.Contains(got, "Architect") {
		t.Error("expected capitalized agent name")
	}
	if !strings.Contains(got, "gpt-4") {
		t.Error("expected model ID in header")
	}
}

func TestFormatMessageExport_OtherRole(t *testing.T) {
	got := formatMessageExport(MessageView{
		Info:  MessageInfo{Role: "system"},
		Parts: []PartView{{Type: "text", Text: "system prompt"}},
	})
	if !strings.Contains(got, "## system") {
		t.Errorf("expected role-based header, got %q", got)
	}
}

func TestFormatTranscript_HeaderAndMessages(t *testing.T) {
	session := SessionInfo{
		ID:        "ses_abc",
		Title:     "Test Session",
		CreatedAt: 1700000000000,
		UpdatedAt: 1700001000000,
	}
	messages := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: "Hello"}},
		},
		{
			Info:  MessageInfo{Role: "assistant"},
			Parts: []PartView{{Type: "text", Text: "Hi there"}},
		},
	}

	got := formatTranscript(session, messages)

	if !strings.Contains(got, "# Test Session") {
		t.Error("expected title heading")
	}
	if !strings.Contains(got, "**Session ID:** ses_abc") {
		t.Error("expected session ID")
	}
	if !strings.Contains(got, "**Created:**") {
		t.Error("expected created timestamp")
	}
	if !strings.Contains(got, "**Updated:**") {
		t.Error("expected updated timestamp")
	}
	if !strings.Contains(got, "## User") {
		t.Error("expected user message header")
	}
	if !strings.Contains(got, "Hi there") {
		t.Error("expected assistant response text")
	}
	// Each message should be followed by a separator.
	if strings.Count(got, "---") < 3 { // header separator + 2 message separators
		t.Errorf("expected at least 3 separators, got %d", strings.Count(got, "---"))
	}
}

func TestFormatTranscript_UntitledSession(t *testing.T) {
	session := SessionInfo{ID: "ses_123"}
	messages := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: "msg"}},
		},
	}
	got := formatTranscript(session, messages)
	if !strings.Contains(got, "# Untitled Session") {
		t.Error("expected 'Untitled Session' for empty title")
	}
}

func TestFormatTranscript_NoTimestamps(t *testing.T) {
	session := SessionInfo{ID: "ses_123", Title: "No Timestamps"}
	messages := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: "msg"}},
		},
	}
	got := formatTranscript(session, messages)
	if strings.Contains(got, "**Created:**") {
		t.Error("expected no Created line when CreatedAt is 0")
	}
	if strings.Contains(got, "**Updated:**") {
		t.Error("expected no Updated line when UpdatedAt is 0")
	}
}

func TestFormatAssistantHeader_NoModelID(t *testing.T) {
	got := formatAssistantHeader(MessageView{
		Info: MessageInfo{Role: "assistant", Agent: "plan"},
	})
	if !strings.Contains(got, "Plan") {
		t.Error("expected capitalized agent name")
	}
	// Should not contain " · " separator when no model.
	if strings.Contains(got, " · ") {
		t.Errorf("expected no separator without model, got %q", got)
	}
}
