package tui

import (
	"strings"
	"testing"
)

func TestWelcomeView_MarkDone(t *testing.T) {
	w := NewWelcomeView()

	for _, check := range bootChecks {
		w.MarkDone(check.key)
	}

	if !w.bootDone {
		t.Error("expected bootDone to be true after marking all checks done")
	}
}

func TestWelcomeView_PartialNotDone(t *testing.T) {
	w := NewWelcomeView()

	w.MarkDone("config")
	w.MarkDone("sse")

	if w.bootDone {
		t.Error("expected bootDone to be false when not all checks are done")
	}
}

func TestWelcomeView_MarkFailed(t *testing.T) {
	w := NewWelcomeView()

	for _, check := range bootChecks {
		if check.key == "mcp" {
			w.MarkFailed(check.key)
		} else {
			w.MarkDone(check.key)
		}
	}

	if !w.bootDone {
		t.Error("expected bootDone to be true even with a failed check")
	}

	view := w.View(120, 30, "", "", 0, 0, 0, 0)
	if !strings.Contains(view, "✗") {
		t.Error("expected ✗ in view for failed check")
	}
}

func TestWelcomeView_Tick(t *testing.T) {
	w := NewWelcomeView()
	initial := w.logoShow

	cmd := w.Tick()
	if cmd == nil {
		t.Error("expected non-nil Cmd from Tick")
	}
	if w.logoShow != initial+1 {
		t.Errorf("expected logoShow to increment from %d to %d, got %d", initial, initial+1, w.logoShow)
	}
}

func TestWelcomeView_TickStopsAfterFull(t *testing.T) {
	w := NewWelcomeView()

	for _, check := range bootChecks {
		w.MarkDone(check.key)
	}

	// Tick past the logo rows
	for i := 0; i < len(logoData)+5; i++ {
		w.Tick()
	}

	cmd := w.Tick()
	if cmd != nil {
		t.Error("expected nil Cmd after boot done and logo fully shown")
	}
}

func TestWelcomeView_ViewShowsCounts(t *testing.T) {
	w := NewWelcomeView()
	for _, check := range bootChecks {
		w.MarkDone(check.key)
	}
	// Show full logo
	for i := 0; i < len(logoData)+1; i++ {
		w.Tick()
	}

	view := w.View(120, 30, "LM Studio", "ornith-1.0-9b-mlx", 9, 19, 2, 1)

	if !strings.Contains(view, "19 agents") {
		t.Error("expected '19 agents' in view")
	}
	if !strings.Contains(view, "9 sessions") {
		t.Error("expected '9 sessions' in view")
	}
	if !strings.Contains(view, "2 plugins") {
		t.Error("expected '2 plugins' in view")
	}
	if !strings.Contains(view, "1 servers") {
		t.Error("expected '1 servers' in view")
	}
	if !strings.Contains(view, "LM Studio") {
		t.Error("expected 'LM Studio' in view")
	}
}

func TestWelcomeView_TipsAfterBoot(t *testing.T) {
	w := NewWelcomeView()

	// Before boot completes
	view := w.View(120, 30, "", "", 0, 0, 0, 0)
	if strings.Contains(view, "/ commands") {
		t.Error("tips should not appear before boot completes")
	}

	// Complete boot
	for _, check := range bootChecks {
		w.MarkDone(check.key)
	}

	view = w.View(120, 30, "", "", 0, 0, 0, 0)
	if !strings.Contains(view, "/ commands") {
		t.Error("expected tips to appear after boot completes")
	}
	if !strings.Contains(view, "ctrl+p palette") {
		t.Error("expected 'ctrl+p palette' in tips")
	}
	if !strings.Contains(view, "ctrl+x sidebar/sessions") {
		t.Error("expected 'ctrl+x sidebar/sessions' in second tips line")
	}
	if !strings.Contains(view, "/help reference") {
		t.Error("expected '/help reference' in second tips line")
	}
}

func TestWelcomeView_ViewContainsLogo(t *testing.T) {
	w := NewWelcomeView()
	// Show all logo rows
	for i := 0; i < len(logoData)+1; i++ {
		w.Tick()
	}

	view := w.View(120, 30, "", "", 0, 0, 0, 0)
	if !strings.Contains(view, "▀") {
		t.Error("expected logo block characters in view")
	}
}

func TestWelcomeView_ViewContainsChecks(t *testing.T) {
	w := NewWelcomeView()
	view := w.View(120, 30, "", "", 0, 0, 0, 0)

	if !strings.Contains(view, "Loading configuration") {
		t.Error("expected 'Loading configuration' in view")
	}
	if !strings.Contains(view, "Connecting to server") {
		t.Error("expected 'Connecting to server' in view")
	}
}
