package tui

import (
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildDebugInfo(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.sidebar.SetVersion("v1.2.3")
	app.state.CurrentModel.ProviderID = "openai"
	app.state.CurrentModel.ModelID = "gpt-4"
	app.state.CurrentAgent = "coder"

	info := app.buildDebugInfo()

	expected := []string{
		"Version:",
		"v1.2.3",
		"Go:",
		runtime.Version(),
		"OS/Arch:",
		runtime.GOOS + "/" + runtime.GOARCH,
		"Terminal:",
		"Model:",
		"Agent:",
		"coder",
		"Config:",
		"Data dir:",
		"Agents:",
		"Plugins:",
		"MCP:",
	}

	for _, s := range expected {
		if !strings.Contains(info, s) {
			t.Errorf("buildDebugInfo() missing %q\nGot:\n%s", s, info)
		}
	}

	if info == "" {
		t.Error("buildDebugInfo() returned empty string")
	}
}

func TestBuildDebugInfoDefaultAgent(t *testing.T) {
	app := NewApp("http://localhost:4096")
	info := app.buildDebugInfo()

	if !strings.Contains(info, "(default)") {
		t.Errorf("expected (default) agent when CurrentAgent is empty\nGot:\n%s", info)
	}
}

func TestBuildDebugInfoMCPServers(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.sidebar.mcpServers = []MCPServer{
		{Name: "srv1", Status: "connected"},
		{Name: "srv2", Status: "error"},
	}

	info := app.buildDebugInfo()

	if !strings.Contains(info, "2 servers") {
		t.Errorf("expected MCP count in output\nGot:\n%s", info)
	}
	if !strings.Contains(info, "1 connected") {
		t.Errorf("expected connected count in output\nGot:\n%s", info)
	}
}

func TestDebugDialogUpdate(t *testing.T) {
	d := NewDebugDialog()
	d.Show("Help", "test info")

	if !d.IsVisible() {
		t.Fatal("expected dialog to be visible after Show")
	}

	// Pressing esc should close.
	d2, _ := d.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if d2.IsVisible() {
		t.Error("expected dialog to close on esc")
	}
}

func TestDebugDialogCopy(t *testing.T) {
	d := NewDebugDialog()
	d.Show("Diagnostics", "test info")

	d2, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if d2.IsVisible() {
		t.Error("expected dialog to close on copy")
	}
	if cmd == nil {
		t.Error("expected a clipboard command on copy")
	}
}

func TestDebugDialogView_HelpTitle(t *testing.T) {
	d := NewDebugDialog()
	d.SetSize(80, 24)
	d.Show("Help", "# tinycode Commands")

	view := d.View()
	if !strings.Contains(view, "Help") {
		t.Errorf("expected Help title in view, got:\n%s", view)
	}
	if strings.Contains(view, "Diagnostics") {
		t.Errorf("help dialog should not show Diagnostics title, got:\n%s", view)
	}
}

func TestDebugDialogView_DiagnosticsTitle(t *testing.T) {
	d := NewDebugDialog()
	d.SetSize(80, 24)
	d.Show("Diagnostics", "Version: 1.0")

	view := d.View()
	if !strings.Contains(view, "Diagnostics") {
		t.Errorf("expected Diagnostics title in view, got:\n%s", view)
	}
}

func TestHelpCommands_SessionLeaderKeyIsO(t *testing.T) {
	if !strings.Contains(helpCommandsText, "`o` sessions") {
		t.Error("help-commands.md should list ctrl+x follow-up `o` for sessions")
	}
	if strings.Contains(helpCommandsText, "`s` sessions") {
		t.Error("help-commands.md should not list obsolete `s` sessions binding")
	}
}
