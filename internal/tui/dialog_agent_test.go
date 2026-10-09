package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

func TestAgentDialog_InitiallyHidden(t *testing.T) {
	d := NewAgentDialog()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden initially")
	}
}

func TestAgentDialog_ShowMakesVisible(t *testing.T) {
	d := NewAgentDialog()
	agents := []api.AgentInfo{
		{Name: "coder", Description: "Writes code"},
	}
	d.Show(agents)
	if !d.IsVisible() {
		t.Fatal("expected dialog to be visible after Show")
	}
	if d.selected != 0 {
		t.Errorf("expected selected=0, got %d", d.selected)
	}
}

func TestAgentDialog_HideMakesInvisible(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "coder"}})
	d.Hide()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden after Hide")
	}
}

func TestAgentDialog_RefreshPreservesSelection(t *testing.T) {
	d := NewAgentDialog()
	agents := []api.AgentInfo{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}
	d.Show(agents)
	d.selected = 2 // gamma

	newAgents := []api.AgentInfo{
		{Name: "beta"},
		{Name: "gamma"},
		{Name: "alpha"},
	}
	d.Refresh(newAgents)
	if d.selected != 1 {
		t.Errorf("expected selected=1 (gamma in new list), got %d", d.selected)
	}
}

func TestAgentDialog_RefreshClampsWhenSelectedRemoved(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	d.selected = 2

	d.Refresh([]api.AgentInfo{{Name: "a"}})
	if d.selected != 0 {
		t.Errorf("expected selected clamped to 0, got %d", d.selected)
	}
}

func TestAgentDialog_NavigateDownWraps(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}, {Name: "b"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 1 {
		t.Errorf("expected selected=1, got %d", d.selected)
	}

	// Wrap around
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 0 {
		t.Errorf("expected selected to wrap to 0, got %d", d.selected)
	}
}

func TestAgentDialog_NavigateUpWraps(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}, {Name: "b"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.selected != 1 {
		t.Errorf("expected selected to wrap to 1, got %d", d.selected)
	}
}

func TestAgentDialog_ArrowKeys(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}, {Name: "b"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != 1 {
		t.Errorf("expected selected=1 after down, got %d", d.selected)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.selected != 0 {
		t.Errorf("expected selected=0 after up, got %d", d.selected)
	}
}

func TestAgentDialog_EnterSelectsEnabledAgent(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "coder", Description: "Writes code"},
		{Name: "reviewer", Description: "Reviews code"},
	})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on enter")
	}
	if cmd == nil {
		t.Fatal("expected command from enter")
	}
	msg := cmd()
	selected, ok := msg.(AgentSelectedMsg)
	if !ok {
		t.Fatalf("expected AgentSelectedMsg, got %T", msg)
	}
	if selected.Agent != "reviewer" {
		t.Errorf("expected Agent=%q, got %q", "reviewer", selected.Agent)
	}
}

func TestAgentDialog_EnterIgnoresDisabledAgent(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "disabled-agent", Disabled: true},
	})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.IsVisible() {
		t.Fatal("dialog should remain visible when selecting disabled agent")
	}
	if cmd != nil {
		t.Error("enter on disabled agent should not produce a command")
	}
}

func TestAgentDialog_ToggleEnableDisable(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "custom-agent", Disabled: false, Native: false},
	})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("expected toggle command")
	}
	msg := cmd()
	toggle, ok := msg.(AgentToggleMsg)
	if !ok {
		t.Fatalf("expected AgentToggleMsg, got %T", msg)
	}
	if toggle.Agent != "custom-agent" {
		t.Errorf("expected Agent=%q, got %q", "custom-agent", toggle.Agent)
	}
	if toggle.Disabled != true {
		t.Error("expected Disabled=true (toggling from enabled to disabled)")
	}
}

func TestAgentDialog_ToggleDisabledToEnabled(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "off-agent", Disabled: true, Native: false},
	})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("expected toggle command")
	}
	msg := cmd()
	toggle := msg.(AgentToggleMsg)
	if toggle.Disabled != false {
		t.Error("expected Disabled=false (toggling from disabled to enabled)")
	}
}

func TestAgentDialog_ToggleIgnoredForNativeAgent(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "native-agent", Native: true},
	})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd != nil {
		t.Error("toggle should not produce command for native agent")
	}
}

func TestAgentDialog_EscapeCloses(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on esc")
	}
	if cmd != nil {
		t.Error("esc should not produce a command")
	}
}

func TestAgentDialog_QCloses(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on q")
	}
}

func TestAgentDialog_InvisibleIgnoresKeys(t *testing.T) {
	d := NewAgentDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invisible dialog should not emit a command")
	}
}

func TestAgentDialog_EmptyAgentsEscCloses(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on esc with empty agents")
	}
}

func TestAgentDialog_EmptyAgentsIgnoresNavigation(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("navigation with empty agents should not produce command")
	}
}

func TestAgentDialog_IgnoresNonKeyMessages(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "a"}})

	d, cmd := d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Error("non-key message should not produce a command")
	}
}

func TestAgentDialog_ViewEmptyWhenHidden(t *testing.T) {
	d := NewAgentDialog()
	if d.View() != "" {
		t.Error("expected empty view when hidden")
	}
}

func TestAgentDialog_ViewNonEmptyWhenVisible(t *testing.T) {
	d := NewAgentDialog()
	d.SetSize(80, 24)
	d.Show([]api.AgentInfo{{Name: "coder", Description: "Writes code"}})
	view := d.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}
