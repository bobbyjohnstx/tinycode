package tui

import (
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/render"
)

func testRenderer() *render.MarkdownRenderer {
	return render.NewMarkdownRenderer(80)
}

// --- renderTextPart tests ---

func TestRenderTextPart_Empty(t *testing.T) {
	md := testRenderer()
	part := PartView{Type: "text", Text: ""}
	got := renderTextPart(part, 80, md)
	if got != "" {
		t.Errorf("empty text should produce empty output, got %q", got)
	}
}

func TestRenderTextPart_Final(t *testing.T) {
	md := testRenderer()
	part := PartView{Type: "text", Text: "hello world", Streaming: false}
	got := renderTextPart(part, 80, md)
	if !strings.Contains(got, "hello world") {
		t.Errorf("final render should contain text, got %q", got)
	}
}

func TestRenderTextPart_Streaming(t *testing.T) {
	md := testRenderer()
	part := PartView{Type: "text", Text: "streaming text", Streaming: true}
	got := renderTextPart(part, 80, md)
	if !strings.Contains(got, "streaming text") {
		t.Errorf("streaming render should contain text, got %q", got)
	}
}

func TestRenderTextPart_StreamingWithBlocks(t *testing.T) {
	md := testRenderer()
	part := PartView{
		Type:      "text",
		Text:      "complete block\n\nincomplete",
		Streaming: true,
	}
	got := renderTextPart(part, 80, md)
	if !strings.Contains(got, "complete block") {
		t.Error("streaming should contain rendered complete block")
	}
	if !strings.Contains(got, "incomplete") {
		t.Error("streaming should contain raw trailing text")
	}
}

// --- renderToolResultPart tests ---

func TestRenderToolResultPart_Error(t *testing.T) {
	part := PartView{Type: "tool-result", ToolName: "bash", ToolError: true}
	got := renderToolResultPart(part, 80)
	if !strings.Contains(got, "error") {
		t.Errorf("error result should contain 'error', got %q", got)
	}
}

func TestRenderToolResultPart_EmptyText(t *testing.T) {
	part := PartView{Type: "tool-result", ToolName: "bash", Text: ""}
	got := renderToolResultPart(part, 80)
	if got != "" {
		t.Errorf("empty result should produce empty output, got %q", got)
	}
}

func TestRenderToolResultPart_ShortOutput(t *testing.T) {
	part := PartView{Type: "tool-result", Text: "line 1\nline 2\nline 3"}
	got := renderToolResultPart(part, 80)
	if !strings.Contains(got, "line 1") {
		t.Error("short result should contain all lines")
	}
	if strings.Contains(got, "more lines") {
		t.Error("short result should not show truncation indicator")
	}
}

func TestRenderToolResultPart_TruncatedOutput(t *testing.T) {
	lines := make([]string, 15)
	for i := range lines {
		lines[i] = "output line"
	}
	part := PartView{Type: "tool-result", Text: strings.Join(lines, "\n")}
	got := renderToolResultPart(part, 80)
	if !strings.Contains(got, "5 more lines") {
		t.Errorf("truncated result should show '5 more lines', got %q", got)
	}
}

func TestRenderToolResultPart_Collapsed(t *testing.T) {
	part := PartView{
		Type:      "tool-result",
		Text:      "some output",
		Collapsed: true,
	}
	got := renderToolResultPart(part, 80)
	if !strings.Contains(got, "collapsed") {
		t.Errorf("collapsed result should show collapsed indicator, got %q", got)
	}
}

// --- renderToolCallPart tests ---

func TestRenderToolCallPart_WithArgs(t *testing.T) {
	part := PartView{
		Type:     "tool-call",
		ToolName: "bash",
		ToolArgs: `{"command":"ls -la"}`,
		Time:     map[string]any{"end": float64(1)},
	}
	got := renderToolCallPart(part)
	if !strings.Contains(got, "ls -la") {
		t.Error("tool call should show command detail")
	}
	if !strings.Contains(got, "done") {
		t.Error("completed tool call should show done status")
	}
}

func TestRenderToolCallPart_InProgress(t *testing.T) {
	part := PartView{
		Type:     "tool-call",
		ToolName: "read",
		ToolArgs: `{"file_path":"/tmp/foo.go"}`,
	}
	got := renderToolCallPart(part)
	if !strings.Contains(got, "/tmp/foo.go") {
		t.Error("tool call should show file path detail")
	}
	if !strings.Contains(got, "...") {
		t.Error("in-progress tool call should show spinner")
	}
}

// --- renderMessage tests ---

func TestRenderMessage_UserRole(t *testing.T) {
	md := testRenderer()
	msg := MessageView{
		Info:  MessageInfo{Role: "user"},
		Parts: []PartView{{Type: "text", Text: "hello"}},
	}
	got := renderMessage(msg, 80, md)
	if !strings.Contains(got, "hello") {
		t.Error("user message should contain text")
	}
}

func TestRenderMessage_AssistantRole(t *testing.T) {
	md := testRenderer()
	msg := MessageView{
		Info:  MessageInfo{Role: "assistant"},
		Parts: []PartView{{Type: "text", Text: "response"}},
	}
	got := renderMessage(msg, 80, md)
	if !strings.Contains(got, "response") {
		t.Error("assistant message should contain text")
	}
}

// --- toolStatus tests ---

func TestToolStatus_Completed(t *testing.T) {
	part := PartView{Time: map[string]any{"end": float64(1234567890)}}
	got := toolStatus(part)
	if !strings.Contains(got, "done") {
		t.Errorf("completed tool should show 'done', got %q", got)
	}
}

func TestToolStatus_InProgress(t *testing.T) {
	part := PartView{}
	got := toolStatus(part)
	if !strings.Contains(got, "...") {
		t.Errorf("in-progress tool should show '...', got %q", got)
	}
}
