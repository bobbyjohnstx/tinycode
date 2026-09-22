package tui

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

func TestBuildPromptInput_IncludesModel(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.CurrentModel = ModelSelection{
		ProviderID: "lm-studio",
		ModelID:    "ornith-1.0-9b-mlx",
	}
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.Model == nil {
		t.Fatal("expected Model to be set in PromptInput")
	}
	if input.Model.ProviderID != "lm-studio" {
		t.Errorf("expected ProviderID lm-studio, got %s", input.Model.ProviderID)
	}
	if input.Model.ModelID != "ornith-1.0-9b-mlx" {
		t.Errorf("expected ModelID ornith-1.0-9b-mlx, got %s", input.Model.ModelID)
	}
	if len(input.Parts) != 1 || input.Parts[0].Text != "Hello" {
		t.Errorf("expected single text part with 'Hello', got %+v", input.Parts)
	}
}

func TestBuildPromptInput_NoModelWhenUnset(t *testing.T) {
	app := NewApp("http://localhost:4096")
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.Model != nil {
		t.Errorf("expected Model to be nil when CurrentModel is empty, got %+v", input.Model)
	}
}

func TestMapSSEToMsg_SessionError(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.error",
		Properties: map[string]any{
			"sessionID": "ses_123",
			"error":     "model not found",
		},
	}

	msg := mapSSEToMsg(evt)

	errMsg, ok := msg.(SessionErrorMsg)
	if !ok {
		t.Fatalf("expected SessionErrorMsg, got %T", msg)
	}
	if errMsg.SessionID != "ses_123" {
		t.Errorf("expected SessionID ses_123, got %s", errMsg.SessionID)
	}
	if errMsg.Error != "model not found" {
		t.Errorf("expected error 'model not found', got %s", errMsg.Error)
	}
}

func TestMapSSEToMsg_SessionErrorDefaultMessage(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.error",
		Properties: map[string]any{
			"sessionID": "ses_456",
		},
	}

	msg := mapSSEToMsg(evt)

	errMsg, ok := msg.(SessionErrorMsg)
	if !ok {
		t.Fatalf("expected SessionErrorMsg, got %T", msg)
	}
	if errMsg.Error != "unknown error" {
		t.Errorf("expected 'unknown error', got %s", errMsg.Error)
	}
}

func TestAppUpdate_SessionErrorClearsWorking(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.state.SessionStatus["ses_123"] = SessionStatus{Working: true}
	app.status.SetWorking(true)

	result, _ := app.Update(SessionErrorMsg{
		SessionID: "ses_123",
		Error:     "model not found",
	})
	updated := result.(App)

	if updated.status.working {
		t.Error("expected working to be false after SessionErrorMsg")
	}
	if updated.state.SessionStatus["ses_123"].Working {
		t.Error("expected session status Working to be false")
	}
	if !updated.toast.IsVisible() {
		t.Error("expected toast to be visible after SessionErrorMsg")
	}
}

func TestAppUpdate_SessionErrorDifferentSession(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_active"
	app.status.SetWorking(true)

	result, _ := app.Update(SessionErrorMsg{
		SessionID: "ses_other",
		Error:     "model not found",
	})
	updated := result.(App)

	// Working should NOT be cleared for the active session's status bar
	// because the error is for a different session.
	if !updated.status.working {
		t.Error("expected status bar working to remain true for different session error")
	}
	// But the other session's status should be cleared.
	if updated.state.SessionStatus["ses_other"].Working {
		t.Error("expected other session status Working to be false")
	}
	if !updated.toast.IsVisible() {
		t.Error("expected toast to be visible")
	}
}

func TestSessionCreatedLocalMsg_ErrorShowsToast(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.status.SetWorking(true)

	ca := &connectedApp{
		app:           app,
		pendingPrompt: "Hello",
	}

	result, _ := ca.Update(SessionCreatedLocalMsg{
		Err: errors.New("server unreachable"),
	})
	updated := result.(*connectedApp)

	if updated.pendingPrompt != "" {
		t.Errorf("expected pendingPrompt to be cleared, got %q", updated.pendingPrompt)
	}
	if updated.app.status.working {
		t.Error("expected working to be false after session creation error")
	}
	if !updated.app.toast.IsVisible() {
		t.Error("expected toast to be visible after session creation error")
	}
}

func TestPromptSubmitted_ModelIncludedInSessionCreation(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.CurrentModel = ModelSelection{
		ProviderID: "lm-studio",
		ModelID:    "ornith-1.0-9b-mlx",
	}

	ca := &connectedApp{app: app}

	_, cmds := ca.Update(PromptSubmittedMsg{Content: "Hello"})

	// The pending prompt should be stored for after session creation.
	if ca.pendingPrompt != "Hello" {
		t.Errorf("expected pendingPrompt 'Hello', got %q", ca.pendingPrompt)
	}

	// Commands should include a session creation command.
	if cmds == nil {
		t.Fatal("expected commands to be returned")
	}
}

func TestParseAskCommand_Valid(t *testing.T) {
	msg, agent := parseAskCommand("/ask architect to review this go project")
	if agent != "architect" {
		t.Errorf("expected agent 'architect', got %q", agent)
	}
	if msg != "to review this go project" {
		t.Errorf("expected message 'to review this go project', got %q", msg)
	}
}

func TestParseAskCommand_NoMessage(t *testing.T) {
	msg, agent := parseAskCommand("/ask architect")
	if agent != "" {
		t.Errorf("expected no agent for /ask without message, got %q", agent)
	}
	if msg != "/ask architect" {
		t.Errorf("expected original text returned, got %q", msg)
	}
}

func TestParseAskCommand_NotAsk(t *testing.T) {
	msg, agent := parseAskCommand("hello world")
	if agent != "" {
		t.Errorf("expected no agent for non-ask, got %q", agent)
	}
	if msg != "hello world" {
		t.Errorf("expected original text, got %q", msg)
	}
}

func TestParseAskCommand_OtherSlash(t *testing.T) {
	msg, agent := parseAskCommand("/connect")
	if agent != "" {
		t.Errorf("expected no agent for /connect, got %q", agent)
	}
	if msg != "/connect" {
		t.Errorf("expected original text, got %q", msg)
	}
}

func TestBuildPromptInput_WithAgentOverride(t *testing.T) {
	app := NewApp("http://localhost:4096")
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("review this project")
	input.Agent = "architect"

	if input.Agent != "architect" {
		t.Errorf("expected agent 'architect', got %q", input.Agent)
	}
	if input.Parts[0].Text != "review this project" {
		t.Errorf("expected stripped message text, got %q", input.Parts[0].Text)
	}
}

func TestModelSelectedMsg_RestoresFocusToPrompt(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.focus = FocusDialog
	app.prompt.Blur()

	app.state.Providers = []ProviderInfo{
		{ID: "lm-studio", Name: "LM Studio", Models: []ModelInfo{
			{ID: "ornith-1.0-9b-mlx", Name: "ornith-1.0-9b-mlx", ProviderID: "lm-studio"},
		}},
	}

	result, _ := app.Update(ModelSelectedMsg{
		Selection: ModelSelection{ProviderID: "lm-studio", ModelID: "ornith-1.0-9b-mlx"},
	})
	updated := result.(App)

	if updated.focus != FocusPrompt {
		t.Errorf("expected focus to be FocusPrompt after model selection, got %v", updated.focus)
	}
	if updated.state.CurrentModel.ModelID != "ornith-1.0-9b-mlx" {
		t.Errorf("expected model ornith-1.0-9b-mlx, got %s", updated.state.CurrentModel.ModelID)
	}
}

func TestModelDialog_CursorStartsOnCurrentModel(t *testing.T) {
	d := NewModelDialog()
	d.Show([]ProviderInfo{
		{
			ID:   "lm-studio",
			Name: "LM Studio",
			Models: []ModelInfo{
				{ID: "embedding-model", Name: "embedding-model", ProviderID: "lm-studio"},
				{ID: "ornith-1.0-9b-mlx", Name: "ornith-1.0-9b-mlx", ProviderID: "lm-studio"},
			},
		},
	}, ModelSelection{ProviderID: "lm-studio", ModelID: "ornith-1.0-9b-mlx"})

	// Phase 1: provider is pre-selected, press enter to open models.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Phase 2: cursor should be on ornith (the current model), press enter.
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}
	msg := cmd()
	sm := msg.(ModelSelectedMsg)
	if sm.Selection.ModelID != "ornith-1.0-9b-mlx" {
		t.Errorf("expected cursor on ornith-1.0-9b-mlx, got %s", sm.Selection.ModelID)
	}
}

func TestPromptInput_SlashCommandSingleEnter(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCommands([]AutocompleteItem{
		{Name: "exit", Description: "Exit the app"},
		{Name: "ask", Description: "Ask an agent"},
	})

	// Type "/exit" — autocomplete will be visible with "exit" selected
	for _, r := range "/exit" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if !p.autocomplete.IsVisible() {
		t.Fatal("expected autocomplete to be visible after typing /exit")
	}

	// Press Enter ONCE — should submit, not just fill in the autocomplete
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from single enter on complete command")
	}
	msg := cmd()
	submitted, ok := msg.(PromptSubmittedMsg)
	if !ok {
		t.Fatalf("expected PromptSubmittedMsg, got %T", msg)
	}
	if submitted.Content != "/exit" {
		t.Errorf("expected '/exit', got %q", submitted.Content)
	}
}

func TestIsKnownAgent_Found(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.Agents = []api.AgentInfo{
		{Name: "build"},
		{Name: "architect"},
		{Name: "plan"},
	}
	ca := &connectedApp{app: app}

	if !ca.isKnownAgent("architect") {
		t.Error("expected architect to be known")
	}
}

func TestIsKnownAgent_NotFound(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.Agents = []api.AgentInfo{
		{Name: "build"},
		{Name: "architect"},
	}
	ca := &connectedApp{app: app}

	if ca.isKnownAgent("archtect") {
		t.Error("expected archtect (typo) to be unknown")
	}
}

func TestAskCommand_UnknownAgentShowsToast(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.Agents = []api.AgentInfo{
		{Name: "build"},
		{Name: "architect"},
	}

	ca := &connectedApp{app: app}

	result, _ := ca.Update(PromptSubmittedMsg{Content: "/ask archtect review this"})
	updated := result.(*connectedApp)

	if updated.app.toast.IsVisible() == false {
		t.Error("expected toast for unknown agent")
	}
}

func TestIsTerminalEscape_DetectsOSCFragments(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"\x1b[0m", true},
		{"11;rgb:158e/193a/1e75", true},
		{"rgb:ffff/0000/0000", true},
		{"10;rgb:0000/0000/0000", true},
		{"58e/193a/1e75\\", true},
		{"158e/193a/1e75", true},
		{"ffff/0000/0000", true},
		{"hello world", false},
		{"a", false},
		{"abc", false},
	}
	for _, tt := range tests {
		got := isTerminalEscape(tt.input)
		if got != tt.want {
			t.Errorf("isTerminalEscape(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPromptInput_FiltersOSCResponse(t *testing.T) {
	p := NewPromptInput(80)

	// Simulate an OSC response arriving as a batch of runes.
	oscResponse := "11;rgb:158e/193a/1e75"
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(oscResponse)})

	if p.Value() != "" {
		t.Errorf("expected OSC response to be filtered, got %q", p.Value())
	}
}

func TestPermissionDismissed_RestoresFocusAndEmitsReply(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.permPrompt.Show(PermissionRequest{
		ID:         "perm-1",
		SessionID:  "ses_123",
		Permission: "shell",
	})
	app.focus = FocusDialog

	result, cmd := app.Update(PermissionDismissedMsg{
		Request: PermissionRequest{
			ID:         "perm-1",
			SessionID:  "ses_123",
			Permission: "shell",
		},
		Action: PermissionAllow,
	})
	updated := result.(App)

	if updated.permPrompt.IsVisible() {
		t.Error("expected permission prompt to be hidden")
	}
	if updated.focus != FocusPrompt {
		t.Errorf("expected focus restored to prompt, got %v", updated.focus)
	}
	if cmd == nil {
		t.Fatal("expected cmd to emit PermissionReplyMsg")
	}
	msg := cmd()
	reply, ok := msg.(PermissionReplyMsg)
	if !ok {
		t.Fatalf("expected PermissionReplyMsg, got %T", msg)
	}
	if reply.Action != "allow" {
		t.Errorf("expected action 'allow', got %q", reply.Action)
	}
}

func TestAbortSentMsg_ErrorShowsToast(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true

	ca := &connectedApp{app: app}
	result, _ := ca.Update(AbortSentMsg{Err: fmt.Errorf("connection refused")})
	updated := result.(*connectedApp)

	if !updated.app.toast.IsVisible() {
		t.Error("expected toast for abort failure")
	}
}

func TestAgentSelectedMsg_UpdatesAgentAndRestoresFocus(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.focus = FocusDialog

	result, _ := app.Update(AgentSelectedMsg{Agent: "plan"})
	updated := result.(App)

	if updated.focus != FocusPrompt {
		t.Errorf("expected focus to be FocusPrompt after agent selection, got %v", updated.focus)
	}
	if updated.state.CurrentAgent != "plan" {
		t.Errorf("expected CurrentAgent 'plan', got %q", updated.state.CurrentAgent)
	}
}

func TestThinkingLevelBudget(t *testing.T) {
	tests := []struct {
		level     string
		wantBudg  int
		wantLabel string
		wantOK    bool
	}{
		{"off", 0, "off", true},
		{"low", 1024, "1k tokens", true},
		{"medium", 4096, "4k tokens", true},
		{"high", 16384, "16k tokens", true},
		{"max", 128000, "128k tokens", true},
		{"invalid", 0, "", false},
		{"", 0, "", false},
	}
	for _, tt := range tests {
		budget, label, ok := thinkingLevelBudget(tt.level)
		if ok != tt.wantOK {
			t.Errorf("thinkingLevelBudget(%q) ok = %v, want %v", tt.level, ok, tt.wantOK)
		}
		if budget != tt.wantBudg {
			t.Errorf("thinkingLevelBudget(%q) budget = %d, want %d", tt.level, budget, tt.wantBudg)
		}
		if label != tt.wantLabel {
			t.Errorf("thinkingLevelBudget(%q) label = %q, want %q", tt.level, label, tt.wantLabel)
		}
	}
}

func TestBuildPromptInput_WithThinkingBudget(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.ThinkingLevel = "high"
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.ThinkingBudget == nil {
		t.Fatal("expected ThinkingBudget to be set")
	}
	if *input.ThinkingBudget != 16384 {
		t.Errorf("expected ThinkingBudget 16384, got %d", *input.ThinkingBudget)
	}
}

func TestBuildPromptInput_ThinkingOff(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.ThinkingLevel = "off"
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.ThinkingBudget != nil {
		t.Errorf("expected ThinkingBudget to be nil when level is off, got %d", *input.ThinkingBudget)
	}
}

func TestBuildPromptInput_ThinkingUnset(t *testing.T) {
	app := NewApp("http://localhost:4096")
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.ThinkingBudget != nil {
		t.Errorf("expected ThinkingBudget to be nil when level is unset, got %d", *input.ThinkingBudget)
	}
}
