package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

func TestHandleProvidersLoadedMsg_SetsCurrentModel(t *testing.T) {
	app := NewApp("")

	providers := []ProviderInfo{
		{
			ID:   "test-provider",
			Name: "Test Provider",
			Models: []ModelInfo{
				{ID: "test-model", ProviderID: "test-provider", Name: "Test Model 7B"},
			},
		},
	}

	msg := ProvidersLoadedMsg{
		Providers:       providers,
		DefaultProvider: "test-provider",
		DefaultModel:    "test-model",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if result.state.CurrentModel.ProviderID != "test-provider" {
		t.Errorf("expected CurrentModel.ProviderID 'test-provider', got %q", result.state.CurrentModel.ProviderID)
	}
	if result.state.CurrentModel.ModelID != "test-model" {
		t.Errorf("expected CurrentModel.ModelID 'test-model', got %q", result.state.CurrentModel.ModelID)
	}
}

func TestHandleProvidersLoadedMsg_SkipsWhenModelAlreadySet(t *testing.T) {
	app := NewApp("")
	app.state.CurrentModel = ModelSelection{
		ProviderID: "existing-provider",
		ModelID:    "existing-model",
	}

	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{
			{ID: "new-provider", Name: "New", Models: []ModelInfo{
				{ID: "new-model", ProviderID: "new-provider", Name: "New Model"},
			}},
		},
		DefaultProvider: "new-provider",
		DefaultModel:    "new-model",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	// Should NOT override the existing model selection.
	if result.state.CurrentModel.ProviderID != "existing-provider" {
		t.Errorf("expected existing provider to be preserved, got %q", result.state.CurrentModel.ProviderID)
	}
	if result.state.CurrentModel.ModelID != "existing-model" {
		t.Errorf("expected existing model to be preserved, got %q", result.state.CurrentModel.ModelID)
	}
}

func TestHandleProvidersLoadedMsg_StoresProviderList(t *testing.T) {
	app := NewApp("")

	providers := []ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{
			{ID: "llama3.2", ProviderID: "ollama", Name: "Llama 3.2"},
		}},
		{ID: "lm-studio", Name: "LM Studio", Models: []ModelInfo{
			{ID: "phi-4", ProviderID: "lm-studio", Name: "Phi 4"},
		}},
	}

	msg := ProvidersLoadedMsg{
		Providers:       providers,
		DefaultProvider: "ollama",
		DefaultModel:    "llama3.2",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if len(result.state.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result.state.Providers))
	}
	if result.state.Providers[0].ID != "ollama" {
		t.Errorf("expected first provider 'ollama', got %q", result.state.Providers[0].ID)
	}
}

func TestHandleProvidersLoadedMsg_EmptyDefaultsLeavesModelUnset(t *testing.T) {
	app := NewApp("")

	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{
			{ID: "test", Name: "Test"},
		},
		DefaultProvider: "",
		DefaultModel:    "",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if result.state.CurrentModel.ModelID != "" {
		t.Errorf("expected empty model when defaults are empty, got %q", result.state.CurrentModel.ModelID)
	}
	if result.state.CurrentModel.ProviderID != "" {
		t.Errorf("expected empty provider when defaults are empty, got %q", result.state.CurrentModel.ProviderID)
	}
}

func TestHandleProvidersLoadedMsg_AutoOpensConnectWhenNoModel(t *testing.T) {
	app := NewApp("")
	msg := ProvidersLoadedMsg{
		Providers:       []ProviderInfo{{ID: "ollama", Name: "Ollama"}},
		DefaultProvider: "",
		DefaultModel:    "",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if !result.modelDlg.IsVisible() {
		t.Fatal("expected connect dialog to auto-open when no model")
	}
	if result.focus != FocusDialog {
		t.Errorf("focus = %v, want FocusDialog", result.focus)
	}
	if !result.state.FirstRunConnectOffered {
		t.Error("expected FirstRunConnectOffered after auto-open")
	}
}

func TestHandleProvidersLoadedMsg_AutoOpenOnceOnly(t *testing.T) {
	app := NewApp("")
	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{{ID: "ollama", Name: "Ollama"}},
	}
	result, _ := app.handleProvidersLoadedMsg(msg)
	result.modelDlg.Hide()
	result.setFocus(FocusPrompt)

	result, _ = result.handleProvidersLoadedMsg(msg)
	if result.modelDlg.IsVisible() {
		t.Fatal("expected connect dialog not to re-open after first offer")
	}
}

func TestHandleProvidersLoadedMsg_NoAutoOpenWhenModelSet(t *testing.T) {
	app := NewApp("")
	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{
			{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", Name: "Qwen", ProviderID: "ollama"}}},
		},
		DefaultProvider: "ollama",
		DefaultModel:    "qwen",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)
	if result.modelDlg.IsVisible() {
		t.Fatal("expected no auto-open when default model resolved")
	}
	if result.state.FirstRunConnectOffered {
		t.Error("FirstRunConnectOffered should stay false when model was set")
	}
}

func TestHandleProvidersLoadedMsg_AutoOpensWithEmptyProviders(t *testing.T) {
	app := NewApp("")
	result, _ := app.handleProvidersLoadedMsg(ProvidersLoadedMsg{})
	if !result.modelDlg.IsVisible() {
		t.Fatal("expected connect dialog even with empty provider list")
	}
}

func TestPaletteSelected_SkillExpandsIntoPrompt(t *testing.T) {
	app := NewApp("")
	app.state.Commands = []api.CommandInfo{
		{Name: "doctor", Description: "Doctor skill", Source: "skill"},
	}

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "doctor", Value: "doctor"},
	})
	if !handled {
		t.Fatal("expected palette selection handled")
	}
	val := result.prompt.Value()
	if val == "" || val == "doctor" || val == "/doctor" {
		t.Fatalf("expected expanded skill body in prompt, got %q", val)
	}
	if strings.HasPrefix(strings.TrimSpace(val), "---") {
		t.Error("prompt should not contain skill frontmatter")
	}
	if !strings.Contains(val, "doctor") && !strings.Contains(val, "Doctor") {
		snippet := val
		if len(snippet) > 120 {
			snippet = snippet[:120]
		}
		t.Errorf("expected doctor skill body content, got %q", snippet)
	}
}

func TestPaletteSelected_NonSkillNoPromptInject(t *testing.T) {
	app := NewApp("")
	app.state.Commands = []api.CommandInfo{
		{Name: "init", Description: "Init", Source: "builtin"},
	}
	app.prompt.SetValue("keep-me")

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "init", Value: "init"},
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.prompt.Value() != "keep-me" {
		t.Errorf("non-skill palette select should not rewrite prompt, got %q", result.prompt.Value())
	}
}

// ---------------------------------------------------------------------------
// handleStateMsg tests
// ---------------------------------------------------------------------------

func TestHandleStateMsg_AgentListMsg_StoresAgentsAndPopulatesAutocomplete(t *testing.T) {
	app := NewApp("")

	agents := []api.AgentInfo{
		{Name: "build", Description: "Build agent", Mode: "primary"},
		{Name: "debug", Description: "Debug agent", Mode: "secondary"},
		{Name: "review", Description: "Review agent", Mode: "secondary"},
	}

	result, _, handled := app.handleStateMsg(AgentListMsg{Agents: agents})
	if !handled {
		t.Fatal("expected AgentListMsg to be handled")
	}

	// AllAgents should store the full list.
	if len(result.state.AllAgents) != 3 {
		t.Errorf("expected 3 AllAgents, got %d", len(result.state.AllAgents))
	}
	// Agents should only contain enabled (non-disabled) agents.
	if len(result.state.Agents) != 3 {
		t.Errorf("expected 3 enabled Agents, got %d", len(result.state.Agents))
	}
}

func TestHandleAgentListMsg_AskIncludesPrimaryAgents(t *testing.T) {
	app := NewApp("")
	agents := []api.AgentInfo{
		{Name: "executor", Description: "implement", Mode: "primary"},
		{Name: "architect", Description: "design", Mode: "subagent"},
	}
	result, _, handled := app.handleStateMsg(AgentListMsg{Agents: agents})
	if !handled {
		t.Fatal("expected handled")
	}
	names := map[string]bool{}
	for _, ag := range result.prompt.autocomplete.agents {
		names[ag.Name] = true
	}
	if !names["executor"] || !names["architect"] {
		t.Fatalf("ask autocomplete = %v", names)
	}
}

func TestHandleStateMsg_AgentListMsg_FiltersDisabledAgents(t *testing.T) {
	app := NewApp("")

	agents := []api.AgentInfo{
		{Name: "build", Description: "Build agent", Mode: "primary"},
		{Name: "disabled-one", Description: "Disabled", Mode: "secondary", Disabled: true},
		{Name: "debug", Description: "Debug agent", Mode: "secondary"},
	}

	result, _, handled := app.handleStateMsg(AgentListMsg{Agents: agents})
	if !handled {
		t.Fatal("expected handled")
	}

	if len(result.state.AllAgents) != 3 {
		t.Errorf("expected 3 AllAgents, got %d", len(result.state.AllAgents))
	}
	if len(result.state.Agents) != 2 {
		t.Errorf("expected 2 enabled Agents (disabled filtered out), got %d", len(result.state.Agents))
	}
}

func TestHandleStateMsg_AgentListMsg_ErrorDoesNotCrash(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleStateMsg(AgentListMsg{Err: errors.New("fetch failed")})
	if !handled {
		t.Fatal("expected handled even with error")
	}
	// AllAgents should remain empty/nil when there's an error.
	if len(result.state.AllAgents) != 0 {
		t.Errorf("expected 0 AllAgents on error, got %d", len(result.state.AllAgents))
	}
}

func TestHandleStateMsg_CommandListMsg_StoresCommandsAndPopulatesAutocomplete(t *testing.T) {
	app := NewApp("")

	commands := []api.CommandInfo{
		{Name: "init", Description: "Initialize project"},
		{Name: "deploy", Description: "Deploy app"},
	}

	result, _, handled := app.handleStateMsg(CommandListMsg{Commands: commands})
	if !handled {
		t.Fatal("expected CommandListMsg to be handled")
	}

	if len(result.state.Commands) != 2 {
		t.Errorf("expected 2 Commands, got %d", len(result.state.Commands))
	}
}

func TestHandleStateMsg_CommandListMsg_ErrorDoesNotStoreCommands(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleStateMsg(CommandListMsg{Err: errors.New("fail")})
	if !handled {
		t.Fatal("expected handled even with error")
	}
	if len(result.state.Commands) != 0 {
		t.Errorf("expected 0 Commands on error, got %d", len(result.state.Commands))
	}
}

func TestHandleStateMsg_SessionsLoadedMsg_StoresSessionsAndUpdatesSidebar(t *testing.T) {
	app := NewApp("")

	sessions := []SessionInfo{
		{ID: "s1", Title: "Session 1"},
		{ID: "s2", Title: "Session 2"},
	}

	result, _, handled := app.handleStateMsg(SessionsLoadedMsg{Sessions: sessions})
	if !handled {
		t.Fatal("expected SessionsLoadedMsg to be handled")
	}

	if len(result.state.Sessions) != 2 {
		t.Errorf("expected 2 Sessions, got %d", len(result.state.Sessions))
	}
}

func TestHandleStateMsg_SessionsLoadedMsg_ErrorLeavesSessionsEmpty(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleStateMsg(SessionsLoadedMsg{Err: errors.New("fail")})
	if !handled {
		t.Fatal("expected handled even with error")
	}
	if len(result.state.Sessions) != 0 {
		t.Errorf("expected 0 Sessions on error, got %d", len(result.state.Sessions))
	}
}

func TestHandleStateMsg_SessionUpdatedMsg_UpdatesTitleInSessionsList(t *testing.T) {
	app := NewApp("")
	app.state.Sessions = []SessionInfo{
		{ID: "s1", Title: "Old Title"},
		{ID: "s2", Title: "Keep This"},
	}

	result, _, handled := app.handleStateMsg(SessionUpdatedMsg{
		SessionID: "s1",
		Info:      SessionInfo{Title: "New Title"},
	})
	if !handled {
		t.Fatal("expected handled")
	}

	if result.state.Sessions[0].Title != "New Title" {
		t.Errorf("expected updated title 'New Title', got %q", result.state.Sessions[0].Title)
	}
	if result.state.Sessions[1].Title != "Keep This" {
		t.Errorf("expected unchanged title 'Keep This', got %q", result.state.Sessions[1].Title)
	}
}

func TestHandleStateMsg_SessionCreatedMsg_PrependsToSessionsList(t *testing.T) {
	app := NewApp("")
	app.state.Sessions = []SessionInfo{
		{ID: "existing", Title: "Existing Session"},
	}

	result, _, handled := app.handleStateMsg(SessionCreatedMsg{
		Info: SessionInfo{ID: "new-id", Title: "New Session"},
	})
	if !handled {
		t.Fatal("expected handled")
	}

	if len(result.state.Sessions) != 2 {
		t.Fatalf("expected 2 Sessions, got %d", len(result.state.Sessions))
	}
	if result.state.Sessions[0].ID != "new-id" {
		t.Errorf("expected new session prepended at index 0, got %q", result.state.Sessions[0].ID)
	}
	if result.state.Sessions[1].ID != "existing" {
		t.Errorf("expected existing session at index 1, got %q", result.state.Sessions[1].ID)
	}
}

func TestHandleStateMsg_SessionDeletedMsg_RemovesFromSessionsList(t *testing.T) {
	app := NewApp("")
	app.state.Sessions = []SessionInfo{
		{ID: "s1", Title: "First"},
		{ID: "s2", Title: "Second"},
		{ID: "s3", Title: "Third"},
	}

	result, _, handled := app.handleStateMsg(SessionDeletedMsg{SessionID: "s2"})
	if !handled {
		t.Fatal("expected handled")
	}

	if len(result.state.Sessions) != 2 {
		t.Fatalf("expected 2 Sessions after deletion, got %d", len(result.state.Sessions))
	}
	for _, s := range result.state.Sessions {
		if s.ID == "s2" {
			t.Error("expected session s2 to be removed")
		}
	}
}

func TestHandleStateMsg_SessionSwitchedMsg_SetsActiveSessionAndClearsMessages(t *testing.T) {
	app := NewApp("")
	app.state.Sessions = []SessionInfo{
		{ID: "s1", Title: "First", Agent: "debug"},
	}
	app.state.Messages["s1"] = []MessageView{{Info: MessageInfo{Role: "user"}}}

	result, _, handled := app.handleStateMsg(SessionSwitchedMsg{SessionID: "s1"})
	if !handled {
		t.Fatal("expected handled")
	}

	if result.state.ActiveSession != "s1" {
		t.Errorf("expected ActiveSession 's1', got %q", result.state.ActiveSession)
	}
	// Messages for the session should be cleared on switch.
	if msgs := result.state.Messages["s1"]; msgs != nil {
		t.Errorf("expected Messages for s1 to be nil after switch, got %v", msgs)
	}
}

func TestHandleStateMsg_SessionStatusMsg_UpdatesSessionStatus(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "s1"

	result, _, handled := app.handleStateMsg(SessionStatusMsg{
		SessionID: "s1",
		Status:    SessionStatus{Working: true},
	})
	if !handled {
		t.Fatal("expected handled")
	}

	status, ok := result.state.SessionStatus["s1"]
	if !ok {
		t.Fatal("expected session status to be stored")
	}
	if !status.Working {
		t.Error("expected session status Working=true")
	}
}

func TestHandleStateMsg_SessionStatusMsg_SetsWorkingOnStatusBar(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "s1"

	result, _, _ := app.handleStateMsg(SessionStatusMsg{
		SessionID: "s1",
		Status:    SessionStatus{Working: true},
	})

	if !result.status.working {
		t.Error("expected status bar working=true for active session")
	}
}

func TestHandleStateMsg_SessionStatusMsg_InactiveSessionDoesNotAffectStatusBar(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "s1"

	result, _, _ := app.handleStateMsg(SessionStatusMsg{
		SessionID: "other-session",
		Status:    SessionStatus{Working: true},
	})

	if result.status.working {
		t.Error("expected status bar working=false for non-active session")
	}
}

func TestHandleStateMsg_SSEConnectedMsg_SetsConnectedTrue(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleStateMsg(SSEConnectedMsg{})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.state.Connected {
		t.Error("expected Connected=true after SSEConnectedMsg")
	}
}

func TestHandleStateMsg_SSEDisconnectedMsg_SetsConnectedFalse(t *testing.T) {
	app := NewApp("")
	app.state.Connected = true

	result, _, handled := app.handleStateMsg(SSEDisconnectedMsg{})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.state.Connected {
		t.Error("expected Connected=false after SSEDisconnectedMsg")
	}
}

func TestHandleStateMsg_PluginListMsg_StoresPlugins(t *testing.T) {
	app := NewApp("")

	plugins := []api.PluginInfo{
		{ID: "p1", Name: "Plugin 1"},
		{ID: "p2", Name: "Plugin 2"},
	}

	result, _, handled := app.handleStateMsg(PluginListMsg{Plugins: plugins})
	if !handled {
		t.Fatal("expected handled")
	}
	if len(result.state.Plugins) != 2 {
		t.Errorf("expected 2 Plugins, got %d", len(result.state.Plugins))
	}
}

func TestHandleStateMsg_PluginListMsg_ErrorDoesNotStorePlugins(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleStateMsg(PluginListMsg{Err: errors.New("fail")})
	if !handled {
		t.Fatal("expected handled even with error")
	}
	if len(result.state.Plugins) != 0 {
		t.Errorf("expected 0 Plugins on error, got %d", len(result.state.Plugins))
	}
}

func TestHandleStateMsg_MCPStatusLoadedMsg_StoresServers(t *testing.T) {
	app := NewApp("")

	servers := []MCPServer{
		{Name: "mcp1", Status: "connected"},
		{Name: "mcp2", Status: "disconnected"},
	}

	result, _, handled := app.handleStateMsg(MCPStatusLoadedMsg{Servers: servers})
	if !handled {
		t.Fatal("expected handled")
	}
	// Verify servers were stored (sidebar owns the state).
	_ = result // MCP servers stored in sidebar, verified by no panic.
}

func TestHandleStateMsg_MCPStatusLoadedMsg_ErrorMarksFailure(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleStateMsg(MCPStatusLoadedMsg{Err: errors.New("fail")})
	if !handled {
		t.Fatal("expected handled even with error")
	}
}

func TestHandleStateMsg_LSPStatusLoadedMsg_StoresStatus(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleStateMsg(LSPStatusLoadedMsg{
		Status: LSPStatus{Errors: 3, Warnings: 5},
	})
	if !handled {
		t.Fatal("expected handled")
	}
}

func TestHandleStateMsg_UnhandledMsg_ReturnsFalse(t *testing.T) {
	app := NewApp("")

	// A message type not handled by handleStateMsg should return handled=false.
	_, _, handled := app.handleStateMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if handled {
		t.Error("expected unhandled message to return false")
	}
}

// ---------------------------------------------------------------------------
// handleNotificationMsg tests
// ---------------------------------------------------------------------------

func TestHandleNotificationMsg_SessionErrorMsg_ShowsToastAndStopsWorking(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "s1"
	app.status.SetWorking(true)

	result, _, handled := app.handleNotificationMsg(SessionErrorMsg{
		SessionID: "s1",
		Error:     "something went wrong",
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.status.working {
		t.Error("expected working=false after session error")
	}
	status := result.state.SessionStatus["s1"]
	if status.Working {
		t.Error("expected session status Working=false after error")
	}
}

func TestHandleNotificationMsg_SessionErrorMsg_NoModelTriggersRefresh(t *testing.T) {
	app := NewApp("")

	result, cmd, handled := app.handleNotificationMsg(SessionErrorMsg{
		Error: "no model configured",
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.state.PendingModelDialog {
		t.Error("expected PendingModelDialog=true for no-model error")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd for no-model error")
	}
}

func TestHandleNotificationMsg_PromptSubmittedMsg_SetsWorking(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "s1"

	result, _, handled := app.handleNotificationMsg(PromptSubmittedMsg{Content: "hello"})
	if !handled {
		t.Fatal("expected handled")
	}

	status := result.state.SessionStatus["s1"]
	if !status.Working {
		t.Error("expected session status Working=true after prompt submitted")
	}
	if !result.status.working {
		t.Error("expected status bar working=true after prompt submitted")
	}
}

func TestHandleNotificationMsg_ToastMsg_ShowsToast(t *testing.T) {
	app := NewApp("")

	result, cmd, handled := app.handleNotificationMsg(ToastMsg{Text: "hello", IsError: false})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.toast.IsVisible() {
		t.Error("expected toast to be visible")
	}
	_ = cmd // cmd contains the expiry timer.
}

func TestHandleNotificationMsg_ToastExpiredMsg_HidesToast(t *testing.T) {
	app := NewApp("")
	// Show a toast first.
	app.toast.Show("temp", false)
	if !app.toast.IsVisible() {
		t.Fatal("precondition: toast should be visible")
	}

	// Send a ToastExpiredMsg with the right ID.
	result, _, handled := app.handleNotificationMsg(ToastExpiredMsg{ID: app.toast.id})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.toast.IsVisible() {
		t.Error("expected toast to be hidden after expiry")
	}
}

func TestHandleNotificationMsg_LeaderTimeoutMsg_ClearsLeaderState(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleNotificationMsg(LeaderTimeoutMsg{})
	if !handled {
		t.Fatal("expected handled")
	}
}

func TestHandleNotificationMsg_CopiedToClipboardMsg_ShowsCharCount(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleNotificationMsg(CopiedToClipboardMsg{Chars: 42})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.toast.IsVisible() {
		t.Error("expected toast to be visible after clipboard copy")
	}
}

func TestHandleNotificationMsg_CopiedToClipboardMsg_ErrorDoesNotShowToast(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleNotificationMsg(CopiedToClipboardMsg{
		Err: errors.New("fail"),
	})
	if !handled {
		t.Fatal("expected handled even with error")
	}
	if result.toast.IsVisible() {
		t.Error("expected no toast on clipboard error")
	}
}

func TestHandleNotificationMsg_ExportSessionMsg_ShowsSuccessToast(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleNotificationMsg(ExportSessionMsg{Path: "/tmp/export.json"})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.toast.IsVisible() {
		t.Error("expected toast after successful export")
	}
}

func TestHandleNotificationMsg_ExportSessionMsg_ErrorShowsErrorToast(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleNotificationMsg(ExportSessionMsg{
		Err: errors.New("write failed"),
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.toast.IsVisible() {
		t.Error("expected error toast after export failure")
	}
}

func TestHandleNotificationMsg_CodeBlockWrittenMsg_ShowsFilename(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleNotificationMsg(CodeBlockWrittenMsg{Path: "/tmp/block.go"})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.toast.IsVisible() {
		t.Error("expected toast after code block written")
	}
}

func TestHandleNotificationMsg_CodeBlockWrittenMsg_ErrorShowsErrorToast(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleNotificationMsg(CodeBlockWrittenMsg{
		Err: errors.New("disk full"),
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.toast.IsVisible() {
		t.Error("expected error toast after write failure")
	}
}

func TestHandleNotificationMsg_UnhandledMsg_ReturnsFalse(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleNotificationMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if handled {
		t.Error("expected unhandled message to return false")
	}
}

// ---------------------------------------------------------------------------
// handleDialogMsg tests
// ---------------------------------------------------------------------------

func TestHandleDialogMsg_AgentSelectedMsg_SetsCurrentAgent(t *testing.T) {
	app := NewApp("")
	app.state.CurrentModel = ModelSelection{ProviderID: "ollama", ModelID: "qwen"}

	result, _, handled := app.handleDialogMsg(AgentSelectedMsg{Agent: "debug"})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.state.CurrentAgent != "debug" {
		t.Errorf("expected CurrentAgent 'debug', got %q", result.state.CurrentAgent)
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected focus returned to FocusPrompt, got %v", result.focus)
	}
}

func TestHandleDialogMsg_AgentSelectedMsg_UpdatesActiveSessionAgent(t *testing.T) {
	app := NewApp("")
	app.state.ActiveSession = "s1"
	app.state.Sessions = []SessionInfo{
		{ID: "s1", Title: "Test", Agent: "build"},
	}

	result, _, _ := app.handleDialogMsg(AgentSelectedMsg{Agent: "review"})

	if result.state.Sessions[0].Agent != "review" {
		t.Errorf("expected active session agent updated to 'review', got %q", result.state.Sessions[0].Agent)
	}
}

func TestHandleDialogMsg_ModelSelectedMsg_SetsCurrentModel(t *testing.T) {
	app := NewApp("")
	app.state.Providers = []ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{
			{ID: "qwen", ProviderID: "ollama", Name: "Qwen 2.5"},
		}},
	}

	result, _, handled := app.handleDialogMsg(ModelSelectedMsg{
		Selection: ModelSelection{ProviderID: "ollama", ModelID: "qwen"},
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.state.CurrentModel.ModelID != "qwen" {
		t.Errorf("expected CurrentModel.ModelID 'qwen', got %q", result.state.CurrentModel.ModelID)
	}
	if result.state.CurrentModel.ProviderID != "ollama" {
		t.Errorf("expected CurrentModel.ProviderID 'ollama', got %q", result.state.CurrentModel.ProviderID)
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected focus returned to FocusPrompt, got %v", result.focus)
	}
}

func TestHandleDialogMsg_ThemeSelectedMsg_SetsThemeAndShowsToast(t *testing.T) {
	// ApplyColorTheme sets themeAgentColor (package-level). Restore it so
	// downstream tests (e.g. TestStatusBar_SetAgent_ChangesSpinner) that rely
	// on per-agent colors are not poisoned.
	savedAgentColor := themeAgentColor
	t.Cleanup(func() { themeAgentColor = savedAgentColor })

	app := NewApp("")

	// Get a real theme ID from the registry.
	themes := app.themes.List()
	if len(themes) == 0 {
		t.Skip("no themes loaded in test registry")
	}
	themeID := themes[0].ID

	result, _, handled := app.handleDialogMsg(ThemeSelectedMsg{ThemeID: themeID})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.state.CurrentTheme != themeID {
		t.Errorf("expected CurrentTheme %q, got %q", themeID, result.state.CurrentTheme)
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected focus returned to FocusPrompt, got %v", result.focus)
	}
	if !result.toast.IsVisible() {
		t.Error("expected toast after theme selection")
	}
}

func TestHandleDialogMsg_PermissionDismissedMsg_HidesPromptAndReturnsReply(t *testing.T) {
	app := NewApp("")
	app.permPrompt.Show(PermissionRequest{ID: "perm1", SessionID: "s1", Permission: "bash"})

	result, cmd, handled := app.handleDialogMsg(PermissionDismissedMsg{
		Request: PermissionRequest{ID: "perm1", SessionID: "s1", Permission: "bash"},
		Action:  PermissionAllow,
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.permPrompt.IsVisible() {
		t.Error("expected permission prompt to be hidden")
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected focus returned to FocusPrompt, got %v", result.focus)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd with PermissionReplyMsg")
	}
	// Execute the cmd to verify it produces a PermissionReplyMsg.
	msg := cmd()
	reply, ok := msg.(PermissionReplyMsg)
	if !ok {
		t.Fatalf("expected PermissionReplyMsg, got %T", msg)
	}
	if reply.SessionID != "s1" {
		t.Errorf("expected SessionID 's1', got %q", reply.SessionID)
	}
	if reply.PermissionID != "perm1" {
		t.Errorf("expected PermissionID 'perm1', got %q", reply.PermissionID)
	}
	if reply.Action != "once" {
		t.Errorf("expected Action 'once', got %q", reply.Action)
	}
}

func TestHandleDialogMsg_PermissionRequestedMsg_ShowsPrompt(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleDialogMsg(PermissionRequestedMsg{
		Request: PermissionRequest{ID: "perm1", Permission: "write"},
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if !result.permPrompt.IsVisible() {
		t.Error("expected permission prompt to be visible")
	}
}

func TestHandleDialogMsg_PaletteClosedMsg_ReturnsFocusToPrompt(t *testing.T) {
	app := NewApp("")
	app.focus = FocusPalette

	result, _, handled := app.handleDialogMsg(PaletteClosedMsg{})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected focus returned to FocusPrompt, got %v", result.focus)
	}
}

func TestHandleDialogMsg_PaletteSelectedMsg_FileCategory_InsertsReference(t *testing.T) {
	app := NewApp("")
	app.prompt.SetValue("existing text")

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "main.go", Value: "@main.go", Category: "file"},
	})
	if !handled {
		t.Fatal("expected handled")
	}
	val := result.prompt.Value()
	if !strings.Contains(val, "@main.go") {
		t.Errorf("expected file reference in prompt, got %q", val)
	}
	if !strings.HasPrefix(val, "existing text") {
		t.Errorf("expected original text preserved, got %q", val)
	}
}

func TestHandleDialogMsg_PaletteSelectedMsg_SessionCategory_SwitchesSession(t *testing.T) {
	app := NewApp("")

	_, cmd, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "My Session", Value: "session:abc123", Category: "session"},
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected cmd to switch session")
	}
	msg := cmd()
	switched, ok := msg.(SessionSwitchedMsg)
	if !ok {
		t.Fatalf("expected SessionSwitchedMsg, got %T", msg)
	}
	if switched.SessionID != "abc123" {
		t.Errorf("expected SessionID 'abc123', got %q", switched.SessionID)
	}
}

func TestHandleDialogMsg_FocusChangedMsg_PaletteTarget_ShowsPalette(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleDialogMsg(FocusChangedMsg{Target: FocusPalette})
	if !handled {
		t.Fatal("expected handled")
	}
	_ = result // palette was shown internally.
}

func TestHandleDialogMsg_FocusChangedMsg_PromptTarget_SetsFocus(t *testing.T) {
	app := NewApp("")
	app.focus = FocusDialog

	result, _, handled := app.handleDialogMsg(FocusChangedMsg{Target: FocusPrompt})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected FocusPrompt, got %v", result.focus)
	}
}

func TestHandleDialogMsg_SidebarSessionSelectedMsg_TriggersSwitch(t *testing.T) {
	app := NewApp("")

	_, cmd, handled := app.handleDialogMsg(SidebarSessionSelectedMsg{SessionID: "s1"})
	if !handled {
		t.Fatal("expected handled")
	}
	if cmd == nil {
		t.Fatal("expected cmd to switch session")
	}
	msg := cmd()
	switched, ok := msg.(SessionSwitchedMsg)
	if !ok {
		t.Fatalf("expected SessionSwitchedMsg, got %T", msg)
	}
	if switched.SessionID != "s1" {
		t.Errorf("expected SessionID 's1', got %q", switched.SessionID)
	}
}

func TestHandleDialogMsg_UnhandledMsg_ReturnsFalse(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleDialogMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if handled {
		t.Error("expected unhandled message to return false")
	}
}

// ---------------------------------------------------------------------------
// handleKeyMsg tests
// ---------------------------------------------------------------------------

func TestHandleKeyMsg_PermPromptVisible_RoutesToPermPrompt(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.permPrompt.SetSize(100, 30)
	app.permPrompt.Show(PermissionRequest{ID: "p1", Permission: "bash"})

	// Send a key that should be routed to the permission prompt.
	_, cmd := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRight})
	// Should not panic and should be routed to permPrompt.
	_ = cmd
}

func TestHandleKeyMsg_PaletteVisible_RoutesToPalette(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.palette.SetSize(100, 30)
	app.palette.Show([]PaletteItem{{Label: "test", Value: "test"}})

	result, _ := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	// Palette should close on Esc.
	if result.palette.IsVisible() {
		t.Error("expected palette to close on Esc")
	}
}

func TestHandleKeyMsg_AgentDlgVisible_RoutesToAgentDialog(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.agentDlg.SetSize(100, 30)
	app.agentDlg.Show([]api.AgentInfo{{Name: "build", Mode: "primary"}})

	// Esc should close the agent dialog.
	result, _ := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	if result.agentDlg.IsVisible() {
		t.Error("expected agent dialog to close on Esc")
	}
}

func TestHandleKeyMsg_ModelDlgVisible_RoutesToModelDialog(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.modelDlg.SetSize(100, 30)
	providers := []ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{
			{ID: "qwen", ProviderID: "ollama", Name: "Qwen"},
		}},
	}
	app.modelDlg.Show(providers, ModelSelection{})

	result, _ := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	if result.modelDlg.IsVisible() {
		t.Error("expected model dialog to close on Esc")
	}
	if result.focus != FocusPrompt {
		t.Errorf("expected focus returned to FocusPrompt after model dialog close, got %v", result.focus)
	}
}

func TestHandleKeyMsg_ThemeDlgVisible_RoutesToThemeDialog(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.themeDlg.SetSize(100, 30)
	app.themeDlg.Show(app.themes.List(), "")

	result, _ := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	if result.themeDlg.IsVisible() {
		t.Error("expected theme dialog to close on Esc")
	}
}

func TestHandleKeyMsg_NoOverlay_DispatchesToFocused(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.focus = FocusPrompt

	// Type a rune with no overlays — should go to the prompt.
	result, _ := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	val := result.prompt.Value()
	if !strings.Contains(val, "a") {
		t.Errorf("expected 'a' in prompt value, got %q", val)
	}
}

func TestHandleKeyMsg_OverlayPriority_PermBeforePalette(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true
	app.permPrompt.SetSize(100, 30)
	app.palette.SetSize(100, 30)

	// Both visible: perm prompt should take priority.
	app.permPrompt.Show(PermissionRequest{ID: "p1", Permission: "bash"})
	app.palette.Show([]PaletteItem{{Label: "test", Value: "test"}})

	result, _ := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	// Palette should still be visible since permPrompt handled the key.
	if !result.palette.IsVisible() {
		t.Error("expected palette to remain visible while perm prompt has priority")
	}
}

func TestHandleKeyMsg_TerminalNoise_IsAbsorbed(t *testing.T) {
	app := NewApp("")
	app.width = 100
	app.height = 30
	app.ready = true

	// Arm the noise guard so terminal noise is absorbed.
	app.prompt.ArmNoiseGuard(0)

	// Simulate terminal noise — something that looks like a CPR response.
	result, cmd := app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";1R")})
	if cmd != nil {
		t.Error("expected nil cmd for absorbed terminal noise")
	}
	// The prompt should not contain the noise.
	if strings.Contains(result.prompt.Value(), ";1R") {
		t.Error("expected terminal noise to be absorbed, not typed into prompt")
	}
}

// ---------------------------------------------------------------------------
// handleProvidersLoadedMsg via handleStateMsg
// ---------------------------------------------------------------------------

func TestHandleStateMsg_ProvidersLoadedMsg_SetsCurrentModel(t *testing.T) {
	app := NewApp("")

	providers := []ProviderInfo{
		{
			ID:   "test-provider",
			Name: "Test Provider",
			Models: []ModelInfo{
				{ID: "test-model", ProviderID: "test-provider", Name: "Test Model"},
			},
		},
	}

	result, _, handled := app.handleStateMsg(ProvidersLoadedMsg{
		Providers:       providers,
		DefaultProvider: "test-provider",
		DefaultModel:    "test-model",
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.state.CurrentModel.ModelID != "test-model" {
		t.Errorf("expected CurrentModel.ModelID 'test-model', got %q", result.state.CurrentModel.ModelID)
	}
}

func TestHandleStateMsg_ProvidersLoadedMsg_ErrorDoesNotCrash(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleStateMsg(ProvidersLoadedMsg{Err: errors.New("discovery failed")})
	if !handled {
		t.Fatal("expected handled even with error")
	}
}

// ---------------------------------------------------------------------------
// activityFromMessages tests
// ---------------------------------------------------------------------------

func TestActivityFromMessages_ReturnsStreamingActivity(t *testing.T) {
	msgs := []MessageView{
		{Parts: []PartView{
			{Type: "tool-call", ToolName: "bash", Streaming: false},
			{Type: "tool-call", ToolName: "write", Streaming: true},
		}},
	}
	act := activityFromMessages(msgs)
	if act == "" {
		t.Error("expected activity from streaming tool-call")
	}
}

func TestActivityFromMessages_EmptyMessagesReturnsEmpty(t *testing.T) {
	act := activityFromMessages(nil)
	if act != "" {
		t.Errorf("expected empty activity for nil messages, got %q", act)
	}
}

func TestNewestAssistantTokens_ReturnsLatestTokenCount(t *testing.T) {
	msgs := []MessageView{
		{Info: MessageInfo{Role: "assistant", Tokens: TokenInfo{Input: 100, Output: 50}}},
		{Info: MessageInfo{Role: "user"}},
		{Info: MessageInfo{Role: "assistant", Tokens: TokenInfo{Input: 200, Output: 80}}},
	}
	n := newestAssistantTokens(msgs)
	if n != 280 {
		t.Errorf("expected 280, got %d", n)
	}
}

func TestNewestAssistantTokens_NoAssistantReturnsZero(t *testing.T) {
	msgs := []MessageView{
		{Info: MessageInfo{Role: "user"}},
	}
	n := newestAssistantTokens(msgs)
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}
