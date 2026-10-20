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
	// Guard against themeAgentColor being set by a prior test (e.g. theme
	// selection tests that call ApplyColorTheme). When non-nil, AgentColor()
	// returns the same color for every agent, making the comparison below
	// always equal.
	savedAgentColor := themeAgentColor
	themeAgentColor = nil
	t.Cleanup(func() { themeAgentColor = savedAgentColor })

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
	if !strings.Contains(view, "ctrl+k") {
		t.Error("expected 'ctrl+k' in normal hints")
	}
	if !strings.Contains(view, "ctrl+f") {
		t.Error("expected 'ctrl+f' in normal hints")
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
	if !strings.Contains(view, "ctrl+k") {
		t.Error("expected 'ctrl+k' in hints after clearing leader pending")
	}
}

func TestStatusBar_SetConnection_ShowsOfflineAndReconnecting(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetConnection(false, false)
	view := sb.View()
	if !strings.Contains(view, "offline") {
		t.Errorf("expected offline when disconnected, got %q", view)
	}

	sb.SetConnection(false, true)
	view = sb.View()
	if !strings.Contains(view, "reconnecting") {
		t.Errorf("expected reconnecting, got %q", view)
	}
	if strings.Contains(view, "offline") {
		t.Errorf("expected no offline while reconnecting, got %q", view)
	}

	sb.SetConnection(true, false)
	view = sb.View()
	if strings.Contains(view, "offline") || strings.Contains(view, "reconnecting") {
		t.Errorf("expected no connection chrome when connected, got %q", view)
	}
}

func TestStatusBar_ViewShowsSpinnerWhenWorking(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetWorking(true)

	view := sb.View()
	if !strings.Contains(view, "thinking") {
		t.Error("expected default activity 'thinking' while working")
	}
	if !strings.Contains(view, "interrupt") {
		t.Error("expected interrupt hint while working")
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
	if !strings.Contains(view, "interrupt") {
		t.Error("expected interrupt hint alongside activity")
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

func TestFormatElapsed_Table(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0.0s"},
		{500 * time.Millisecond, "0.5s"},
		{2400 * time.Millisecond, "2.4s"},
		{59*time.Second + 900*time.Millisecond, "59.9s"},
		{60 * time.Second, "1m0s"},
		{90 * time.Second, "1m30s"},
		{-5 * time.Second, "0.0s"},
		{5*time.Minute + 15*time.Second, "5m15s"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatElapsed(tt.d)
			if got != tt.want {
				t.Errorf("formatElapsed(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestShortenCwd_HomeDirReplaced(t *testing.T) {
	got := shortenCwd("/tmp/not-home/project")
	if strings.HasPrefix(got, "~") {
		t.Errorf("non-home path should not be shortened: %q", got)
	}
}

func TestShortenCwd_IdentityForNonHome(t *testing.T) {
	got := shortenCwd("/usr/local/bin")
	if got != "/usr/local/bin" {
		t.Errorf("expected /usr/local/bin, got %q", got)
	}
}

func TestClampStatusLine_SmallWidth(t *testing.T) {
	got := clampStatusLine(0, "content")
	if got == "" {
		t.Error("expected non-empty even with 0 width (clamped to 1)")
	}
}

func TestClampStatusLine_NormalWidth(t *testing.T) {
	got := clampStatusLine(20, "hi")
	if got == "" {
		t.Error("expected non-empty for normal width")
	}
}

func TestRenderHintsLine_EmptyRight(t *testing.T) {
	got := renderHintsLine(40, "left side", "")
	if got == "" {
		t.Error("expected non-empty for empty right")
	}
}

func TestRenderHintsLine_BothSides(t *testing.T) {
	got := renderHintsLine(80, "left", "right")
	if got == "" {
		t.Error("expected non-empty with both sides")
	}
}

func TestRenderHintsLine_NarrowWidth(t *testing.T) {
	got := renderHintsLine(5, "very long left side", "right")
	if got == "" {
		t.Error("expected non-empty even when narrow")
	}
}

func TestSetSafeMode(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetSafeMode(true)
	if !sb.safeMode {
		t.Error("expected safeMode true")
	}
	sb.SetSafeMode(false)
	if sb.safeMode {
		t.Error("expected safeMode false")
	}
}

func TestSetCwd(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetCwd("/tmp/project")
	if sb.Cwd() != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %q", sb.Cwd())
	}
	view := sb.View()
	if !strings.Contains(view, "project") {
		t.Errorf("expected cwd basename in idle view, got %q", view)
	}
}

func TestSetGoal_ClearAndSet(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetGoal("implement feature")
	if sb.goalDisplay == nil {
		t.Fatal("expected goalDisplay set")
	}
	if sb.goalDisplay.text != "implement feature" {
		t.Errorf("goal text = %q", sb.goalDisplay.text)
	}
	sb.SetGoal("")
	if sb.goalDisplay != nil {
		t.Error("expected goalDisplay cleared")
	}
}

func TestSetGoal_AlreadySet(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetGoal("first")
	sb.SetGoal("second")
	if sb.goalDisplay.text != "first" {
		t.Errorf("SetGoal on existing should not overwrite text, got %q", sb.goalDisplay.text)
	}
}

func TestSetGoalState_Full(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetGoalState("fixing bug", 3, 5)
	if sb.goalDisplay == nil {
		t.Fatal("expected goalDisplay set")
	}
	if sb.goalDisplay.text != "fixing bug" {
		t.Errorf("goal text = %q", sb.goalDisplay.text)
	}
	if sb.goalDisplay.iteration != 3 || sb.goalDisplay.maxIterations != 5 {
		t.Errorf("iterations = %d/%d", sb.goalDisplay.iteration, sb.goalDisplay.maxIterations)
	}
}

func TestSetGoalState_ClearOnEmpty(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetGoalState("first", 1, 3)
	sb.SetGoalState("", 0, 0)
	if sb.goalDisplay != nil {
		t.Error("expected goalDisplay cleared when text is empty")
	}
}

func TestAccent_ReturnsColor(t *testing.T) {
	color := accent()
	if color == nil {
		t.Fatal("expected non-nil color")
	}
}
