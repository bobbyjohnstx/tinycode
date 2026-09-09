package render

import (
	"strings"
	"testing"
)

// --- ToolIcon tests ---

func TestToolIcon_KnownTools(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"bash", "$"},
		{"shell", "$"},
		{"read", "▸"},
		{"write", "←"},
		{"edit", "←"},
		{"glob", "≡"},
		{"grep", "⌕"},
		{"web_fetch", "☁"},
		{"webfetch", "☁"},
		{"task", "■"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolIcon(tt.name)
			if got != tt.expected {
				t.Errorf("ToolIcon(%q) = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

func TestToolIcon_UnknownTool(t *testing.T) {
	got := ToolIcon("unknown_tool")
	if got != "•" {
		t.Errorf("ToolIcon(unknown) = %q, want %q", got, "•")
	}
}

func TestToolIcon_CaseInsensitive(t *testing.T) {
	if ToolIcon("Bash") != "$" {
		t.Error("ToolIcon should be case-insensitive")
	}
	if ToolIcon("READ") != "▸" {
		t.Error("ToolIcon should be case-insensitive")
	}
}

// --- RenderToolInline tests ---

func TestRenderToolInline_BashCommand(t *testing.T) {
	args := `{"command":"ls -la"}`
	out := RenderToolInline("bash", args, false)
	if !strings.Contains(out, "$") {
		t.Error("bash inline should contain $ icon")
	}
	if !strings.Contains(out, "ls -la") {
		t.Error("bash inline should contain command text")
	}
}

func TestRenderToolInline_ReadFile(t *testing.T) {
	args := `{"file_path":"/tmp/foo.go"}`
	out := RenderToolInline("read", args, false)
	if !strings.Contains(out, "/tmp/foo.go") {
		t.Error("read inline should contain file path")
	}
}

func TestRenderToolInline_Error(t *testing.T) {
	out := RenderToolInline("bash", `{"command":"fail"}`, true)
	if !strings.Contains(out, "error") {
		t.Error("error inline should contain 'error'")
	}
}

func TestRenderToolInline_EmptyArgs(t *testing.T) {
	out := RenderToolInline("bash", "", false)
	if out == "" {
		t.Error("should produce output even with empty args")
	}
}

func TestRenderToolInline_InvalidJSON(t *testing.T) {
	out := RenderToolInline("bash", "not json", false)
	if out == "" {
		t.Error("should produce output with invalid JSON args")
	}
}

// --- RenderToolBlock tests ---

func TestRenderToolBlock_BashCommand(t *testing.T) {
	args := `{"command":"go build ./..."}`
	out := RenderToolBlock("bash", args, 60, false)
	if !strings.Contains(out, "go build ./...") {
		t.Error("bash block should contain command text")
	}
}

func TestRenderToolBlock_SmallWidth(t *testing.T) {
	args := `{"command":"ls"}`
	out := RenderToolBlock("bash", args, 5, false)
	if out == "" {
		t.Error("should handle small width gracefully")
	}
}

// --- Markdown streaming tests ---

func TestMarkdownRenderer_RenderFinal_Empty(t *testing.T) {
	r := NewMarkdownRenderer(80)
	got := r.RenderFinal("")
	if got != "" {
		t.Errorf("RenderFinal('') = %q, want empty", got)
	}
}

func TestMarkdownRenderer_RenderFinal_SimpleText(t *testing.T) {
	r := NewMarkdownRenderer(80)
	got := r.RenderFinal("hello world")
	if !strings.Contains(got, "hello world") {
		t.Errorf("RenderFinal should contain 'hello world', got %q", got)
	}
}

func TestMarkdownRenderer_RenderStreaming_NoDoubleNewline(t *testing.T) {
	r := NewMarkdownRenderer(80)
	got := r.RenderStreaming("partial text without separator")
	if got != "partial text without separator" {
		t.Errorf("streaming without double-newline should return raw text, got %q", got)
	}
}

func TestMarkdownRenderer_RenderStreaming_SplitsOnDoubleNewline(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "complete block\n\nincomplete"
	got := r.RenderStreaming(input)
	// The complete part should be rendered (glamour may add formatting).
	if !strings.Contains(got, "complete block") {
		t.Errorf("streaming should contain rendered complete block, got %q", got)
	}
	// The trailing part should appear as raw text.
	if !strings.Contains(got, "incomplete") {
		t.Errorf("streaming should contain raw trailing text, got %q", got)
	}
}

func TestMarkdownRenderer_RenderStreaming_Empty(t *testing.T) {
	r := NewMarkdownRenderer(80)
	got := r.RenderStreaming("")
	if got != "" {
		t.Errorf("RenderStreaming('') = %q, want empty", got)
	}
}

func TestMarkdownRenderer_RenderStreaming_MultipleBlocks(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "first block\n\nsecond block\n\ntrailing"
	got := r.RenderStreaming(input)
	if !strings.Contains(got, "first block") {
		t.Error("should contain first block")
	}
	if !strings.Contains(got, "second block") {
		t.Error("should contain second block")
	}
	if !strings.Contains(got, "trailing") {
		t.Error("should contain trailing text")
	}
}

// --- truncate tests ---

func TestTruncate_ShortString(t *testing.T) {
	got := truncate("short", 20)
	if got != "short" {
		t.Errorf("short string should not be truncated, got %q", got)
	}
}

func TestTruncate_LongString(t *testing.T) {
	input := "this is a long string that should be truncated"
	got := truncate(input, 20)
	if len(got) > len(input) {
		t.Errorf("truncated string should be shorter than input")
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("truncated string should end with ellipsis")
	}
	// The truncated string should have maxLen-1 bytes from original + ellipsis.
	if !strings.HasPrefix(got, input[:19]) {
		t.Error("truncated string should preserve prefix of original")
	}
}

func TestFirstLine_SingleLine(t *testing.T) {
	got := firstLine("single line")
	if got != "single line" {
		t.Errorf("single line should return as-is, got %q", got)
	}
}

func TestFirstLine_MultiLine(t *testing.T) {
	got := firstLine("first\nsecond\nthird")
	if got != "first" {
		t.Errorf("firstLine should return 'first', got %q", got)
	}
}
