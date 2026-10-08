package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// handleClientCommand
// ---------------------------------------------------------------------------

func TestHandleClientCommand_UnknownCommandReturnsNotHandled(t *testing.T) {
	app := NewApp("")
	cmd, handled := app.handleClientCommand("nonexistent-command")
	if handled {
		t.Fatal("expected handled=false for unknown command")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for unknown command")
	}
}

func TestHandleClientCommand_AllPaletteCommandsAreRecognized(t *testing.T) {
	app := NewApp("")
	// Give app an active session so session-dependent commands succeed.
	app.state.ActiveSession = "test-session"
	app.state.Sessions = []SessionInfo{{ID: "test-session", Title: "Test"}}

	for _, def := range clientCommandDefs {
		if !def.InPalette {
			continue
		}
		t.Run(def.Name, func(t *testing.T) {
			a := app // shallow copy is fine; value receiver semantics
			_, handled := a.handleClientCommand(def.Name)
			if !handled {
				t.Errorf("palette command %q was not recognized by handleClientCommand", def.Name)
			}
		})
	}
}

func TestHandleClientCommand_NonPaletteCommandsAreRecognized(t *testing.T) {
	nonPalette := []string{"exit", "undo", "redo", "privacy"}
	app := NewApp("")

	for _, name := range nonPalette {
		t.Run(name, func(t *testing.T) {
			a := app
			_, handled := a.handleClientCommand(name)
			if !handled {
				t.Errorf("non-palette command %q was not recognized", name)
			}
		})
	}
}

// Table-driven tests for individual command behaviors.

func TestHandleClientCommand_Exit(t *testing.T) {
	app := NewApp("")
	cmd, handled := app.handleClientCommand("exit")
	if !handled {
		t.Fatal("expected handled")
	}
	// tea.Quit is a specific function; running the cmd should produce a QuitMsg.
	if cmd == nil {
		t.Fatal("expected non-nil cmd for exit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestHandleClientCommand_CompactWithNoSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = ""
	cmd, handled := app.handleClientCommand("compact")
	if !handled {
		t.Fatal("expected handled")
	}
	// Should produce a toast (non-nil cmd), not a CompactRequestMsg.
	if cmd == nil {
		t.Fatal("expected toast cmd when no active session")
	}
}

func TestHandleClientCommand_CompactWithActiveSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "sess-1"
	cmd, handled := app.handleClientCommand("compact")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(CompactRequestMsg); !ok {
		t.Fatalf("expected CompactRequestMsg, got %T", msg)
	}
}

func TestHandleClientCommand_ThemeOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("theme")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.themeDlg.IsVisible() {
		t.Error("expected theme dialog to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_HelpOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("help")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.debugDlg.IsVisible() {
		t.Error("expected debug dialog (help) to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_DiagnosticsOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("diagnostics")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.debugDlg.IsVisible() {
		t.Error("expected debug dialog (diagnostics) to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_PrivacyOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("privacy")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.privacyDlg.IsVisible() {
		t.Error("expected privacy dialog to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_MCPOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("mcp")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.mcpDlg.IsVisible() {
		t.Error("expected MCP dialog to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_ContextOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("context")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.contextDlg.IsVisible() {
		t.Error("expected context dialog to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_HooksOpensDialog(t *testing.T) {
	app := NewApp("")
	_, handled := app.handleClientCommand("hooks")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.debugDlg.IsVisible() {
		t.Error("expected debug dialog (hooks) to be visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_RewindWithNoSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = ""
	cmd, handled := app.handleClientCommand("rewind")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected toast cmd when no session")
	}
}

func TestHandleClientCommand_RewindWithSessionButNoTurns(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "sess-1"
	// No messages in chat, so ExtractTurns returns empty.
	cmd, handled := app.handleClientCommand("rewind")
	if !handled {
		t.Fatal("expected handled")
	}
	// Should produce a toast about no turns.
	if cmd == nil {
		t.Fatal("expected toast cmd when no turns")
	}
}

func TestHandleClientCommand_ConnectSetsFlag(t *testing.T) {
	app := NewApp("")
	cmd, handled := app.handleClientCommand("connect")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.state.PendingModelDialog {
		t.Error("expected PendingModelDialog to be true")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd (ProvidersRefreshMsg)")
	}
	msg := cmd()
	if _, ok := msg.(ProvidersRefreshMsg); !ok {
		t.Fatalf("expected ProvidersRefreshMsg, got %T", msg)
	}
}

func TestHandleClientCommand_ScopedModelsSetsFlags(t *testing.T) {
	app := NewApp("")
	cmd, handled := app.handleClientCommand("scoped-models")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.state.PendingModelDialog {
		t.Error("expected PendingModelDialog to be true")
	}
	if !app.state.PendingScopingMode {
		t.Error("expected PendingScopingMode to be true")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

func TestHandleClientCommand_AutoApproveToggles(t *testing.T) {
	app := NewApp("")
	if app.state.AutoApprove {
		t.Fatal("expected auto-approve off initially")
	}

	_, handled := app.handleClientCommand("auto-approve")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.state.AutoApprove {
		t.Error("expected auto-approve to be enabled after first toggle")
	}

	_, _ = app.handleClientCommand("auto-approve")
	if app.state.AutoApprove {
		t.Error("expected auto-approve to be disabled after second toggle")
	}
}

func TestHandleClientCommand_ExportWithNoSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = ""
	cmd, handled := app.handleClientCommand("export")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected toast cmd when no session")
	}
}

func TestHandleClientCommand_ExportHTMLWithNoSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = ""
	cmd, handled := app.handleClientCommand("export-html")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected toast cmd when no session")
	}
}

func TestHandleClientCommand_ArchiveWithNoSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = ""
	cmd, handled := app.handleClientCommand("archive")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected toast cmd when no session")
	}
}

func TestHandleClientCommand_ArchiveWithActiveSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "sess-1"
	cmd, handled := app.handleClientCommand("archive")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(ArchiveRequestMsg); !ok {
		t.Fatalf("expected ArchiveRequestMsg, got %T", msg)
	}
}

func TestHandleClientCommand_BranchWithNoSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = ""
	cmd, handled := app.handleClientCommand("branch")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected toast cmd when no session")
	}
}

func TestHandleClientCommand_BranchWithActiveSession(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "sess-1"
	cmd, handled := app.handleClientCommand("branch")
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(BranchRequestMsg); !ok {
		t.Fatalf("expected BranchRequestMsg, got %T", msg)
	}
}

func TestHandleClientCommand_MsgProducingCommands(t *testing.T) {
	tests := []struct {
		name    string
		command string
		msgType string
	}{
		{"undo returns RevertRequestMsg", "undo", "RevertRequestMsg"},
		{"redo returns UnrevertRequestMsg", "redo", "UnrevertRequestMsg"},
		{"diff returns DiffRequestMsg", "diff", "DiffRequestMsg"},
		{"changes returns ChangesRequestMsg", "changes", "ChangesRequestMsg"},
		{"editor returns EditorRequestMsg", "editor", "EditorRequestMsg"},
		{"shell returns ShellSessionRequestMsg", "shell", "ShellSessionRequestMsg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewApp("")
			cmd, handled := a.handleClientCommand(tt.command)
			if !handled {
				t.Fatalf("command %q not handled", tt.command)
			}
			if cmd == nil {
				t.Fatalf("expected non-nil cmd for %q", tt.command)
			}
			msg := cmd()
			switch tt.command {
			case "undo":
				if _, ok := msg.(RevertRequestMsg); !ok {
					t.Fatalf("expected RevertRequestMsg, got %T", msg)
				}
			case "redo":
				if _, ok := msg.(UnrevertRequestMsg); !ok {
					t.Fatalf("expected UnrevertRequestMsg, got %T", msg)
				}
			case "diff":
				if _, ok := msg.(DiffRequestMsg); !ok {
					t.Fatalf("expected DiffRequestMsg, got %T", msg)
				}
			case "changes":
				if _, ok := msg.(ChangesRequestMsg); !ok {
					t.Fatalf("expected ChangesRequestMsg, got %T", msg)
				}
			case "editor":
				if _, ok := msg.(EditorRequestMsg); !ok {
					t.Fatalf("expected EditorRequestMsg, got %T", msg)
				}
			case "shell":
				if _, ok := msg.(ShellSessionRequestMsg); !ok {
					t.Fatalf("expected ShellSessionRequestMsg, got %T", msg)
				}
			}
		})
	}
}

func TestHandleClientCommand_ToastOnlyCommands(t *testing.T) {
	// Commands that display usage hints via toast.
	tests := []struct {
		name    string
		command string
	}{
		{"btw shows usage", "btw"},
		{"effort shows current level", "effort"},
		{"rename shows usage", "rename"},
		{"thinking shows current level", "thinking"},
		{"goal shows usage", "goal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewApp("")
			cmd, handled := a.handleClientCommand(tt.command)
			if !handled {
				t.Fatalf("command %q not handled", tt.command)
			}
			if cmd == nil {
				t.Fatalf("expected non-nil toast cmd for %q", tt.command)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// dispatchLeaderAction
// ---------------------------------------------------------------------------

func TestDispatchLeaderAction_SidebarToggles(t *testing.T) {
	app := NewApp("")
	if app.sidebar.IsOpen() {
		t.Fatal("expected sidebar closed initially")
	}

	app.dispatchLeaderAction(LeaderActionSidebar)
	if !app.sidebar.IsOpen() {
		t.Error("expected sidebar open after toggle")
	}
	if !app.state.SidebarOpen {
		t.Error("expected state.SidebarOpen=true")
	}

	app.dispatchLeaderAction(LeaderActionSidebar)
	if app.sidebar.IsOpen() {
		t.Error("expected sidebar closed after second toggle")
	}
}

func TestDispatchLeaderAction_AgentListOpensDialog(t *testing.T) {
	app := NewApp("")
	app.dispatchLeaderAction(LeaderActionAgentList)
	if !app.agentDlg.IsVisible() {
		t.Error("expected agent dialog visible")
	}
}

func TestDispatchLeaderAction_ModelListSetsFlag(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction(LeaderActionModelList)
	if !app.state.PendingModelDialog {
		t.Error("expected PendingModelDialog=true")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(ProvidersRefreshMsg); !ok {
		t.Fatalf("expected ProvidersRefreshMsg, got %T", msg)
	}
}

func TestDispatchLeaderAction_SessionListOpensDialog(t *testing.T) {
	app := NewApp("")
	app.dispatchLeaderAction(LeaderActionSessionList)
	if !app.dialog.IsVisible() {
		t.Error("expected session dialog visible")
	}
}

func TestDispatchLeaderAction_NewSession(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction(LeaderActionNewSession)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if m, ok := msg.(SessionSwitchedMsg); !ok {
		t.Fatalf("expected SessionSwitchedMsg, got %T", msg)
	} else if m.SessionID != "" {
		t.Errorf("expected empty session ID for new session, got %q", m.SessionID)
	}
}

func TestDispatchLeaderAction_ThemePickerOpensDialog(t *testing.T) {
	app := NewApp("")
	app.dispatchLeaderAction(LeaderActionThemePicker)
	if !app.themeDlg.IsVisible() {
		t.Error("expected theme dialog visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestDispatchLeaderAction_MCPListOpensDialog(t *testing.T) {
	app := NewApp("")
	app.dispatchLeaderAction(LeaderActionMCPList)
	if !app.mcpDlg.IsVisible() {
		t.Error("expected MCP dialog visible")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestDispatchLeaderAction_UndoReturnsRevert(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction(LeaderActionUndo)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(RevertRequestMsg); !ok {
		t.Fatalf("expected RevertRequestMsg, got %T", msg)
	}
}

func TestDispatchLeaderAction_RedoReturnsUnrevert(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction(LeaderActionRedo)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(UnrevertRequestMsg); !ok {
		t.Fatalf("expected UnrevertRequestMsg, got %T", msg)
	}
}

func TestDispatchLeaderAction_EditorReturnsEditorRequest(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction(LeaderActionEditor)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(EditorRequestMsg); !ok {
		t.Fatalf("expected EditorRequestMsg, got %T", msg)
	}
}

func TestDispatchLeaderAction_DiffViewReturnsDiffRequest(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction(LeaderActionDiffView)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(DiffRequestMsg); !ok {
		t.Fatalf("expected DiffRequestMsg, got %T", msg)
	}
}

func TestDispatchLeaderAction_UnknownReturnsNil(t *testing.T) {
	app := NewApp("")
	cmd := app.dispatchLeaderAction("totally-unknown-action")
	if cmd != nil {
		t.Fatal("expected nil cmd for unknown action")
	}
}

// ---------------------------------------------------------------------------
// handleGlobalKey
// ---------------------------------------------------------------------------

func TestHandleGlobalKey_CtrlPOpensPalette(t *testing.T) {
	app := NewApp("")
	app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	if !app.palette.IsVisible() {
		t.Error("expected command palette visible after ctrl+p")
	}
	if app.focus != FocusPalette {
		t.Errorf("expected FocusPalette, got %v", app.focus)
	}
}

func TestHandleGlobalKey_CtrlDQuitsApp(t *testing.T) {
	app := NewApp("")
	cmd := app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for ctrl+d")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestHandleGlobalKey_CtrlCWithEmptyPromptQuits(t *testing.T) {
	app := NewApp("")
	app.prompt.SetValue("")
	cmd := app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit cmd when prompt is empty")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestHandleGlobalKey_CtrlCWithTextClearsPrompt(t *testing.T) {
	app := NewApp("")
	app.prompt.SetValue("some text")
	cmd := app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected nil cmd (no quit) when prompt has text")
	}
	if app.prompt.Value() != "" {
		t.Errorf("expected prompt cleared, got %q", app.prompt.Value())
	}
}

func TestHandleGlobalKey_EscWithWorkingSessionSendsAbort(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "sess-1"
	app.state.SessionStatus["sess-1"] = SessionStatus{Working: true}

	cmd := app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for esc during working session")
	}
	msg := cmd()
	if _, ok := msg.(AbortRequestMsg); !ok {
		t.Fatalf("expected AbortRequestMsg, got %T", msg)
	}
}

func TestHandleGlobalKey_EscWithIdleSessionReturnsNil(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "sess-1"
	app.state.SessionStatus["sess-1"] = SessionStatus{Working: false}

	cmd := app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("expected nil cmd for esc with idle session and no toast")
	}
}

func TestHandleGlobalKey_UnhandledKeyReturnsNil(t *testing.T) {
	app := NewApp("")
	cmd := app.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Error("expected nil cmd for unhandled key")
	}
}
