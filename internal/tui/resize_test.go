package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestApp_WindowSizeResizesChrome(t *testing.T) {
	app := NewApp("http://localhost:4096")
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app = model.(App)
	if app.width != 100 || app.height != 40 {
		t.Fatalf("size = %dx%d, want 100x40", app.width, app.height)
	}
	if app.prompt.width != 100 {
		t.Fatalf("prompt width = %d, want 100", app.prompt.width)
	}
	if app.status.width != 100 {
		t.Fatalf("status width = %d, want 100", app.status.width)
	}
	if app.chat.width != 100 {
		t.Fatalf("chat width = %d, want 100", app.chat.width)
	}

	model, _ = app.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	app = model.(App)
	if app.prompt.width != 160 {
		t.Fatalf("after resize prompt width = %d, want 160", app.prompt.width)
	}
	if app.status.width != 160 {
		t.Fatalf("after resize status width = %d, want 160", app.status.width)
	}
	if app.chat.width != 160 {
		t.Fatalf("after resize chat width = %d, want 160", app.chat.width)
	}
	if app.chat.viewport.Width != 160 {
		t.Fatalf("viewport width = %d, want 160", app.chat.viewport.Width)
	}
}

func TestPromptHeight_MatchesChrome(t *testing.T) {
	p := NewPromptInput(80)
	if p.Height() != 5 { // top + 3 textarea + bottom
		t.Fatalf("Height() = %d, want 5", p.Height())
	}
	p.SetThinkingLevel("high")
	if p.Height() != 6 {
		t.Fatalf("Height() with meta = %d, want 6", p.Height())
	}
}

func TestPromptView_SlashPopoverNotJoinedIntoChrome(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCommands(testCommands())
	p.SetValue("/")
	p.autocomplete.UpdateInput("/")
	if !p.autocomplete.IsVisible() {
		t.Fatal("expected autocomplete visible")
	}
	viewLines := strings.Count(p.View(), "\n") + 1
	if viewLines != p.Height() {
		t.Fatalf("View lines=%d Height=%d; popover must not inflate prompt chrome", viewLines, p.Height())
	}
	if p.PopoverView() == "" {
		t.Fatal("expected PopoverView when slash autocomplete is visible")
	}
}

func TestPlacePromptPopover_SitsAbovePromptChrome(t *testing.T) {
	width, height := 40, 20
	statusH, promptH := 2, 5
	base := strings.Repeat(strings.Repeat(".", width)+"\n", height)
	base = strings.TrimSuffix(base, "\n")
	panel := "POPOVER-LINE"
	out := placePromptPopover(base, panel, width, height, statusH, promptH, 0)
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("lines=%d, want %d", len(lines), height)
	}
	wantY := height - statusH - promptH - 1
	if !strings.Contains(lines[wantY], "POPOVER-LINE") {
		t.Fatalf("popover not at y=%d: %q", wantY, lines[wantY])
	}
	// Prompt/status rows below must be untouched.
	for y := wantY + 1; y < height; y++ {
		if strings.Contains(lines[y], "POPOVER") {
			t.Fatalf("popover leaked into chrome row %d: %q", y, lines[y])
		}
	}
}

func TestComposeView_DoesNotInflateChromeHeight(t *testing.T) {
	for _, w := range []int{60, 80, 100, 120, 200} {
		sb := NewStatusBar(w)
		sb.SetWorking(true)
		sb.SetAgent("build")
		sb.SetModel("ornith-1.0-9b-mlx", "lm-studio")
		sb.SetContextPercent(58)
		status := sb.View()

		p := NewPromptInput(w)
		prompt := p.View()

		l := calculateLayout(w, 40, false, p.Height(), sb.Height())
		composed := composeView("chat", prompt, status, "", l)
		lines := strings.Split(composed, "\n")
		// mainArea is Height(chatHeight) so exactly chatHeight lines, plus prompt+status
		wantMax := l.chatHeight + p.Height() + sb.Height()
		// Allow +0; inflation means wrap ghosts
		if len(lines) > wantMax {
			t.Fatalf("width=%d: composed lines=%d > layout budget %d (chat=%d promptH=%d statusH=%d)\nprompt lines=%d status lines=%d\nstatus=%q",
				w, len(lines), wantMax, l.chatHeight, p.Height(), sb.Height(),
				strings.Count(prompt, "\n")+1, strings.Count(status, "\n")+1, status)
		}
		for i, ln := range strings.Split(status, "\n") {
			if lw := lipgloss.Width(ln); lw > w {
				t.Fatalf("status line %d width %d > %d: %q", i, lw, w, ln)
			}
		}
		for i, ln := range strings.Split(prompt, "\n") {
			if lw := lipgloss.Width(ln); lw > w {
				t.Fatalf("prompt line %d width %d > %d", i, lw, w)
			}
		}
	}
}
