package tui

import (
	"strings"
	"testing"
)

func TestRenderSessionHTML_producesValidStructure(t *testing.T) {
	session := SessionInfo{
		ID:        "test-123",
		Title:     "Test Session",
		CreatedAt: 1700000000000,
	}

	messages := []MessageView{
		{
			Info: MessageInfo{Role: "user"},
			Parts: []PartView{
				{Type: "text", Text: "Hello world"},
			},
		},
		{
			Info: MessageInfo{Role: "assistant", Agent: "build", ModelID: "gpt-4"},
			Parts: []PartView{
				{Type: "text", Text: "Here is some **bold** text and `code`."},
			},
		},
	}

	html, err := renderSessionHTML(session, messages)
	if err != nil {
		t.Fatalf("renderSessionHTML returned error: %v", err)
	}

	checks := []struct {
		name    string
		substr  string
	}{
		{"doctype", "<!DOCTYPE html>"},
		{"title tag", "<title>Test Session</title>"},
		{"session ID", "test-123"},
		{"header title", "<h1>Test Session</h1>"},
		{"user message class", `class="message message-user"`},
		{"assistant message class", `class="message message-assistant"`},
		{"user role label", "User"},
		{"assistant role label", "Assistant (Build"},
		{"model in label", "gpt-4"},
		{"user text rendered", "Hello world"},
		{"bold rendered", "<strong>bold</strong>"},
		{"inline code rendered", "<code>code</code>"},
		{"meta charset", `charset="utf-8"`},
		{"viewport meta", `name="viewport"`},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(html, c.substr) {
				t.Errorf("expected HTML to contain %q", c.substr)
			}
		})
	}
}

func TestRenderSessionHTML_toolCallCollapsed(t *testing.T) {
	session := SessionInfo{ID: "s1", Title: "Tools"}
	messages := []MessageView{
		{
			Info: MessageInfo{Role: "assistant"},
			Parts: []PartView{
				{Type: "tool-call", ToolName: "read_file", ToolArgs: `{"path": "/tmp/test.go"}`},
				{Type: "tool-result", Text: "file contents here"},
			},
		},
	}

	html, err := renderSessionHTML(session, messages)
	if err != nil {
		t.Fatalf("renderSessionHTML returned error: %v", err)
	}

	if !strings.Contains(html, `<details class="tool-call">`) {
		t.Error("expected tool-call to be in a <details> element")
	}
	if !strings.Contains(html, "<summary>read_file</summary>") {
		t.Error("expected tool name in summary")
	}
	if !strings.Contains(html, `<details class="tool-result">`) {
		t.Error("expected tool-result to be in a <details> element")
	}
	if !strings.Contains(html, "file contents here") {
		t.Error("expected tool result text in output")
	}
}

func TestRenderSessionHTML_emptyMessages(t *testing.T) {
	session := SessionInfo{ID: "s1"}
	html, err := renderSessionHTML(session, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "Untitled Session") {
		t.Error("expected default title for empty session")
	}
}

func TestRenderSessionHTML_reasoningCollapsed(t *testing.T) {
	session := SessionInfo{ID: "s1", Title: "Thinking"}
	messages := []MessageView{
		{
			Info: MessageInfo{Role: "assistant"},
			Parts: []PartView{
				{Type: "reasoning", Text: "Let me think about this..."},
			},
		},
	}

	html, err := renderSessionHTML(session, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `<details class="thinking">`) {
		t.Error("expected reasoning in a <details> element")
	}
	if !strings.Contains(html, "Let me think about this...") {
		t.Error("expected reasoning text in output")
	}
}

func TestRenderSessionHTML_toolResultTruncation(t *testing.T) {
	session := SessionInfo{ID: "s1", Title: "Truncation"}

	// Build a tool result with 60 lines
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, "line content")
	}
	longText := strings.Join(lines, "\n")

	messages := []MessageView{
		{
			Info: MessageInfo{Role: "assistant"},
			Parts: []PartView{
				{Type: "tool-result", Text: longText},
			},
		},
	}

	html, err := renderSessionHTML(session, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "(10 more lines)") {
		t.Error("expected truncation notice for long tool results")
	}
}

func TestRenderSessionHTML_htmlEscaping(t *testing.T) {
	session := SessionInfo{ID: "s1", Title: "Escaping"}
	messages := []MessageView{
		{
			Info: MessageInfo{Role: "assistant"},
			Parts: []PartView{
				{Type: "tool-call", ToolName: "run", ToolArgs: `<script>alert("xss")</script>`},
			},
		},
	}

	html, err := renderSessionHTML(session, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, `<script>alert`) {
		t.Error("expected HTML entities for script tags in tool args, got raw HTML")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("expected escaped script tag in tool args")
	}
}

func TestRenderSessionHTML_codeBlock(t *testing.T) {
	session := SessionInfo{ID: "s1", Title: "Code"}
	messages := []MessageView{
		{
			Info: MessageInfo{Role: "assistant"},
			Parts: []PartView{
				{Type: "text", Text: "```go\nfunc main() {}\n```"},
			},
		},
	}

	html, err := renderSessionHTML(session, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<pre>") {
		t.Error("expected <pre> for code block")
	}
	if !strings.Contains(html, "<code") {
		t.Error("expected <code> for code block")
	}
	if !strings.Contains(html, "func main()") {
		t.Error("expected code content in output")
	}
}
