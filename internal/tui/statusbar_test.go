package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestBrailleWaveSpinner_FrameCount(t *testing.T) {
	color := lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#CC4444"}
	sp := brailleWaveSpinner(color)

	if len(sp.Frames) != 32 {
		t.Errorf("expected 32 frames, got %d", len(sp.Frames))
	}
}

func TestBrailleWaveSpinner_FPS(t *testing.T) {
	color := lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#CC4444"}
	sp := brailleWaveSpinner(color)

	expected := time.Second / 20
	if sp.FPS != expected {
		t.Errorf("expected FPS %v, got %v", expected, sp.FPS)
	}
}

func TestBrailleWaveSpinner_FramesNotEmpty(t *testing.T) {
	color := lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#CC4444"}
	sp := brailleWaveSpinner(color)

	for i, frame := range sp.Frames {
		if frame == "" {
			t.Errorf("frame %d is empty", i)
		}
	}
}

func TestStatusBar_SetAgent_ChangesSpinner(t *testing.T) {
	sb := NewStatusBar(120)

	originalColor := sb.agentColor

	sb.SetAgent("architect")

	architectColor := AgentColor("architect")
	if sb.agentColor == originalColor {
		t.Error("expected agent color to change after SetAgent")
	}
	if sb.agentColor != architectColor {
		t.Errorf("expected architect color, got %v", sb.agentColor)
	}
	if len(sb.spinner.Spinner.Frames) != 32 {
		t.Errorf("expected 32 frames after agent change, got %d", len(sb.spinner.Spinner.Frames))
	}
}

func TestStatusBar_SetAgent_SameAgent_NoRegenerate(t *testing.T) {
	sb := NewStatusBar(120)

	firstFrames := sb.spinner.Spinner.Frames
	sb.SetAgent("build") // same as default

	// Should be the same object since color didn't change
	if &firstFrames[0] != &sb.spinner.Spinner.Frames[0] {
		t.Error("expected no frame regeneration for same agent color")
	}
}

func TestStatusBar_SetWorking(t *testing.T) {
	sb := NewStatusBar(120)

	cmd := sb.SetWorking(true)
	if cmd == nil {
		t.Error("expected non-nil Cmd when transitioning to working")
	}
	if !sb.working {
		t.Error("expected working to be true")
	}

	_ = sb.SetWorking(false)
	if sb.working {
		t.Error("expected working to be false")
	}
}

func TestStatusBar_LeaderPendingHints(t *testing.T) {
	sb := NewStatusBar(120)

	// Normal state should show standard hints
	view := sb.View()
	if !strings.Contains(view, "commands") {
		t.Error("expected 'commands' in normal hints")
	}
	if !strings.Contains(view, "/help") {
		t.Error("expected '/help' in normal hints")
	}

	// Leader pending should show follow-up keys
	sb.SetLeaderPending(true)
	view = sb.View()
	if !strings.Contains(view, "sidebar") {
		t.Error("expected 'sidebar' in leader pending hints")
	}
	if !strings.Contains(view, "agents") {
		t.Error("expected 'agents' in leader pending hints")
	}
	if !strings.Contains(view, "models") {
		t.Error("expected 'models' in leader pending hints")
	}
	if !strings.Contains(view, "sessions") {
		t.Error("expected 'sessions' in leader pending hints")
	}

	// Clear pending should restore normal hints
	sb.SetLeaderPending(false)
	view = sb.View()
	if !strings.Contains(view, "commands") {
		t.Error("expected 'commands' in hints after clearing leader pending")
	}
}

func TestStatusBar_ViewShowsSpinnerWhenWorking(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetWorking(true)

	view := sb.View()
	if !strings.Contains(view, "thinking") {
		t.Error("expected default activity 'thinking' while working")
	}
	// Elapsed telemetry (tokens omitted when zero).
	if !strings.Contains(view, "s") {
		t.Errorf("expected elapsed telemetry, got %q", view)
	}
	// While working, shortcut hints are hidden (avoids wrap-duplication).
	if strings.Contains(view, "ctrl+p") || strings.Contains(view, "/help") {
		t.Error("shortcut hints should be hidden while working")
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 status line while working, got %d:\n%s", len(lines), view)
	}
	if sb.Height() != 1 {
		t.Fatalf("working Height() = %d, want 1", sb.Height())
	}
}

func TestStatusBar_WorkingShowsActivityAndTelemetry(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetWorking(true)
	sb.SetActivity("bash ls -la")
	sb.SetTurnTokens(320)
	sb.turnStart = time.Now().Add(-2400 * time.Millisecond)

	view := sb.View()
	if !strings.Contains(view, "bash ls -la") {
		t.Errorf("expected activity in working view, got %q", view)
	}
	if !strings.Contains(view, "s · ") {
		t.Errorf("expected elapsed pattern, got %q", view)
	}
	if !strings.Contains(view, "320") {
		t.Errorf("expected turn tokens in view, got %q", view)
	}
	if strings.Contains(view, "ctrl+p") {
		t.Error("ctrl+p must not appear while working")
	}
	if strings.Count(view, "\n") != 0 {
		t.Fatalf("working view must be 1 line, got %q", view)
	}
	if sb.Height() != 1 {
		t.Fatalf("working Height() = %d, want 1", sb.Height())
	}
}

func TestStatusBar_SetWorkingClearsActivity(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetWorking(true)
	sb.SetActivity("read")
	sb.SetTurnTokens(99)
	_ = sb.SetWorking(false)
	if sb.activity != "" {
		t.Errorf("activity = %q, want empty after SetWorking(false)", sb.activity)
	}
	if sb.turnTokens != 0 {
		t.Errorf("turnTokens = %d, want 0 after SetWorking(false)", sb.turnTokens)
	}
}

func TestFormatActivity(t *testing.T) {
	tests := []struct {
		name string
		part PartView
		want string
	}{
		{name: "reasoning", part: PartView{Type: "reasoning"}, want: "thinking"},
		{name: "thinking", part: PartView{Type: "thinking"}, want: "thinking"},
		{name: "bash", part: PartView{Type: "tool-call", ToolName: "bash", ToolArgs: `{"command":"go test ./..."}`}, want: "bash go test ./..."},
		{name: "read basename", part: PartView{Type: "tool", ToolName: "read", ToolArgs: `{"file_path":"/tmp/foo/bar.go"}`}, want: "read bar.go"},
		{name: "webfetch", part: PartView{Type: "tool-call", ToolName: "webfetch", ToolArgs: `{"url":"https://example.com"}`}, want: "webfetch"},
		{name: "other tool", part: PartView{Type: "tool-call", ToolName: "glob"}, want: "glob"},
		{name: "text ignored", part: PartView{Type: "text", Text: "hi"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatActivity(tt.part)
			if got != tt.want {
				t.Errorf("formatActivity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderHintsLine_SingleRow(t *testing.T) {
	left := "AAAAAAAA" // 8 cols
	right := "tab agents  ctrl+p commands  /help reference"
	line := renderHintsLine(40, left, right)
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("hints line must be a single row, got %q", line)
	}
	if lipgloss.Width(line) > 40 {
		t.Fatalf("hints line width %d > 40", lipgloss.Width(line))
	}
}

func TestStatusBar_DotSeparatedFormat(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetAgent("executor")
	sb.SetModel("opus-4", "anthropic")

	view := sb.View()
	if !strings.Contains(view, "executor") {
		t.Error("expected agent name in status bar")
	}
	if !strings.Contains(view, "opus-4") {
		t.Error("expected model name in status bar")
	}
	if !strings.Contains(view, "anthropic") {
		t.Error("expected provider in status bar")
	}
	// Dot separator should be present
	if !strings.Contains(view, "·") {
		t.Error("expected dot separator in status bar")
	}
}

func TestStatusBar_EffortHiddenWhenMedium(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetAgent("build")
	sb.SetEffort("medium")

	view := sb.View()
	// "medium" should not appear in the status bar
	if strings.Contains(view, "medium") {
		t.Error("effort 'medium' should be hidden from status bar")
	}
}

func TestStatusBar_EffortShownWhenNotMedium(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetAgent("build")
	sb.SetEffort("high")

	view := sb.View()
	if !strings.Contains(view, "high") {
		t.Error("expected effort 'high' in status bar")
	}
}

func TestStatusBar_ContextPercent(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetAgent("build")
	sb.SetContextPercent(42)

	view := sb.View()
	if !strings.Contains(view, "42%") {
		t.Error("expected '42%' in status bar")
	}
	if !strings.Contains(view, "█") || !strings.Contains(view, "░") {
		t.Error("expected context meter bar glyphs in status bar")
	}
}

func TestStatusBar_ContextPercentZeroHidden(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetAgent("build")
	sb.SetContextPercent(0)

	view := sb.View()
	if strings.Contains(view, "█") || strings.Contains(view, "0%") {
		t.Error("context meter should be hidden when 0")
	}
}

func TestRenderContextMeter_Thresholds(t *testing.T) {
	low := renderContextMeter(42)
	if !strings.Contains(low, "42%") {
		t.Fatalf("low meter = %q", low)
	}
	warn := renderContextMeter(75)
	if !strings.Contains(warn, "75%") {
		t.Fatalf("warn meter = %q", warn)
	}
	hot := renderContextMeter(95)
	if !strings.Contains(hot, "95%") {
		t.Fatalf("hot meter = %q", hot)
	}
}

func TestStatusBar_Height_Idle(t *testing.T) {
	sb := NewStatusBar(120)
	if sb.Height() != 2 {
		t.Errorf("idle height = %d, want 2", sb.Height())
	}
}

func TestStatusBar_Height_GoalActive(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetGoalState("all tests pass", 3, 10)
	if sb.Height() != 5 {
		t.Errorf("goal active height = %d, want 5", sb.Height())
	}
}

func TestStatusBar_Height_GoalComplete(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetGoalComplete("all tests pass", 5)
	if sb.Height() != 3 {
		t.Errorf("goal complete height = %d, want 3", sb.Height())
	}
}

func TestStatusBar_GoalBox_Renders(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetGoalState("all tests pass", 3, 10)

	view := sb.View()
	if !strings.Contains(view, "Goal:") {
		t.Error("expected 'Goal:' in goal box")
	}
	if !strings.Contains(view, "3/10") {
		t.Error("expected '3/10' progress in goal box")
	}
	if !strings.Contains(view, "Iteration 3 of 10") {
		t.Error("expected 'Iteration 3 of 10' in goal box")
	}
	// Box drawing characters
	if !strings.Contains(view, "┌") {
		t.Error("expected top-left box corner")
	}
	if !strings.Contains(view, "┘") {
		t.Error("expected bottom-right box corner")
	}
}

func TestStatusBar_GoalComplete_SuccessLine(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetGoalComplete("all tests pass", 5)

	view := sb.View()
	if !strings.Contains(view, "✓") {
		t.Error("expected checkmark in success line")
	}
	if !strings.Contains(view, "Goal met") {
		t.Error("expected 'Goal met' in success line")
	}
	if !strings.Contains(view, "5 iterations") {
		t.Error("expected '5 iterations' in success line")
	}
}

func TestStatusBar_GoalFadeMsg_ClearsGoal(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetGoalComplete("all tests pass", 5)

	if sb.Height() != 3 {
		t.Fatalf("expected height 3 before fade, got %d", sb.Height())
	}

	sb, _ = sb.Update(goalFadeMsg{})
	if sb.goalDisplay != nil {
		t.Error("expected goalDisplay to be nil after fade")
	}
	if sb.Height() != 2 {
		t.Errorf("expected height 2 after fade, got %d", sb.Height())
	}
}

func TestStatusBar_SetGoalClearsDisplay(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetGoalState("all tests pass", 3, 10)
	if sb.goalDisplay == nil {
		t.Fatal("expected goalDisplay to be set")
	}

	sb.SetGoal("")
	if sb.goalDisplay != nil {
		t.Error("expected goalDisplay to be nil after SetGoal('')")
	}
}
