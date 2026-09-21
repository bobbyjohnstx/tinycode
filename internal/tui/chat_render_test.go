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
	part := PartView{Type: "tool", ToolName: "bash", ToolError: true}
	got := renderToolResultPart(part, 80)
	if !strings.Contains(got, "error") {
		t.Errorf("error result should contain 'error', got %q", got)
	}
}

func TestRenderToolResultPart_EmptyText(t *testing.T) {
	part := PartView{Type: "tool", ToolName: "bash", Text: ""}
	got := renderToolResultPart(part, 80)
	if got != "" {
		t.Errorf("empty result should produce empty output, got %q", got)
	}
}

func TestRenderToolResultPart_ShortOutput(t *testing.T) {
	part := PartView{Type: "tool", Text: "line 1\nline 2\nline 3"}
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
	if got != "" {
		t.Errorf("collapsed result should be empty, got %q", got)
	}
}

func TestRenderToolResultPart_ReadHidden(t *testing.T) {
	part := PartView{
		Type:     "tool-result",
		ToolName: "read",
		Text:     "file contents here\nline 2\nline 3",
	}
	got := renderToolResultPart(part, 80)
	if got != "" {
		t.Errorf("read tool result should be hidden, got %q", got)
	}
}

// --- renderToolCallPart tests ---

func TestRenderToolCallPart_WithArgs(t *testing.T) {
	part := PartView{
		Type:     "tool",
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
		Type:     "tool",
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

// --- unified tool part rendering tests ---

func TestRenderAssistantMessage_UnifiedToolPart(t *testing.T) {
	md := testRenderer()
	msg := MessageView{
		Info: MessageInfo{Role: "assistant"},
		Parts: []PartView{
			{
				Type:     "tool",
				ToolName: "bash",
				ToolArgs: `{"command":"echo hello"}`,
				Time:     map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}
	got := renderMessage(msg, 80, md)
	if !strings.Contains(got, "echo hello") {
		t.Error("unified tool part should render command detail")
	}
	if !strings.Contains(got, "done") {
		t.Error("completed unified tool part should show done status")
	}
}

func TestRenderAssistantMessage_UnifiedToolPartWithOutput(t *testing.T) {
	md := testRenderer()
	msg := MessageView{
		Info: MessageInfo{Role: "assistant"},
		Parts: []PartView{
			{
				Type:     "tool",
				ToolName: "bash",
				ToolArgs: `{"command":"ls"}`,
				Text:     "file1.go\nfile2.go",
				Time:     map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}
	got := renderMessage(msg, 80, md)
	if !strings.Contains(got, "file1.go") {
		t.Error("unified tool part with output should render output text")
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

// --- subagent grouping tests ---

func TestGroupSubagentParts_NoLabels(t *testing.T) {
	parts := []PartView{
		{Type: "text", Text: "hello"},
		{Type: "tool", ToolName: "bash"},
	}
	groups := groupSubagentParts(parts)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.label != "" {
			t.Errorf("expected empty label, got %q", g.label)
		}
		if len(g.parts) != 1 {
			t.Errorf("expected 1 part per group, got %d", len(g.parts))
		}
	}
}

func TestGroupSubagentParts_ConsecutiveSameLabel(t *testing.T) {
	parts := []PartView{
		{Type: "tool", ToolName: "task", SubagentLabel: "executor-1"},
		{Type: "tool", ToolName: "bash", SubagentLabel: "executor-1"},
		{Type: "tool", ToolName: "read", SubagentLabel: "executor-1"},
	}
	groups := groupSubagentParts(parts)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].label != "executor-1" {
		t.Errorf("expected label 'executor-1', got %q", groups[0].label)
	}
	if len(groups[0].parts) != 3 {
		t.Errorf("expected 3 parts, got %d", len(groups[0].parts))
	}
}

func TestGroupSubagentParts_MixedLabels(t *testing.T) {
	parts := []PartView{
		{Type: "text", Text: "intro"},
		{Type: "tool", ToolName: "task", SubagentLabel: "executor-1"},
		{Type: "tool", ToolName: "bash", SubagentLabel: "executor-1"},
		{Type: "tool", ToolName: "task", SubagentLabel: "explore-2"},
		{Type: "tool", ToolName: "read", SubagentLabel: "explore-2"},
		{Type: "text", Text: "conclusion"},
	}
	groups := groupSubagentParts(parts)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups (text, executor-1, explore-2, text), got %d", len(groups))
	}
	if groups[0].label != "" {
		t.Errorf("group 0: expected empty label, got %q", groups[0].label)
	}
	if groups[1].label != "executor-1" {
		t.Errorf("group 1: expected 'executor-1', got %q", groups[1].label)
	}
	if len(groups[1].parts) != 2 {
		t.Errorf("group 1: expected 2 parts, got %d", len(groups[1].parts))
	}
	if groups[2].label != "explore-2" {
		t.Errorf("group 2: expected 'explore-2', got %q", groups[2].label)
	}
	if len(groups[2].parts) != 2 {
		t.Errorf("group 2: expected 2 parts, got %d", len(groups[2].parts))
	}
	if groups[3].label != "" {
		t.Errorf("group 3: expected empty label, got %q", groups[3].label)
	}
}

func TestRenderSubagentGroup_Collapsed(t *testing.T) {
	g := subagentGroup{
		label: "executor-1",
		parts: []PartView{
			{
				Type:     "tool",
				ToolName: "task",
				ToolArgs: `{"description":"Run wc -l on all files"}`,
				Time:     map[string]any{"start": float64(1000), "end": float64(3400)},
			},
			{
				Type:     "tool",
				ToolName: "bash",
				ToolArgs: `{"command":"wc -l *.md"}`,
				Time:     map[string]any{"start": float64(1500), "end": float64(3000)},
			},
		},
	}
	got := renderSubagentGroup(g, false, SubagentStatus{}, 80)
	stripped := stripAnsi(got)
	if !strings.Contains(stripped, "+ executor-1") {
		t.Errorf("collapsed should show '+ executor-1', got %q", stripped)
	}
	if !strings.Contains(stripped, "Run wc -l on all files") {
		t.Errorf("collapsed should show task description, got %q", stripped)
	}
	if !strings.Contains(stripped, "done") {
		t.Errorf("collapsed should show 'done', got %q", stripped)
	}
	// Should NOT show child tool calls
	if strings.Contains(stripped, "wc -l *.md") {
		t.Errorf("collapsed should not show child tool calls, got %q", stripped)
	}
}

func TestRenderSubagentGroup_Expanded(t *testing.T) {
	g := subagentGroup{
		label: "executor-1",
		parts: []PartView{
			{
				Type:     "tool",
				ToolName: "task",
				ToolArgs: `{"description":"Run wc -l on all files"}`,
				Time:     map[string]any{"start": float64(1000), "end": float64(3400)},
			},
			{
				Type:     "tool",
				ToolName: "bash",
				ToolArgs: `{"command":"wc -l *.md"}`,
				Time:     map[string]any{"start": float64(1500), "end": float64(2500)},
			},
			{
				Type:     "tool",
				ToolName: "read",
				ToolArgs: `{"file_path":"README.md"}`,
				Time:     map[string]any{"start": float64(2600), "end": float64(3000)},
			},
		},
	}
	got := renderSubagentGroup(g, true, SubagentStatus{}, 80)
	stripped := stripAnsi(got)
	if !strings.Contains(stripped, "- executor-1") {
		t.Errorf("expanded should show '- executor-1', got %q", stripped)
	}
	if !strings.Contains(stripped, "wc -l *.md") {
		t.Errorf("expanded should show child tool command, got %q", stripped)
	}
	if !strings.Contains(stripped, "README.md") {
		t.Errorf("expanded should show child tool file path, got %q", stripped)
	}
	if !strings.Contains(got, "┃") {
		t.Errorf("expanded should show left border guide, got %q", got)
	}
}

func TestRenderSubagentGroup_WithTokens(t *testing.T) {
	g := subagentGroup{
		label: "executor-1",
		parts: []PartView{
			{
				Type:     "tool",
				ToolName: "task",
				ToolArgs: `{"description":"Test task"}`,
				Time:     map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}
	status := SubagentStatus{
		Done:         true,
		InputTokens:  800,
		OutputTokens: 400,
	}
	got := renderSubagentGroup(g, false, status, 80)
	stripped := stripAnsi(got)
	if !strings.Contains(stripped, "1k tok") {
		t.Errorf("should show token count, got %q", stripped)
	}
}

func TestRenderSubagentGroup_Running(t *testing.T) {
	g := subagentGroup{
		label: "explore-3",
		parts: []PartView{
			{
				Type:     "tool",
				ToolName: "task",
				ToolArgs: `{"description":"Read README.md"}`,
				Time:     map[string]any{"start": float64(1000)},
			},
		},
	}
	got := renderSubagentGroup(g, false, SubagentStatus{}, 80)
	stripped := stripAnsi(got)
	if !strings.Contains(stripped, "...") {
		t.Errorf("running group should show '...', got %q", stripped)
	}
}

func TestRenderAssistantMessage_SubagentGroupCollapsed(t *testing.T) {
	md := testRenderer()
	msg := MessageView{
		Info: MessageInfo{Role: "assistant"},
		Parts: []PartView{
			{Type: "text", Text: "Launching agents"},
			{
				Type:          "tool",
				ToolName:      "task",
				ToolArgs:      `{"description":"Run tests"}`,
				SubagentLabel: "executor-1",
				Time:          map[string]any{"start": float64(1000), "end": float64(2000)},
			},
			{
				Type:          "tool",
				ToolName:      "bash",
				ToolArgs:      `{"command":"go test ./..."}`,
				SubagentLabel: "executor-1",
				Time:          map[string]any{"start": float64(1200), "end": float64(1800)},
			},
		},
	}
	got := renderMessage(msg, 80, md)
	stripped := stripAnsi(got)
	if !strings.Contains(stripped, "Launching agents") {
		t.Error("should render text part before subagent group")
	}
	if !strings.Contains(stripped, "+ executor-1") {
		t.Error("should render collapsed subagent group")
	}
	// Collapsed by default, should not show child tool calls
	if strings.Contains(stripped, "go test") {
		t.Error("collapsed group should not show child tool calls")
	}
}

func TestExtractAgentType(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"executor-1", "executor"},
		{"explore-3", "explore"},
		{"code-reviewer-2", "code-reviewer"},
		{"solo", "solo"},
	}
	for _, tt := range tests {
		got := extractAgentType(tt.label)
		if got != tt.want {
			t.Errorf("extractAgentType(%q) = %q, want %q", tt.label, got, tt.want)
		}
	}
}

func TestExtractTaskDescription(t *testing.T) {
	parts := []PartView{
		{Type: "tool", ToolName: "task", ToolArgs: `{"description":"Fix the bug","prompt":"do it"}`},
		{Type: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`},
	}
	got := extractTaskDescription(parts)
	if got != "Fix the bug" {
		t.Errorf("expected 'Fix the bug', got %q", got)
	}
}

func TestExtractTaskDescription_NoTask(t *testing.T) {
	parts := []PartView{
		{Type: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`},
	}
	got := extractTaskDescription(parts)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestGroupAllDone_AllComplete(t *testing.T) {
	parts := []PartView{
		{Type: "tool", Time: map[string]any{"start": float64(1000), "end": float64(2000)}},
		{Type: "tool", Time: map[string]any{"start": float64(1500), "end": float64(2500)}},
	}
	if !groupAllDone(parts) {
		t.Error("expected all done")
	}
}

func TestGroupAllDone_OneRunning(t *testing.T) {
	parts := []PartView{
		{Type: "tool", Time: map[string]any{"start": float64(1000), "end": float64(2000)}},
		{Type: "tool", Time: map[string]any{"start": float64(1500)}},
	}
	if groupAllDone(parts) {
		t.Error("expected not all done when one is running")
	}
}
