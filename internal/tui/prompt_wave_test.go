package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptWave_StartsOn(t *testing.T) {
	p := NewPromptInput(40)
	if !p.waveOn {
		t.Fatal("expected waveOn by default")
	}
	line := p.renderWaveLine()
	if line == "" {
		t.Fatal("expected non-empty wave line")
	}
	if strings.Contains(line, "─") {
		t.Error("wave line should use braille, not box-drawing dashes")
	}
}

func TestPromptWave_TickAdvancesPhase(t *testing.T) {
	p := NewPromptInput(40)
	p, cmd := p.Update(promptWaveTickMsg{})
	if p.wavePhase != 1 {
		t.Fatalf("wavePhase = %d, want 1", p.wavePhase)
	}
	if cmd == nil {
		t.Fatal("expected tick cmd while wave is on")
	}
}

func TestPromptWave_StopsOnFirstSubmit(t *testing.T) {
	p := NewPromptInput(40)
	p.SetValue("hello")
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.waveOn {
		t.Fatal("expected waveOn false after first submit")
	}
	if cmd == nil {
		t.Fatal("expected PromptSubmittedMsg cmd")
	}
	msg := cmd()
	if _, ok := msg.(PromptSubmittedMsg); !ok {
		t.Fatalf("got %T, want PromptSubmittedMsg", msg)
	}

	p, cmd = p.Update(promptWaveTickMsg{})
	if p.wavePhase != 0 {
		t.Fatalf("phase should not advance after stop, got %d", p.wavePhase)
	}
	if cmd != nil {
		t.Fatal("expected nil tick cmd after wave stops")
	}

	view := p.View()
	if !strings.Contains(view, "─") {
		t.Error("expected quiet rule after wave stops")
	}
}

func TestPromptWave_EmptyEnterKeepsWave(t *testing.T) {
	p := NewPromptInput(40)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.waveOn {
		t.Fatal("empty enter should not stop the wave")
	}
}
