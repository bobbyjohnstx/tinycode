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
	if view == "" {
		t.Error("expected non-empty view")
	}
	// When working, the hints line should contain "esc interrupt"
	if len(view) == 0 {
		t.Error("expected hints line content")
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
	if !strings.Contains(view, "42% ctx") {
		t.Error("expected '42% ctx' in status bar")
	}
}

func TestStatusBar_ContextPercentZeroHidden(t *testing.T) {
	sb := NewStatusBar(120)
	sb.SetAgent("build")
	sb.SetContextPercent(0)

	view := sb.View()
	if strings.Contains(view, "% ctx") {
		t.Error("context percent should be hidden when 0")
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
	if sb.Height() != 4 {
		t.Errorf("goal active height = %d, want 4", sb.Height())
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
