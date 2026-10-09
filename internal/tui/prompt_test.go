package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewPromptInput
// ---------------------------------------------------------------------------

func TestNewPromptInput_DefaultAgent(t *testing.T) {
	p := NewPromptInput(80)
	if p.agent != "build" {
		t.Fatalf("expected default agent 'build', got %q", p.agent)
	}
}

func TestNewPromptInput_WaveOnByDefault(t *testing.T) {
	p := NewPromptInput(80)
	if !p.waveOn {
		t.Fatal("expected waveOn true by default")
	}
}

func TestNewPromptInput_WidthStored(t *testing.T) {
	p := NewPromptInput(120)
	if p.width != 120 {
		t.Fatalf("expected width 120, got %d", p.width)
	}
}

func TestNewPromptInput_TextareaFocused(t *testing.T) {
	p := NewPromptInput(80)
	if !p.textarea.Focused() {
		t.Fatal("expected textarea to be focused by default")
	}
}

func TestNewPromptInput_CharLimitZero(t *testing.T) {
	p := NewPromptInput(80)
	if p.textarea.CharLimit != 0 {
		t.Fatalf("expected CharLimit 0 (unlimited), got %d", p.textarea.CharLimit)
	}
}

// ---------------------------------------------------------------------------
// SetValue / Value
// ---------------------------------------------------------------------------

func TestSetValue_GetValue(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("hello world")
	if p.Value() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", p.Value())
	}
}

func TestSetValue_Overwrite(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("first")
	p.SetValue("second")
	if p.Value() != "second" {
		t.Fatalf("expected 'second', got %q", p.Value())
	}
}

func TestReset_ClearsTextarea(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("some text")
	p.Reset()
	if p.Value() != "" {
		t.Fatalf("expected empty after Reset, got %q", p.Value())
	}
}

// ---------------------------------------------------------------------------
// SetMetadata
// ---------------------------------------------------------------------------

func TestSetMetadata_UpdatesFields(t *testing.T) {
	p := NewPromptInput(80)
	p.SetMetadata("architect", "ornith-1.0-9b-mlx", "lm-studio")
	if p.agent != "architect" {
		t.Fatalf("expected agent 'architect', got %q", p.agent)
	}
	if p.model != "ornith-1.0-9b-mlx" {
		t.Fatalf("expected model 'ornith-1.0-9b-mlx', got %q", p.model)
	}
	if p.provider != "lm-studio" {
		t.Fatalf("expected provider 'lm-studio', got %q", p.provider)
	}
}

// ---------------------------------------------------------------------------
// SetThinkingLevel / SetEffortLevel
// ---------------------------------------------------------------------------

func TestSetThinkingLevel_UpdatesField(t *testing.T) {
	p := NewPromptInput(80)
	p.SetThinkingLevel("high")
	if p.thinkingLevel != "high" {
		t.Fatalf("expected thinking level 'high', got %q", p.thinkingLevel)
	}
}

func TestSetEffortLevel_UpdatesField(t *testing.T) {
	p := NewPromptInput(80)
	p.SetEffortLevel("low")
	if p.effortLevel != "low" {
		t.Fatalf("expected effort level 'low', got %q", p.effortLevel)
	}
}

// ---------------------------------------------------------------------------
// AddImage / ClearImages / HasImages
// ---------------------------------------------------------------------------

func TestAddImage_IncrementsCount(t *testing.T) {
	p := NewPromptInput(80)
	if p.HasImages() {
		t.Fatal("expected no images initially")
	}
	p.AddImage(1024)
	if !p.HasImages() {
		t.Fatal("expected HasImages true after AddImage")
	}
	if p.imageCount != 1 {
		t.Fatalf("expected imageCount 1, got %d", p.imageCount)
	}
	if p.imageSize != 1024 {
		t.Fatalf("expected imageSize 1024, got %d", p.imageSize)
	}
}

func TestAddImage_AccumulatesSize(t *testing.T) {
	p := NewPromptInput(80)
	p.AddImage(1024)
	p.AddImage(2048)
	if p.imageCount != 2 {
		t.Fatalf("expected imageCount 2, got %d", p.imageCount)
	}
	if p.imageSize != 3072 {
		t.Fatalf("expected imageSize 3072, got %d", p.imageSize)
	}
}

func TestClearImages_ResetsCountAndSize(t *testing.T) {
	p := NewPromptInput(80)
	p.AddImage(1024)
	p.AddImage(2048)
	p.ClearImages()
	if p.HasImages() {
		t.Fatal("expected HasImages false after ClearImages")
	}
	if p.imageCount != 0 {
		t.Fatalf("expected imageCount 0, got %d", p.imageCount)
	}
	if p.imageSize != 0 {
		t.Fatalf("expected imageSize 0, got %d", p.imageSize)
	}
}

// ---------------------------------------------------------------------------
// Focus / Blur
// ---------------------------------------------------------------------------

func TestFocusBlur_TogglesTextareaFocus(t *testing.T) {
	p := NewPromptInput(80)
	p.Blur()
	if p.textarea.Focused() {
		t.Fatal("expected textarea blurred after Blur()")
	}
	p.Focus()
	if !p.textarea.Focused() {
		t.Fatal("expected textarea focused after Focus()")
	}
}

// ---------------------------------------------------------------------------
// SetSize / Height
// ---------------------------------------------------------------------------

func TestSetSize_UpdatesWidth(t *testing.T) {
	p := NewPromptInput(80)
	p.SetSize(120)
	if p.width != 120 {
		t.Fatalf("expected width 120, got %d", p.width)
	}
}

func TestHeight_ReturnsAtLeastMinimum(t *testing.T) {
	p := NewPromptInput(80)
	h := p.Height()
	// Minimum: 1 (top rule) + textarea height + 1 (bottom bar) = 5
	if h < 3 {
		t.Fatalf("expected height >= 3, got %d", h)
	}
}

func TestHeight_IncreasesWithMetadata(t *testing.T) {
	p := NewPromptInput(80)
	baseHeight := p.Height()

	p.SetThinkingLevel("high")
	withMeta := p.Height()
	if withMeta <= baseHeight {
		t.Fatalf("expected height to increase with metadata, base=%d meta=%d", baseHeight, withMeta)
	}
}

// ---------------------------------------------------------------------------
// SetCwd / SetCommands / SetAgents / SetCycleAgents
// ---------------------------------------------------------------------------

func TestSetCwd_UpdatesField(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCwd("/tmp/test")
	if p.cwd != "/tmp/test" {
		t.Fatalf("expected cwd '/tmp/test', got %q", p.cwd)
	}
}

func TestSetCycleAgents_UpdatesField(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCycleAgents([]string{"build", "architect", "debugger"})
	if len(p.cycleAgents) != 3 {
		t.Fatalf("expected 3 cycle agents, got %d", len(p.cycleAgents))
	}
}

// ---------------------------------------------------------------------------
// nextAgent
// ---------------------------------------------------------------------------

func TestNextAgent_CyclesForward(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "build"
	p.SetCycleAgents([]string{"build", "architect", "debugger"})

	next := p.nextAgent(1)
	if next != "architect" {
		t.Fatalf("expected 'architect', got %q", next)
	}
}

func TestNextAgent_CyclesBackward(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "build"
	p.SetCycleAgents([]string{"build", "architect", "debugger"})

	next := p.nextAgent(-1)
	if next != "debugger" {
		t.Fatalf("expected 'debugger', got %q", next)
	}
}

func TestNextAgent_WrapsForward(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "debugger"
	p.SetCycleAgents([]string{"build", "architect", "debugger"})

	next := p.nextAgent(1)
	if next != "build" {
		t.Fatalf("expected 'build' after wrap, got %q", next)
	}
}

func TestNextAgent_EmptyCycleReturnsCurrent(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "build"
	// No cycle agents set
	next := p.nextAgent(1)
	if next != "build" {
		t.Fatalf("expected current agent 'build' when no cycle agents, got %q", next)
	}
}

func TestNextAgent_UnknownAgentStartsAtZero(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "nonexistent"
	p.SetCycleAgents([]string{"build", "architect", "debugger"})

	// When current agent isn't found, idx stays 0, so nextAgent(1) returns index 1
	next := p.nextAgent(1)
	if next != "architect" {
		t.Fatalf("expected 'architect' from index 0+1, got %q", next)
	}
}

// ---------------------------------------------------------------------------
// Update — Enter submits
// ---------------------------------------------------------------------------

func TestUpdate_EnterSubmitsContent(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("hello agent")

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd after Enter with content")
	}
	msg := cmd()
	sub, ok := msg.(PromptSubmittedMsg)
	if !ok {
		t.Fatalf("expected PromptSubmittedMsg, got %T", msg)
	}
	if sub.Content != "hello agent" {
		t.Fatalf("expected content 'hello agent', got %q", sub.Content)
	}
	// Textarea should be cleared after submit
	if p.Value() != "" {
		t.Fatalf("expected textarea cleared after submit, got %q", p.Value())
	}
}

func TestUpdate_EnterAddsToHistory(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("some prompt")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.history.Len() != 1 {
		t.Fatalf("expected 1 history entry, got %d", p.history.Len())
	}
}

func TestUpdate_EnterEmptyDoesNotSubmit(t *testing.T) {
	p := NewPromptInput(80)
	// Empty textarea
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected nil cmd for empty Enter")
	}
}

func TestUpdate_EnterDisablesWave(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("first")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.waveOn {
		t.Fatal("expected waveOn false after first submit")
	}
}

// ---------------------------------------------------------------------------
// Update — Tab cycles agent
// ---------------------------------------------------------------------------

func TestUpdate_TabCyclesAgent(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "build"
	p.SetCycleAgents([]string{"build", "architect", "debugger"})

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected cmd from Tab")
	}
	msg := cmd()
	sel, ok := msg.(AgentSelectedMsg)
	if !ok {
		t.Fatalf("expected AgentSelectedMsg, got %T", msg)
	}
	if sel.Agent != "architect" {
		t.Fatalf("expected 'architect', got %q", sel.Agent)
	}
}

func TestUpdate_ShiftTabCyclesAgentBackward(t *testing.T) {
	p := NewPromptInput(80)
	p.agent = "build"
	p.SetCycleAgents([]string{"build", "architect", "debugger"})

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if cmd == nil {
		t.Fatal("expected cmd from Shift+Tab")
	}
	msg := cmd()
	sel, ok := msg.(AgentSelectedMsg)
	if !ok {
		t.Fatalf("expected AgentSelectedMsg, got %T", msg)
	}
	if sel.Agent != "debugger" {
		t.Fatalf("expected 'debugger', got %q", sel.Agent)
	}
}

func TestUpdate_TabNoCycleAgentsReturnsNil(t *testing.T) {
	p := NewPromptInput(80)
	// No cycle agents set
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		t.Fatal("expected nil cmd when no cycle agents")
	}
}

// ---------------------------------------------------------------------------
// Update — Up/Down navigate history
// ---------------------------------------------------------------------------

func TestUpdate_UpNavigatesHistory(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("older")
	p.history.Add("newer")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p.Value() != "newer" {
		t.Fatalf("expected 'newer', got %q", p.Value())
	}

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p.Value() != "older" {
		t.Fatalf("expected 'older', got %q", p.Value())
	}
}

func TestUpdate_DownNavigatesHistoryForward(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("oldest")
	p.history.Add("newest")

	// Go to oldest first
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Then forward
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p.Value() != "newest" {
		t.Fatalf("expected 'newest', got %q", p.Value())
	}
}

// ---------------------------------------------------------------------------
// Update — Ctrl+S stash/restore
// ---------------------------------------------------------------------------

func TestUpdate_CtrlS_StashesWhenTextPresent(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("in progress")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !p.HasDraft() {
		t.Fatal("expected draft stashed")
	}
	if p.Value() != "" {
		t.Fatalf("expected textarea cleared, got %q", p.Value())
	}
}

func TestUpdate_CtrlS_RestoresWhenEmpty(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("draft text")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlS}) // stash
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlS}) // restore
	if p.Value() != "draft text" {
		t.Fatalf("expected restored 'draft text', got %q", p.Value())
	}
	if p.HasDraft() {
		t.Fatal("expected no draft after restore")
	}
}

// ---------------------------------------------------------------------------
// Update — Ctrl+R toggles history browser
// ---------------------------------------------------------------------------

func TestUpdate_CtrlR_OpensHistoryBrowser(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !p.historyBrowserOn {
		t.Fatal("expected history browser open")
	}
}

func TestUpdate_CtrlR_ClosesHistoryBrowser(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR}) // open
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR}) // close
	if p.historyBrowserOn {
		t.Fatal("expected history browser closed")
	}
}

func TestUpdate_CtrlR_NoopWhenNoHistory(t *testing.T) {
	p := NewPromptInput(80)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if p.historyBrowserOn {
		t.Fatal("expected history browser to stay closed with no entries")
	}
}

// ---------------------------------------------------------------------------
// EnableStartupGuard
// ---------------------------------------------------------------------------

func TestEnableStartupGuard_BlocksRunesDuringGracePeriod(t *testing.T) {
	p := NewPromptInput(80)
	p.EnableStartupGuard()

	// Rune input during the 2-second grace period should be discarded.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if p.Value() != "" {
		t.Fatalf("expected rune 'x' discarded during startup guard, got %q", p.Value())
	}
}

func TestEnableStartupGuard_AllowsRunesAfterGracePeriod(t *testing.T) {
	p := NewPromptInput(80)
	p.EnableStartupGuard()
	// Simulate that the guard expired
	p.startTime = time.Now().Add(-3 * time.Second)

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if p.Value() == "" {
		t.Fatal("expected rune 'x' accepted after guard expired")
	}
}

func TestResetStartupGuard_RestartsTimer(t *testing.T) {
	p := NewPromptInput(80)
	p.EnableStartupGuard()
	p.startTime = time.Now().Add(-3 * time.Second) // expired

	p.ResetStartupGuard()
	// After reset, guard should be active again
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if p.Value() != "" {
		t.Fatalf("expected rune discarded after guard reset, got %q", p.Value())
	}
}

func TestResetStartupGuard_NoopWhenNotEnabled(t *testing.T) {
	p := NewPromptInput(80)
	// Not enabled, ResetStartupGuard should not panic or arm the guard
	p.ResetStartupGuard()
	// Runes should still pass through
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !strings.Contains(p.Value(), "y") {
		t.Fatal("expected rune to pass through when guard not enabled")
	}
}

// ---------------------------------------------------------------------------
// ArmNoiseGuard
// ---------------------------------------------------------------------------

func TestArmNoiseGuard_SetsFutureTime(t *testing.T) {
	p := NewPromptInput(80)
	p.ArmNoiseGuard(100 * time.Millisecond)
	if p.noiseUntil.Before(time.Now()) {
		t.Fatal("expected noiseUntil in the future")
	}
}

func TestArmNoiseGuard_ZeroDurationNoop(t *testing.T) {
	p := NewPromptInput(80)
	p.ArmNoiseGuard(0)
	if !p.noiseUntil.IsZero() {
		t.Fatal("expected noiseUntil to remain zero for 0 duration")
	}
}

func TestArmNoiseGuard_DoesNotShrinkWindow(t *testing.T) {
	p := NewPromptInput(80)
	p.ArmNoiseGuard(5 * time.Second)
	first := p.noiseUntil

	p.ArmNoiseGuard(1 * time.Millisecond) // shorter window
	if p.noiseUntil.Before(first) {
		t.Fatal("expected noiseUntil not to shrink")
	}
}

// ---------------------------------------------------------------------------
// absorbTerminalNoise
// ---------------------------------------------------------------------------

func TestAbsorbTerminalNoise_AbsorbsEscapeSequence(t *testing.T) {
	p := NewPromptInput(80)
	if !p.absorbTerminalNoise("\x1b[0m") {
		t.Fatal("expected ESC sequence to be absorbed")
	}
}

func TestAbsorbTerminalNoise_PassesNormalText(t *testing.T) {
	p := NewPromptInput(80)
	if p.absorbTerminalNoise("hello") {
		t.Fatal("expected normal text to pass through")
	}
}

func TestAbsorbTerminalNoise_AbsorbsCPR(t *testing.T) {
	p := NewPromptInput(80)
	// A CPR-like sequence: "[53;1R"
	if !p.absorbTerminalNoise("[53;1R") {
		t.Fatal("expected CPR sequence to be absorbed")
	}
}

func TestAbsorbTerminalNoise_AbsorbsSplitCPR(t *testing.T) {
	p := NewPromptInput(80)
	// Simulates split delivery: first "[", then "53;1R"
	absorbed1 := p.absorbTerminalNoise("[")
	if !absorbed1 {
		t.Fatal("expected '[' prefix to be absorbed")
	}
	absorbed2 := p.absorbTerminalNoise("53;1R")
	if !absorbed2 {
		t.Fatal("expected '53;1R' completion to be absorbed")
	}
}

func TestAbsorbTerminalNoise_AbsorbsOSCColor(t *testing.T) {
	p := NewPromptInput(80)
	if !p.absorbTerminalNoise("]10;rgb:ff/ff/ff") {
		t.Fatal("expected OSC color sequence to be absorbed")
	}
}

func TestAbsorbTerminalNoise_AbsorbsHexFragment(t *testing.T) {
	p := NewPromptInput(80)
	if !p.absorbTerminalNoise("ff/ff/ff") {
		t.Fatal("expected hex color fragment to be absorbed")
	}
}

// ---------------------------------------------------------------------------
// isTerminalEscape / isTerminalNoise (package-level helpers)
// ---------------------------------------------------------------------------

func TestIsTerminalEscape_TrueForESC(t *testing.T) {
	if !isTerminalEscape("\x1b[0m") {
		t.Fatal("expected true for ESC byte")
	}
}

func TestIsTerminalEscape_TrueForRGB(t *testing.T) {
	if !isTerminalEscape("rgb:ff/00/ff") {
		t.Fatal("expected true for rgb: prefix")
	}
}

func TestIsTerminalEscape_TrueForOSC10(t *testing.T) {
	if !isTerminalEscape("]10;rgb:ff/ff/ff") {
		t.Fatal("expected true for ]10; prefix")
	}
}

func TestIsTerminalEscape_FalseForNormalText(t *testing.T) {
	if isTerminalEscape("hello world") {
		t.Fatal("expected false for normal text")
	}
}

func TestIsTerminalEscape_FalseForEmpty(t *testing.T) {
	if isTerminalEscape("") {
		t.Fatal("expected false for empty string")
	}
}

func TestIsTerminalNoise_TrueForBracket(t *testing.T) {
	if !isTerminalNoise("]") {
		t.Fatal("expected true for lone ']'")
	}
}

func TestIsTerminalNoise_TrueForSemicolonDigitR(t *testing.T) {
	if !isTerminalNoise("53;1R") {
		t.Fatal("expected true for CPR body '53;1R'")
	}
}

// ---------------------------------------------------------------------------
// stripCPRPrefix
// ---------------------------------------------------------------------------

func TestStripCPRPrefix_RemovesBrackets(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"]53;1R", "53;1R"},
		{"\\[53;1R", "53;1R"},
		{"[53;1R", "53;1R"},
		{"]\\[53;1R", "53;1R"},
		{"53;1R", "53;1R"},
	}
	for _, tt := range tests {
		got := stripCPRPrefix(tt.input)
		if got != tt.want {
			t.Errorf("stripCPRPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// scrubCPRValue
// ---------------------------------------------------------------------------

func TestScrubCPRValue_RemovesCPRFromText(t *testing.T) {
	cleaned, changed := scrubCPRValue("hello]53;1Rworld")
	if !changed {
		t.Fatal("expected changed=true")
	}
	if cleaned != "helloworld" {
		t.Fatalf("expected 'helloworld', got %q", cleaned)
	}
}

func TestScrubCPRValue_NoChangeForCleanText(t *testing.T) {
	cleaned, changed := scrubCPRValue("hello world")
	if changed {
		t.Fatal("expected changed=false for clean text")
	}
	if cleaned != "hello world" {
		t.Fatalf("expected 'hello world', got %q", cleaned)
	}
}

func TestScrubCPRValue_EmptyString(t *testing.T) {
	cleaned, changed := scrubCPRValue("")
	if changed {
		t.Fatal("expected changed=false for empty string")
	}
	if cleaned != "" {
		t.Fatalf("expected empty string, got %q", cleaned)
	}
}

func TestScrubCPRValue_FullEscapeReturnsEmpty(t *testing.T) {
	cleaned, changed := scrubCPRValue("\x1b[0m")
	if !changed {
		t.Fatal("expected changed=true for full escape sequence")
	}
	if cleaned != "" {
		t.Fatalf("expected empty string, got %q", cleaned)
	}
}

// ---------------------------------------------------------------------------
// formatImageSize
// ---------------------------------------------------------------------------

func TestFormatImageSize_Bytes(t *testing.T) {
	result := formatImageSize(512)
	if result != "512B PNG" {
		t.Fatalf("expected '512B PNG', got %q", result)
	}
}

func TestFormatImageSize_Kilobytes(t *testing.T) {
	result := formatImageSize(2048)
	if result != "2KB PNG" {
		t.Fatalf("expected '2KB PNG', got %q", result)
	}
}

func TestFormatImageSize_Megabytes(t *testing.T) {
	result := formatImageSize(2 * 1024 * 1024)
	if result != "2.0MB PNG" {
		t.Fatalf("expected '2.0MB PNG', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// renderMetadata
// ---------------------------------------------------------------------------

func TestRenderMetadata_EmptyByDefault(t *testing.T) {
	p := NewPromptInput(80)
	meta := p.renderMetadata()
	if meta != "" {
		t.Fatalf("expected empty metadata by default, got %q", meta)
	}
}

func TestRenderMetadata_ShowsThinkingLevel(t *testing.T) {
	p := NewPromptInput(80)
	p.SetThinkingLevel("high")
	meta := p.renderMetadata()
	if !strings.Contains(meta, "thinking:high") {
		t.Fatalf("expected 'thinking:high' in metadata, got %q", meta)
	}
}

func TestRenderMetadata_HidesThinkingOff(t *testing.T) {
	p := NewPromptInput(80)
	p.SetThinkingLevel("off")
	meta := p.renderMetadata()
	if strings.Contains(meta, "thinking") {
		t.Fatalf("expected no thinking entry for 'off', got %q", meta)
	}
}

func TestRenderMetadata_ShowsEffortLevel(t *testing.T) {
	p := NewPromptInput(80)
	p.SetEffortLevel("high")
	meta := p.renderMetadata()
	if !strings.Contains(meta, "effort:high") {
		t.Fatalf("expected 'effort:high' in metadata, got %q", meta)
	}
}

func TestRenderMetadata_HidesDefaultEffort(t *testing.T) {
	p := NewPromptInput(80)
	p.SetEffortLevel("medium")
	meta := p.renderMetadata()
	if strings.Contains(meta, "effort") {
		t.Fatalf("expected no effort entry for default 'medium', got %q", meta)
	}
}

func TestRenderMetadata_ShowsSingleImage(t *testing.T) {
	p := NewPromptInput(80)
	p.AddImage(2048)
	meta := p.renderMetadata()
	if !strings.Contains(meta, "[image:") {
		t.Fatalf("expected single image indicator, got %q", meta)
	}
}

func TestRenderMetadata_ShowsMultipleImages(t *testing.T) {
	p := NewPromptInput(80)
	p.AddImage(1024)
	p.AddImage(2048)
	meta := p.renderMetadata()
	if !strings.Contains(meta, "[2 images:") {
		t.Fatalf("expected '2 images' indicator, got %q", meta)
	}
}

func TestRenderMetadata_ShowsDraftStashHint(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("draft")
	p.StashDraft()
	meta := p.renderMetadata()
	if !strings.Contains(meta, "stash") {
		t.Fatalf("expected stash hint in metadata, got %q", meta)
	}
}

func TestRenderMetadata_ShowsHistoryHint(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry1")
	p.history.Add("entry2")
	meta := p.renderMetadata()
	if !strings.Contains(meta, "history: 2") {
		t.Fatalf("expected 'history: 2' in metadata, got %q", meta)
	}
}

func TestRenderMetadata_DraftStashTakesPrecedenceOverHistory(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("old prompt")
	p.SetValue("wip")
	p.StashDraft()
	meta := p.renderMetadata()
	// When draft is stashed, stash hint should show, not history hint
	if !strings.Contains(meta, "stash") {
		t.Fatalf("expected stash hint, got %q", meta)
	}
	if strings.Contains(meta, "history:") {
		t.Fatalf("expected no history hint when draft is stashed, got %q", meta)
	}
}

// ---------------------------------------------------------------------------
// View — basic structure
// ---------------------------------------------------------------------------

func TestView_ContainsBorderCharacters(t *testing.T) {
	p := NewPromptInput(80)
	view := p.View()
	if !strings.Contains(view, "┃") {
		t.Fatal("expected left border character in view")
	}
	if !strings.Contains(view, "╹") {
		t.Fatal("expected bottom-left character in view")
	}
}

func TestView_WaveOrRule(t *testing.T) {
	// With wave on: should have braille characters
	p := NewPromptInput(40)
	view := p.View()
	if strings.Contains(view, "─") {
		t.Error("expected braille wave line, not rule, when waveOn")
	}

	// After submit: should have rule
	p.SetValue("submit")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view = p.View()
	if !strings.Contains(view, "─") {
		t.Error("expected quiet rule after wave stops")
	}
}

// ---------------------------------------------------------------------------
// PopoverView
// ---------------------------------------------------------------------------

func TestPopoverView_EmptyByDefault(t *testing.T) {
	p := NewPromptInput(80)
	if p.PopoverView() != "" {
		t.Fatal("expected empty popover view by default")
	}
}

func TestPopoverView_ShowsHistoryBrowser(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("prompt 1")
	p.toggleHistoryBrowser()
	popover := p.PopoverView()
	if popover == "" {
		t.Fatal("expected non-empty popover for history browser")
	}
	if !strings.Contains(popover, "History") {
		t.Fatalf("expected 'History' in popover, got %q", popover)
	}
}

// ---------------------------------------------------------------------------
// toggleHistoryBrowser
// ---------------------------------------------------------------------------

func TestToggleHistoryBrowser_OpensWhenHistoryExists(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry")
	p.toggleHistoryBrowser()
	if !p.historyBrowserOn {
		t.Fatal("expected history browser open")
	}
	// Cursor should point to the most recent entry
	if p.historyBrowserCursor != 0 {
		t.Fatalf("expected cursor at 0 (most recent), got %d", p.historyBrowserCursor)
	}
}

func TestToggleHistoryBrowser_NoopWhenEmpty(t *testing.T) {
	p := NewPromptInput(80)
	p.toggleHistoryBrowser()
	if p.historyBrowserOn {
		t.Fatal("expected browser to stay closed with no history")
	}
}

func TestToggleHistoryBrowser_ClosesWhenOpen(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry")
	p.toggleHistoryBrowser() // open
	p.toggleHistoryBrowser() // close
	if p.historyBrowserOn {
		t.Fatal("expected browser closed after second toggle")
	}
}

// ---------------------------------------------------------------------------
// handleHistoryBrowserKey
// ---------------------------------------------------------------------------

func TestHandleHistoryBrowserKey_EscCloses(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	result, _, handled := p.handleHistoryBrowserKey(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if result.historyBrowserOn {
		t.Fatal("expected browser closed after esc")
	}
}

func TestHandleHistoryBrowserKey_UpMovesUp(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("first")
	p.history.Add("second")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 1

	result, _, handled := p.handleHistoryBrowserKey(tea.KeyMsg{Type: tea.KeyUp})
	if !handled {
		t.Fatal("expected up to be handled")
	}
	if result.historyBrowserCursor != 0 {
		t.Fatalf("expected cursor 0, got %d", result.historyBrowserCursor)
	}
}

func TestHandleHistoryBrowserKey_UpClampsAtZero(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("only")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	result, _, _ := p.handleHistoryBrowserKey(tea.KeyMsg{Type: tea.KeyUp})
	if result.historyBrowserCursor != 0 {
		t.Fatalf("expected cursor clamped at 0, got %d", result.historyBrowserCursor)
	}
}

func TestHandleHistoryBrowserKey_DownMovesDown(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("first")
	p.history.Add("second")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	result, _, handled := p.handleHistoryBrowserKey(tea.KeyMsg{Type: tea.KeyDown})
	if !handled {
		t.Fatal("expected down to be handled")
	}
	if result.historyBrowserCursor != 1 {
		t.Fatalf("expected cursor 1, got %d", result.historyBrowserCursor)
	}
}

func TestHandleHistoryBrowserKey_EnterSelectsEntry(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("first")
	p.history.Add("second")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	result, _, handled := p.handleHistoryBrowserKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected enter to be handled")
	}
	if result.historyBrowserOn {
		t.Fatal("expected browser closed after enter")
	}
	if result.Value() != "first" {
		t.Fatalf("expected 'first' selected, got %q", result.Value())
	}
}

func TestHandleHistoryBrowserKey_UnhandledKeyPassesThrough(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("entry")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	_, _, handled := p.handleHistoryBrowserKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if handled {
		t.Fatal("expected unrelated key to not be handled by history browser")
	}
}

// ---------------------------------------------------------------------------
// handleFileCompleteKey
// ---------------------------------------------------------------------------

func TestHandleFileCompleteKey_EscDismisses(t *testing.T) {
	p := NewPromptInput(80)
	p.fileComplete.visible = true
	p.fileComplete.items = []FileItem{{Path: "file.go"}}

	result, _, handled := p.handleFileCompleteKey(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if result.fileComplete.IsVisible() {
		t.Fatal("expected file completer dismissed after esc")
	}
}

func TestHandleFileCompleteKey_UpDownNavigates(t *testing.T) {
	p := NewPromptInput(80)
	p.fileComplete.visible = true
	p.fileComplete.items = []FileItem{
		{Path: "a.go"},
		{Path: "b.go"},
	}

	result, _, handled := p.handleFileCompleteKey(tea.KeyMsg{Type: tea.KeyDown})
	if !handled {
		t.Fatal("expected down to be handled")
	}
	if result.fileComplete.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", result.fileComplete.cursor)
	}
}

func TestHandleFileCompleteKey_TabSelectsFile(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("@main")
	p.fileComplete.visible = true
	p.fileComplete.items = []FileItem{{Path: "main.go", IsDir: false}}
	p.fileComplete.cursor = 0
	p.fileComplete.tokenStart = 0
	p.fileComplete.tokenEnd = 5

	result, _, handled := p.handleFileCompleteKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("expected tab to be handled")
	}
	val := result.Value()
	if !strings.Contains(val, "main.go") {
		t.Fatalf("expected value to contain 'main.go', got %q", val)
	}
}

func TestHandleFileCompleteKey_TabSelectsDirectory(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("@sr")
	p.fileComplete.visible = true
	p.fileComplete.items = []FileItem{{Path: "src", IsDir: true}}
	p.fileComplete.cursor = 0
	p.fileComplete.tokenStart = 0
	p.fileComplete.tokenEnd = 3

	result, _, handled := p.handleFileCompleteKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("expected tab to be handled")
	}
	val := result.Value()
	if !strings.Contains(val, "@src/") {
		t.Fatalf("expected value to contain '@src/', got %q", val)
	}
}

// ---------------------------------------------------------------------------
// formatSelection
// ---------------------------------------------------------------------------

func TestFormatSelection_SlashMode(t *testing.T) {
	p := NewPromptInput(80)
	p.autocomplete.mode = "/"
	result := p.formatSelection("build")
	if result != "/build " {
		t.Fatalf("expected '/build ', got %q", result)
	}
}

func TestFormatSelection_AskMode(t *testing.T) {
	p := NewPromptInput(80)
	p.autocomplete.mode = "/ask"
	result := p.formatSelection("architect")
	if result != "/ask architect " {
		t.Fatalf("expected '/ask architect ', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// StashDraft / RestoreDraft / HasDraft / DraftValue
// ---------------------------------------------------------------------------

func TestStashDraft_ReturnsFalseWhenEmpty(t *testing.T) {
	p := NewPromptInput(80)
	if p.StashDraft() {
		t.Fatal("expected StashDraft to return false for empty textarea")
	}
}

func TestStashDraft_StoresAndClears(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("my draft")
	if !p.StashDraft() {
		t.Fatal("expected StashDraft to return true")
	}
	if p.Value() != "" {
		t.Fatalf("expected textarea cleared, got %q", p.Value())
	}
	if p.DraftValue() != "my draft" {
		t.Fatalf("expected draft value 'my draft', got %q", p.DraftValue())
	}
}

func TestRestoreDraft_ReturnsFalseWhenNone(t *testing.T) {
	p := NewPromptInput(80)
	if p.RestoreDraft() {
		t.Fatal("expected RestoreDraft to return false with no stash")
	}
}

func TestRestoreDraft_RestoresAndClears(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("parked")
	p.StashDraft()
	if !p.RestoreDraft() {
		t.Fatal("expected RestoreDraft to return true")
	}
	if p.Value() != "parked" {
		t.Fatalf("expected 'parked', got %q", p.Value())
	}
	if p.HasDraft() {
		t.Fatal("expected no draft after restore")
	}
	if p.DraftValue() != "" {
		t.Fatalf("expected empty draft value, got %q", p.DraftValue())
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func TestInit_ReturnsCmd(t *testing.T) {
	p := NewPromptInput(80)
	cmd := p.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a cmd (blink + wave tick)")
	}
}

// ---------------------------------------------------------------------------
// Update — promptWaveTickMsg
// ---------------------------------------------------------------------------

func TestUpdate_WaveTickAdvancesPhaseWhenOn(t *testing.T) {
	p := NewPromptInput(80)
	p, cmd := p.Update(promptWaveTickMsg{})
	if p.wavePhase != 1 {
		t.Fatalf("expected wavePhase 1, got %d", p.wavePhase)
	}
	if cmd == nil {
		t.Fatal("expected tick cmd while wave is on")
	}
}

func TestUpdate_WaveTickIgnoredWhenOff(t *testing.T) {
	p := NewPromptInput(80)
	p.waveOn = false
	p, cmd := p.Update(promptWaveTickMsg{})
	if p.wavePhase != 0 {
		t.Fatalf("expected wavePhase 0 when off, got %d", p.wavePhase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when wave is off")
	}
}

// ---------------------------------------------------------------------------
// Update — FileCompletionMsg
// ---------------------------------------------------------------------------

func TestUpdate_FileCompletionMsgSetsResults(t *testing.T) {
	p := NewPromptInput(80)
	p.fileComplete.query = "main"
	// Set lastQuery to match so SetResults accepts it
	q := "main"
	p.fileComplete.lastQuery = &q

	items := []FileItem{{Path: "main.go", IsDir: false}}
	p, _ = p.Update(FileCompletionMsg{Items: items, Query: "main"})
	if !p.fileComplete.IsVisible() {
		t.Fatal("expected file completer visible after receiving results")
	}
}

// ---------------------------------------------------------------------------
// handleAutocompleteKey
// ---------------------------------------------------------------------------

func TestHandleAutocompleteKey_TabSelectsItem(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCommands(testCommands())
	p.autocomplete.UpdateInput("/bu")

	result, _, handled := p.handleAutocompleteKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("expected tab to be handled")
	}
	if result.Value() != "/build " {
		t.Fatalf("expected '/build ', got %q", result.Value())
	}
}

func TestHandleAutocompleteKey_EscDismisses(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCommands(testCommands())
	p.autocomplete.UpdateInput("/")

	result, _, handled := p.handleAutocompleteKey(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if result.autocomplete.IsVisible() {
		t.Fatal("expected autocomplete dismissed")
	}
}

func TestHandleAutocompleteKey_UpDownNavigates(t *testing.T) {
	p := NewPromptInput(80)
	p.SetCommands(testCommands())
	p.autocomplete.UpdateInput("/")

	result, _, handled := p.handleAutocompleteKey(tea.KeyMsg{Type: tea.KeyDown})
	if !handled {
		t.Fatal("expected down to be handled")
	}
	if result.autocomplete.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", result.autocomplete.cursor)
	}
}

// ---------------------------------------------------------------------------
// historyBrowserView
// ---------------------------------------------------------------------------

func TestHistoryBrowserView_EmptyWhenNoEntries(t *testing.T) {
	p := NewPromptInput(80)
	view := p.historyBrowserView()
	if view != "" {
		t.Fatalf("expected empty view for no entries, got %q", view)
	}
}

func TestHistoryBrowserView_ShowsEntries(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("prompt one")
	p.history.Add("prompt two")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 1

	view := p.historyBrowserView()
	if !strings.Contains(view, "History") {
		t.Fatal("expected 'History' header in browser view")
	}
	if !strings.Contains(view, "prompt one") {
		t.Fatal("expected 'prompt one' in browser view")
	}
	if !strings.Contains(view, "prompt two") {
		t.Fatal("expected 'prompt two' in browser view")
	}
}

func TestHistoryBrowserView_TruncatesLongEntries(t *testing.T) {
	p := NewPromptInput(40) // narrow width
	longEntry := strings.Repeat("x", 200)
	p.history.Add(longEntry)
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	view := p.historyBrowserView()
	// Should contain the truncation ellipsis
	if !strings.Contains(view, "…") {
		t.Fatal("expected truncation ellipsis in narrow view")
	}
}

func TestHistoryBrowserView_ReplacesNewlines(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("line1\nline2")
	p.historyBrowserOn = true
	p.historyBrowserCursor = 0

	view := p.historyBrowserView()
	// Newlines should be replaced with return symbol
	if !strings.Contains(view, "↵") {
		t.Fatal("expected newline replacement symbol in browser view")
	}
}

// ---------------------------------------------------------------------------
// cursorOffset
// ---------------------------------------------------------------------------

func TestCursorOffset_EmptyTextarea(t *testing.T) {
	p := NewPromptInput(80)
	offset := p.cursorOffset()
	if offset != 0 {
		t.Fatalf("expected cursor offset 0 for empty textarea, got %d", offset)
	}
}

// ---------------------------------------------------------------------------
// renderWaveLine
// ---------------------------------------------------------------------------

func TestRenderWaveLine_NonEmpty(t *testing.T) {
	p := NewPromptInput(40)
	line := p.renderWaveLine()
	if line == "" {
		t.Fatal("expected non-empty wave line")
	}
}

func TestRenderWaveLine_MinWidth(t *testing.T) {
	p := NewPromptInput(1)
	p.width = 0 // force minimum
	line := p.renderWaveLine()
	if line == "" {
		t.Fatal("expected non-empty wave line even at zero width")
	}
}
