package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// execCmdQuick runs a tea.Cmd and returns its message, or nil if it does not
// resolve quickly. This avoids blocking tests on long-lived commands such as
// toast expiry timers, which fire well outside any test-relevant window.
func execCmdQuick(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// flattenCmd runs a tea.Cmd and recursively unwraps any tea.BatchMsg,
// returning the flat list of resulting messages.
func flattenCmd(cmd tea.Cmd) []tea.Msg {
	msg := execCmdQuick(cmd)
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, flattenCmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// findMsg returns the first message of type T within msgs, or the zero
// value and false if none is found.
func findMsg[T any](msgs []tea.Msg) (T, bool) {
	var zero T
	for _, m := range msgs {
		if typed, ok := m.(T); ok {
			return typed, true
		}
	}
	return zero, false
}

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

func TestBuildPromptInput_IncludesCurrentAgent(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.CurrentAgent = "plan"
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.Agent != "plan" {
		t.Errorf("expected Agent %q, got %q", "plan", input.Agent)
	}
}

func TestBuildPromptInput_NoAgentWhenUnset(t *testing.T) {
	app := NewApp("http://localhost:4096")
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.Agent != "" {
		t.Errorf("expected empty Agent when CurrentAgent unset, got %q", input.Agent)
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

func TestMapSSEToMsg_SessionErrorStructuredPayload(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.error",
		Properties: map[string]any{
			"sessionID": "ses_789",
			"error": map[string]any{
				"name": "UnknownError",
				"data": map[string]any{
					"message": "doom loop detected: last 3 tool calls were identical (bash)",
				},
			},
		},
	}

	msg := mapSSEToMsg(evt)

	errMsg, ok := msg.(SessionErrorMsg)
	if !ok {
		t.Fatalf("expected SessionErrorMsg, got %T", msg)
	}
	if errMsg.SessionID != "ses_789" {
		t.Errorf("expected SessionID ses_789, got %s", errMsg.SessionID)
	}
	expected := "doom loop detected: last 3 tool calls were identical (bash)"
	if errMsg.Error != expected {
		t.Errorf("expected error %q, got %q", expected, errMsg.Error)
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

func TestAppUpdate_SessionErrorNoModel_OpensConnect(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.state.SessionStatus["ses_123"] = SessionStatus{Working: true}
	app.status.SetWorking(true)

	result, cmd := app.Update(SessionErrorMsg{
		SessionID: "ses_123",
		Error:     "no model specified for prompt — configure a default model",
	})
	updated := result.(App)

	if !updated.state.PendingModelDialog {
		t.Error("expected PendingModelDialog to be set for no-model error")
	}
	if !updated.toast.IsVisible() {
		t.Error("expected toast to be visible after SessionErrorMsg")
	}
	if cmd == nil {
		t.Fatal("expected ProvidersRefreshMsg cmd")
	}
	// tea.Batch may wrap; execute and look for ProvidersRefreshMsg
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		found := false
		for _, c := range batch {
			if c == nil {
				continue
			}
			inner := c()
			if _, ok := inner.(ProvidersRefreshMsg); ok {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ProvidersRefreshMsg in batch, got %#v", batch)
		}
	} else if _, ok := msg.(ProvidersRefreshMsg); !ok {
		t.Errorf("expected ProvidersRefreshMsg, got %T", msg)
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
		{";1R", true},
		{"24;1R", true},
		{"12;80R", true},
		{"][53;1R", true},
		{"]\\[53;1R", true},
		{"[53;1R", true},
		{"1R", false}, // too ambiguous without semicolon
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

func TestPromptInput_FiltersCPRFragment(t *testing.T) {
	p := NewPromptInput(80)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";1R")})
	if p.Value() != "" {
		t.Errorf("expected CPR fragment to be filtered, got %q", p.Value())
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("24;1R")})
	if p.Value() != "" {
		t.Errorf("expected full CPR to be filtered, got %q", p.Value())
	}
}

func TestPromptInput_FiltersMangledCPR(t *testing.T) {
	cases := []string{"][53;1R", `]\[53;1R`, "[53;1R", "53;1R"}
	for _, leak := range cases {
		p := NewPromptInput(80)
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leak)})
		if p.Value() != "" {
			t.Errorf("expected mangled CPR %q to be filtered, got %q", leak, p.Value())
		}
	}
}

func TestPromptInput_FiltersSplitMangledCPR(t *testing.T) {
	p := NewPromptInput(80)
	for _, ch := range []string{"]", "\\", "[", "5", "3", ";", "1", "R"} {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(ch)})
	}
	if p.Value() != "" {
		t.Errorf("expected split mangled CPR to be filtered, got %q", p.Value())
	}
}

func TestPromptInput_ScrubsCPRAppendedToText(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("hello]\\[53;1R")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	// Scrub runs on forward; typing 'x' triggers it.
	if strings.Contains(p.Value(), ";1R") || strings.Contains(p.Value(), "]\\[") {
		t.Errorf("expected CPR scrub from value, got %q", p.Value())
	}
}

func TestScrubCPRValue(t *testing.T) {
	tests := []struct {
		in, want string
		changed  bool
	}{
		{`]\[53;1R`, "", true},
		{"][53;1R", "", true},
		{"hello][53;1R", "hello", true},
		{"hello", "hello", false},
		{";1R", "", true},
	}
	for _, tt := range tests {
		got, changed := scrubCPRValue(tt.in)
		if got != tt.want || changed != tt.changed {
			t.Errorf("scrubCPRValue(%q) = (%q, %v), want (%q, %v)", tt.in, got, changed, tt.want, tt.changed)
		}
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
	if reply.Action != "once" {
		t.Errorf("expected action 'once', got %q", reply.Action)
	}
}

func TestEscDismissesToastWhenNotWorking(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.state.SessionStatus["ses_123"] = SessionStatus{Working: false}

	// Show an error toast
	app.toast.Show("doom loop detected", true)

	if !app.toast.IsVisible() {
		t.Fatal("expected toast to be visible before escape")
	}

	result, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(App)

	if updated.toast.IsVisible() {
		t.Error("expected toast to be dismissed by escape key")
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

func TestAgentSelectedMsg_UpdatesPromptSubmissionAgent(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_1"
	app.state.Sessions = []SessionInfo{{ID: "ses_1", Agent: "build"}}
	app.state.CurrentAgent = "build"
	ca := &connectedApp{app: app}

	result, _ := ca.app.Update(AgentSelectedMsg{Agent: "architect"})
	ca.app = result.(App)

	input := ca.buildPromptInput("review this")
	if input.Agent != "architect" {
		t.Errorf("expected submission agent %q after Tab selection, got %q", "architect", input.Agent)
	}
	if ca.app.state.Sessions[0].Agent != "architect" {
		t.Errorf("expected local session agent updated to %q, got %q", "architect", ca.app.state.Sessions[0].Agent)
	}

	// syncPromptMetadata must not wipe Tab selection back to a stale session agent.
	ca.app.syncPromptMetadata()
	input = ca.buildPromptInput("still architect")
	if input.Agent != "architect" {
		t.Errorf("expected agent %q after syncPromptMetadata, got %q", "architect", input.Agent)
	}
}

func TestSyncPromptMetadata_SetsCurrentAgent(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.ActiveSession = "ses_1"
	app.state.Sessions = []SessionInfo{{ID: "ses_1", Agent: "plan", ModelID: "m", ProviderID: "p"}}
	app.state.CurrentAgent = ""

	app.syncPromptMetadata()

	if app.state.CurrentAgent != "plan" {
		t.Errorf("expected CurrentAgent %q after sync, got %q", "plan", app.state.CurrentAgent)
	}
	ca := &connectedApp{app: app}
	input := ca.buildPromptInput("hello")
	if input.Agent != "plan" {
		t.Errorf("expected buildPromptInput agent %q, got %q", "plan", input.Agent)
	}
}

func TestHandleAgentListMsg_DefaultPrimaryCycle(t *testing.T) {
	app := NewApp("http://localhost:4096")
	result, _ := app.handleAgentListMsg(AgentListMsg{
		Agents: []api.AgentInfo{
			{Name: "build", Mode: "primary"},
			{Name: "plan", Mode: "primary"},
			{Name: "architect", Mode: "subagent"},
			{Name: "code-reviewer", Mode: "subagent"},
			{Name: "debugger", Mode: "subagent"},
			{Name: "executor", Mode: "primary"},
		},
	})

	got := result.prompt.cycleAgents
	want := []string{"build", "plan", "architect", "code-reviewer"}
	if len(got) != len(want) {
		t.Fatalf("cycle agents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cycle agents = %v, want %v", got, want)
		}
	}
	// Autocomplete / picker still has all enabled agents in state.
	if len(result.state.Agents) != 6 {
		t.Errorf("expected all 6 enabled agents in state, got %d", len(result.state.Agents))
	}
}

func TestResolveCycleAgents_ConfigOverride(t *testing.T) {
	enabled := []api.AgentInfo{
		{Name: "build"},
		{Name: "plan"},
		{Name: "debugger"},
		{Name: "architect"},
	}
	got := resolveCycleAgents([]string{"build", "debugger", "missing"}, enabled)
	want := []string{"build", "debugger"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
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

func TestEffortLevelSettings(t *testing.T) {
	tests := []struct {
		level         string
		wantMaxTokens int
		wantPrefix    string
		wantMaxIter   int
		wantOK        bool
	}{
		{"low", 1024, "Be concise and direct.", 2, true},
		{"medium", 4096, "", 5, true},
		{"high", 8192, "Be thorough and comprehensive.", 10, true},
		{"max", 0, "Be exhaustive. Use every tool at your disposal.", 20, true},
		{"invalid", 0, "", 0, false},
		{"", 0, "", 0, false},
	}
	for _, tt := range tests {
		settings, ok := effortLevelSettings(tt.level)
		if ok != tt.wantOK {
			t.Errorf("effortLevelSettings(%q) ok = %v, want %v", tt.level, ok, tt.wantOK)
		}
		if settings.MaxTokens != tt.wantMaxTokens {
			t.Errorf("effortLevelSettings(%q) MaxTokens = %d, want %d", tt.level, settings.MaxTokens, tt.wantMaxTokens)
		}
		if settings.SystemPrefix != tt.wantPrefix {
			t.Errorf("effortLevelSettings(%q) SystemPrefix = %q, want %q", tt.level, settings.SystemPrefix, tt.wantPrefix)
		}
		if settings.MaxIterations != tt.wantMaxIter {
			t.Errorf("effortLevelSettings(%q) MaxIterations = %d, want %d", tt.level, settings.MaxIterations, tt.wantMaxIter)
		}
	}
}

func TestBuildPromptInput_WithEffort(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.EffortLevel = "high"
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.MaxTokens == nil {
		t.Fatal("expected MaxTokens to be set for effort high")
	}
	if *input.MaxTokens != 8192 {
		t.Errorf("expected MaxTokens 8192, got %d", *input.MaxTokens)
	}
	if input.SystemPrefix != "Be thorough and comprehensive." {
		t.Errorf("expected SystemPrefix for high, got %q", input.SystemPrefix)
	}
	if input.MaxIterations == nil {
		t.Fatal("expected MaxIterations to be set for effort high")
	}
	if *input.MaxIterations != 10 {
		t.Errorf("expected MaxIterations 10, got %d", *input.MaxIterations)
	}
}

func TestBuildPromptInput_EffortMediumDefault(t *testing.T) {
	app := NewApp("http://localhost:4096")
	ca := &connectedApp{app: app}

	input := ca.buildPromptInput("Hello")

	if input.MaxTokens != nil {
		t.Errorf("expected MaxTokens nil for default effort, got %d", *input.MaxTokens)
	}
	if input.SystemPrefix != "" {
		t.Errorf("expected empty SystemPrefix for default effort, got %q", input.SystemPrefix)
	}
	if input.MaxIterations != nil {
		t.Errorf("expected MaxIterations nil for default effort, got %d", *input.MaxIterations)
	}
}

func TestConnectedApp_RewindForkMsg_CallsForkAPI(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":        "ses_fork1",
			"parentID":  "ses_parent",
			"title":     "Test Session (turn 2)",
			"projectID": "prj_test",
			"directory": "/tmp",
			"version":   "1.0",
			"tokens":    map[string]any{},
			"time":      map[string]any{"created": 0, "updated": 0},
		})
	}))
	defer srv.Close()

	app := NewApp(srv.URL)
	app.state.ActiveSession = "ses_parent"
	app.state.Sessions = []SessionInfo{{ID: "ses_parent", Title: "Test Session"}}
	client := api.New(srv.URL, "/tmp", "")
	ca := &connectedApp{app: app, client: client}

	_, cmd := ca.Update(RewindForkMsg{Turn: RewindTurn{Index: 2, MessageID: "m3", Preview: "Second"}})
	if cmd == nil {
		t.Fatal("expected a command from RewindForkMsg")
	}

	forkDone, ok := findMsg[ForkDoneMsg](flattenCmd(cmd))
	if !ok {
		t.Fatal("expected a ForkDoneMsg from the dispatched command")
	}
	if forkDone.Err != nil {
		t.Fatalf("unexpected fork error: %v", forkDone.Err)
	}
	if forkDone.Session == nil || forkDone.Session.ID != "ses_fork1" {
		t.Fatalf("expected forked session ses_fork1, got %+v", forkDone.Session)
	}
	if forkDone.Session.ParentID != "ses_parent" {
		t.Errorf("forked session ParentID = %q, want %q", forkDone.Session.ParentID, "ses_parent")
	}

	if gotPath != "/session/ses_parent/fork" {
		t.Errorf("path = %q, want /session/ses_parent/fork", gotPath)
	}
	if gotBody["messageID"] != "m3" {
		t.Errorf("body messageID = %v, want %q", gotBody["messageID"], "m3")
	}
}

func TestConnectedApp_RewindForkMsg_NoActiveSession(t *testing.T) {
	app := NewApp("http://localhost:4096")
	ca := &connectedApp{app: app}

	_, cmd := ca.Update(RewindForkMsg{Turn: RewindTurn{Index: 1, MessageID: "m1"}})
	if cmd == nil {
		t.Fatal("expected an error toast command when no active session")
	}
}

func TestConnectedApp_ForkDoneMsg_UpdatesSidebarAndSwitchesSession(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_parent"
	app.state.Sessions = []SessionInfo{{ID: "ses_parent", Title: "Test Session"}}
	ca := &connectedApp{app: app}

	forked := SessionInfo{ID: "ses_fork1", Title: "Test Session (turn 2)", ParentID: "ses_parent"}
	_, cmd := ca.Update(ForkDoneMsg{Session: &forked, TurnIndex: 2})

	found := false
	for _, s := range ca.app.state.Sessions {
		if s.ID == "ses_fork1" {
			found = true
			if s.ParentID != "ses_parent" {
				t.Errorf("forked session ParentID = %q, want %q", s.ParentID, "ses_parent")
			}
		}
	}
	if !found {
		t.Fatal("expected forked session to be added to state.Sessions")
	}

	if cmd == nil {
		t.Fatal("expected a command after ForkDoneMsg")
	}
	switched, ok := findMsg[SessionSwitchedMsg](flattenCmd(cmd))
	if !ok {
		t.Fatal("expected a SessionSwitchedMsg from the dispatched command")
	}
	if switched.SessionID != "ses_fork1" {
		t.Errorf("expected switch to ses_fork1, got %q", switched.SessionID)
	}
}

func TestConnectedApp_ForkDoneMsg_Error(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.state.ActiveSession = "ses_parent"
	ca := &connectedApp{app: app}

	_, cmd := ca.Update(ForkDoneMsg{Err: errors.New("boom")})
	if cmd == nil {
		t.Fatal("expected an error toast command")
	}
	if len(ca.app.state.Sessions) != 0 {
		t.Errorf("expected no sessions added on error, got %+v", ca.app.state.Sessions)
	}
}
