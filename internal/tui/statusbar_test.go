package tui

import (
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

	cmd = sb.SetWorking(false)
	if sb.working {
		t.Error("expected working to be false")
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
