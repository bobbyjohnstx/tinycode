package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestPromptView_LineCountMatchesHeight(t *testing.T) {
	for _, w := range []int{40, 60, 80, 100, 160} {
		p := NewPromptInput(w)
		view := p.View()
		lines := strings.Count(view, "\n") + 1
		if lines != p.Height() {
			t.Fatalf("w=%d: View lines=%d Height()=%d\n%s", w, lines, p.Height(), view)
		}
		p.SetThinkingLevel("high")
		view = p.View()
		lines = strings.Count(view, "\n") + 1
		if lines != p.Height() {
			t.Fatalf("w=%d meta: View lines=%d Height()=%d", w, lines, p.Height())
		}
	}
}

func TestStatusView_LineCountMatchesHeight(t *testing.T) {
	for _, w := range []int{40, 60, 80, 120} {
		sb := NewStatusBar(w)
		sb.SetAgent("build")
		sb.SetModel("ornith-1.0-9b-mlx", "lm-studio")
		sb.SetContextPercent(58)
		view := sb.View()
		lines := strings.Count(view, "\n") + 1
		if lines != sb.Height() {
			t.Fatalf("idle w=%d: View lines=%d Height()=%d\n%q", w, lines, sb.Height(), view)
		}
		sb.SetWorking(true)
		view = sb.View()
		lines = strings.Count(view, "\n") + 1
		if lines != sb.Height() {
			t.Fatalf("working w=%d: View lines=%d Height()=%d\n%q", w, lines, sb.Height(), view)
		}
		sb.SetWorking(false)
		sb.SetGoalState("do the thing", 1, 5)
		view = sb.View()
		lines = strings.Count(view, "\n") + 1
		if lines != sb.Height() {
			t.Fatalf("goal w=%d: View lines=%d Height()=%d\n%q", w, lines, sb.Height(), view)
		}
	}
}

func TestAppView_FitsTerminal(t *testing.T) {
	app := NewApp("http://localhost:4096")
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = model.(App)
	app.status.SetWorking(true)
	app.status.SetAgent("build")
	app.status.SetModel("ornith-1.0-9b-mlx", "lm-studio")
	app.status.SetContextPercent(58)
	app.resize()
	view := app.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 30 {
		t.Fatalf("App.View lines=%d > terminal height 30", len(lines))
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) > 100 {
			t.Fatalf("line %d width %d > 100", i, lipgloss.Width(ln))
		}
	}
	// Shrink
	model, _ = app.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	app = model.(App)
	view = app.View()
	lines = strings.Split(view, "\n")
	if len(lines) > 24 {
		t.Fatalf("after shrink View lines=%d > 24", len(lines))
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) > 70 {
			t.Fatalf("after shrink line %d width %d > 70", i, lipgloss.Width(ln))
		}
	}
}
