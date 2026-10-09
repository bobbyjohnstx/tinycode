package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMCPDialog_InitiallyHidden(t *testing.T) {
	d := NewMCPDialog()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden initially")
	}
}

func TestMCPDialog_ShowMakesVisible(t *testing.T) {
	d := NewMCPDialog()
	servers := []MCPServer{
		{Name: "server1", Status: "connected", ToolCount: 3},
	}
	d.Show(servers)
	if !d.IsVisible() {
		t.Fatal("expected dialog to be visible after Show")
	}
	if d.selected != 0 {
		t.Errorf("expected selected=0, got %d", d.selected)
	}
}

func TestMCPDialog_HideMakesInvisible(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "s1"}})
	d.Hide()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden after Hide")
	}
}

func TestMCPDialog_RefreshPreservesSelection(t *testing.T) {
	d := NewMCPDialog()
	servers := []MCPServer{
		{Name: "alpha", Status: "connected"},
		{Name: "beta", Status: "connected"},
		{Name: "gamma", Status: "error"},
	}
	d.Show(servers)
	d.selected = 2 // gamma

	// Refresh with same servers in different order
	newServers := []MCPServer{
		{Name: "beta", Status: "connected"},
		{Name: "gamma", Status: "reconnecting"},
		{Name: "alpha", Status: "connected"},
	}
	d.Refresh(newServers)
	if d.selected != 1 {
		t.Errorf("expected selected=1 (gamma in new list), got %d", d.selected)
	}
}

func TestMCPDialog_RefreshClampsWhenSelectedRemoved(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	d.selected = 2

	// Refresh with fewer servers, "c" removed
	d.Refresh([]MCPServer{{Name: "a"}})
	if d.selected != 0 {
		t.Errorf("expected selected clamped to 0, got %d", d.selected)
	}
}

func TestMCPDialog_NavigateDownWraps(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "a"}, {Name: "b"}})

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

func TestMCPDialog_NavigateUpWraps(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "a"}, {Name: "b"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.selected != 1 {
		t.Errorf("expected selected to wrap to 1, got %d", d.selected)
	}
}

func TestMCPDialog_ArrowKeys(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "a"}, {Name: "b"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != 1 {
		t.Errorf("expected selected=1 after down, got %d", d.selected)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.selected != 0 {
		t.Errorf("expected selected=0 after up, got %d", d.selected)
	}
}

func TestMCPDialog_ReconnectErrorServer(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{
		{Name: "healthy", Status: "connected"},
		{Name: "broken", Status: "error", Error: "connection refused"},
	})
	// Select the error server
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected reconnect command for error server")
	}
	msg := cmd()
	reconnect, ok := msg.(MCPReconnectRequestMsg)
	if !ok {
		t.Fatalf("expected MCPReconnectRequestMsg, got %T", msg)
	}
	if reconnect.Name != "broken" {
		t.Errorf("expected reconnect name=%q, got %q", "broken", reconnect.Name)
	}
}

func TestMCPDialog_ReconnectDisconnectedServer(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{
		{Name: "offline", Status: "disconnected"},
	})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected reconnect command for disconnected server")
	}
	msg := cmd()
	reconnect, ok := msg.(MCPReconnectRequestMsg)
	if !ok {
		t.Fatalf("expected MCPReconnectRequestMsg, got %T", msg)
	}
	if reconnect.Name != "offline" {
		t.Errorf("expected reconnect name=%q, got %q", "offline", reconnect.Name)
	}
}

func TestMCPDialog_ReconnectIgnoredForConnectedServer(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{
		{Name: "healthy", Status: "connected"},
	})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd != nil {
		t.Error("reconnect should not produce command for connected server")
	}
}

func TestMCPDialog_EscapeCloses(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "s1"}})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on esc")
	}
	if cmd != nil {
		t.Error("esc should not produce a command")
	}
}

func TestMCPDialog_QCloses(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "s1"}})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on q")
	}
}

func TestMCPDialog_InvisibleIgnoresKeys(t *testing.T) {
	d := NewMCPDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invisible dialog should not emit a command")
	}
}

func TestMCPDialog_EmptyServersEscCloses(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on esc with empty servers")
	}
}

func TestMCPDialog_EmptyServersIgnoresNavigation(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("navigation with empty servers should not produce command")
	}
}

func TestMCPDialog_IgnoresNonKeyMessages(t *testing.T) {
	d := NewMCPDialog()
	d.Show([]MCPServer{{Name: "s1"}})

	d, cmd := d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Error("non-key message should not produce a command")
	}
}

func TestMCPDialog_ViewEmptyWhenHidden(t *testing.T) {
	d := NewMCPDialog()
	if d.View() != "" {
		t.Error("expected empty view when hidden")
	}
}

func TestMCPDialog_ViewNonEmptyWhenVisible(t *testing.T) {
	d := NewMCPDialog()
	d.SetSize(80, 24)
	d.Show([]MCPServer{{Name: "s1", Status: "connected", ToolCount: 5}})
	view := d.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}
