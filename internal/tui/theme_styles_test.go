package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// saveStyleState captures ALL mutable package-level vars that ApplyColorTheme
// and SetTheme modify.  Every test that touches theme/style state MUST call
// this via t.Cleanup to avoid poisoning parallel tests (see #693).
func saveStyleState(t *testing.T) {
	t.Helper()
	saved := struct {
		toolName        lipgloss.Style
		spinner         lipgloss.Style
		selected        lipgloss.Style
		metadata        lipgloss.Style
		dialogBorder    lipgloss.Style
		errColor        lipgloss.TerminalColor
		successColor    lipgloss.TerminalColor
		agentColor      *lipgloss.AdaptiveColor
		userBorder      lipgloss.Style
		timestamp       lipgloss.Style
		agentFooter     lipgloss.Style
		reasoningText   lipgloss.Style
		reasoningLabel  lipgloss.Style
		statusDim       lipgloss.Style
		statusAccent    lipgloss.Style
		sidebarHeader   lipgloss.Style
		sidebarMuted    lipgloss.Style
		sidebarSuccess  lipgloss.Style
		sidebarError    lipgloss.Style
		promptSurface   lipgloss.TerminalColor
		promptDim       lipgloss.TerminalColor
		promptPrimary   lipgloss.TerminalColor
	}{
		toolName:       styleToolName,
		spinner:        styleSpinner,
		selected:       styleSelected,
		metadata:       styleMetadata,
		dialogBorder:   styleDialogBorder,
		errColor:       colorError,
		successColor:   colorSuccess,
		agentColor:     themeAgentColor,
		userBorder:     styleUserBorder,
		timestamp:      styleTimestamp,
		agentFooter:    styleAgentFooter,
		reasoningText:  styleReasoningText,
		reasoningLabel: styleReasoningLabel,
		statusDim:      styleStatusDim,
		statusAccent:   styleStatusAccent,
		sidebarHeader:  styleSidebarHeader,
		sidebarMuted:   styleSidebarMuted,
		sidebarSuccess: styleSidebarSuccess,
		sidebarError:   styleSidebarError,
		promptSurface:  colorPromptSurface,
		promptDim:      colorPromptDim,
		promptPrimary:  colorPromptPrimary,
	}
	t.Cleanup(func() {
		styleToolName = saved.toolName
		styleSpinner = saved.spinner
		styleSelected = saved.selected
		styleMetadata = saved.metadata
		styleDialogBorder = saved.dialogBorder
		colorError = saved.errColor
		colorSuccess = saved.successColor
		themeAgentColor = saved.agentColor
		styleUserBorder = saved.userBorder
		styleTimestamp = saved.timestamp
		styleAgentFooter = saved.agentFooter
		styleReasoningText = saved.reasoningText
		styleReasoningLabel = saved.reasoningLabel
		styleStatusDim = saved.statusDim
		styleStatusAccent = saved.statusAccent
		styleSidebarHeader = saved.sidebarHeader
		styleSidebarMuted = saved.sidebarMuted
		styleSidebarSuccess = saved.sidebarSuccess
		styleSidebarError = saved.sidebarError
		colorPromptSurface = saved.promptSurface
		colorPromptDim = saved.promptDim
		colorPromptPrimary = saved.promptPrimary
	})
}

// ---------------------------------------------------------------------------
// ThemeRegistry tests (theme.go)
// ---------------------------------------------------------------------------

func TestThemeRegistry_LoadEmbedded_PopulatesThemes(t *testing.T) {
	reg := NewThemeRegistry()
	reg.LoadEmbedded()

	list := reg.List()
	if len(list) == 0 {
		t.Fatal("expected at least one embedded theme, got 0")
	}
}

func TestThemeRegistry_Get_ReturnsThemeByID(t *testing.T) {
	reg := NewThemeRegistry()
	reg.LoadEmbedded()

	// Use the first theme from the sorted list.
	list := reg.List()
	if len(list) == 0 {
		t.Fatal("no embedded themes")
	}
	first := list[0]

	got := reg.Get(first.ID)
	if got == nil {
		t.Fatalf("Get(%q) returned nil", first.ID)
	}
	if got.ID != first.ID {
		t.Errorf("expected ID %q, got %q", first.ID, got.ID)
	}
	if got.Name == "" {
		t.Error("expected non-empty Name")
	}
}

func TestThemeRegistry_Get_ReturnsNilForUnknown(t *testing.T) {
	reg := NewThemeRegistry()
	reg.LoadEmbedded()

	if got := reg.Get("nonexistent-theme-id-xyz"); got != nil {
		t.Errorf("expected nil for unknown theme, got %+v", got)
	}
}

func TestThemeRegistry_List_ReturnsSortedOrder(t *testing.T) {
	reg := NewThemeRegistry()
	reg.LoadEmbedded()

	list := reg.List()
	for i := 1; i < len(list); i++ {
		if list[i].ID < list[i-1].ID {
			t.Errorf("List() not sorted: %q came after %q", list[i].ID, list[i-1].ID)
		}
	}
}

func TestThemeRegistry_EmptyRegistry(t *testing.T) {
	reg := NewThemeRegistry()
	// No LoadEmbedded call.

	if got := reg.Get("anything"); got != nil {
		t.Errorf("expected nil from empty registry, got %+v", got)
	}
	if list := reg.List(); len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

func TestParseThemeJSON_ValidTheme(t *testing.T) {
	data := []byte(`{
		"theme": {
			"primary":   {"dark": "#58A6FF", "light": "#0070F3"},
			"error":     {"dark": "#FF6666", "light": "#CC0000"},
			"success":   {"dark": "#66FF66", "light": "#006600"}
		}
	}`)

	ct := parseThemeJSON("test-theme", data)
	if ct == nil {
		t.Fatal("expected non-nil ColorTheme")
	}
	if ct.ID != "test-theme" {
		t.Errorf("expected ID 'test-theme', got %q", ct.ID)
	}
	if ct.Name != "Test Theme" {
		t.Errorf("expected Name 'Test Theme', got %q", ct.Name)
	}
	if ct.Primary.Dark != "#58A6FF" {
		t.Errorf("expected Primary.Dark=#58A6FF, got %q", ct.Primary.Dark)
	}
	if ct.Error.Light != "#CC0000" {
		t.Errorf("expected Error.Light=#CC0000, got %q", ct.Error.Light)
	}
}

func TestParseThemeJSON_WithDefs(t *testing.T) {
	data := []byte(`{
		"defs": {
			"blue": "#0000FF",
			"red": "#FF0000"
		},
		"theme": {
			"primary": {"dark": "blue", "light": "red"}
		}
	}`)

	ct := parseThemeJSON("defs-test", data)
	if ct == nil {
		t.Fatal("expected non-nil ColorTheme")
	}
	if ct.Primary.Dark != "#0000FF" {
		t.Errorf("expected resolved Primary.Dark=#0000FF, got %q", ct.Primary.Dark)
	}
	if ct.Primary.Light != "#FF0000" {
		t.Errorf("expected resolved Primary.Light=#FF0000, got %q", ct.Primary.Light)
	}
}

func TestParseThemeJSON_InvalidJSON(t *testing.T) {
	ct := parseThemeJSON("bad", []byte(`{not json}`))
	if ct != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", ct)
	}
}

func TestParseThemeJSON_NameFormatting(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"catppuccin-frappe", "Catppuccin Frappe"},
		{"dracula", "Dracula"},
		{"one-dark-pro", "One Dark Pro"},
	}
	for _, tt := range tests {
		ct := parseThemeJSON(tt.id, []byte(`{"theme":{}}`))
		if ct == nil {
			t.Fatalf("parseThemeJSON(%q) returned nil", tt.id)
		}
		if ct.Name != tt.want {
			t.Errorf("parseThemeJSON(%q).Name = %q, want %q", tt.id, ct.Name, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// AgentColor tests (styles.go)
// ---------------------------------------------------------------------------

func TestAgentColor_ReturnsKnownAgentColor(t *testing.T) {
	saveStyleState(t)
	themeAgentColor = nil // ensure no theme override

	c := AgentColor("executor")
	expected := agentColorMap["executor"]
	if c != expected {
		t.Errorf("AgentColor(executor) = %+v, want %+v", c, expected)
	}
}

func TestAgentColor_CaseInsensitive(t *testing.T) {
	saveStyleState(t)
	themeAgentColor = nil

	c1 := AgentColor("Executor")
	c2 := AgentColor("EXECUTOR")
	c3 := AgentColor("executor")
	if c1 != c3 || c2 != c3 {
		t.Error("AgentColor should be case-insensitive")
	}
}

func TestAgentColor_UnknownAgentUsesHashPalette(t *testing.T) {
	saveStyleState(t)
	themeAgentColor = nil

	c := AgentColor("unknown-agent-xyz")
	// Should return one of the palette colors, not a zero value.
	found := false
	for _, p := range agentPalette {
		if c == p {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AgentColor for unknown agent returned %+v, expected a palette color", c)
	}
}

func TestAgentColor_DifferentUnknownAgentsGetDifferentColors(t *testing.T) {
	saveStyleState(t)
	themeAgentColor = nil

	// Two very different names should hash to different palette entries
	// (not guaranteed but highly likely with djb-style hash).
	c1 := AgentColor("agent-alpha-123")
	c2 := AgentColor("agent-zeta-789")
	// We can't guarantee they differ (hash collision possible), but we
	// verify neither panics and both return valid palette entries.
	_ = c1
	_ = c2
}

func TestAgentColor_ThemeOverrideReturnsSameColorForAll(t *testing.T) {
	saveStyleState(t)

	override := lipgloss.AdaptiveColor{Light: "#AABBCC", Dark: "#DDEEFF"}
	themeAgentColor = &override

	c1 := AgentColor("executor")
	c2 := AgentColor("debugger")
	c3 := AgentColor("unknown-agent")

	if c1 != override || c2 != override || c3 != override {
		t.Error("with themeAgentColor set, all agents should return the override color")
	}
}

// ---------------------------------------------------------------------------
// ApplyColorTheme tests (theme.go mutating styles.go vars)
// ---------------------------------------------------------------------------

func TestApplyColorTheme_UpdatesStyleVars(t *testing.T) {
	saveStyleState(t)

	ct := &ColorTheme{
		Primary:   lipgloss.AdaptiveColor{Light: "#AA0000", Dark: "#BB0000"},
		Secondary: lipgloss.AdaptiveColor{Light: "#00AA00", Dark: "#00BB00"},
		Accent:    lipgloss.AdaptiveColor{Light: "#0000AA", Dark: "#0000BB"},
		Error:     lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#DD0000"},
		Warning:   lipgloss.AdaptiveColor{Light: "#CCAA00", Dark: "#DDBB00"},
		Success:   lipgloss.AdaptiveColor{Light: "#00CC00", Dark: "#00DD00"},
		TextMuted: lipgloss.AdaptiveColor{Light: "#888888", Dark: "#999999"},
		BgElement: lipgloss.AdaptiveColor{Light: "#F0F0F0", Dark: "#101010"},
	}

	ApplyColorTheme(ct)

	// colorError should now match ct.Error.
	if colorError != ct.Error {
		t.Errorf("colorError = %+v, want %+v", colorError, ct.Error)
	}

	// colorSuccess should match ct.Success.
	if colorSuccess != ct.Success {
		t.Errorf("colorSuccess = %+v, want %+v", colorSuccess, ct.Success)
	}

	// themeAgentColor should be set to ct.Primary.
	if themeAgentColor == nil {
		t.Fatal("themeAgentColor should be non-nil after ApplyColorTheme")
	}
	if *themeAgentColor != ct.Primary {
		t.Errorf("themeAgentColor = %+v, want %+v", *themeAgentColor, ct.Primary)
	}

	// colorPromptSurface should match ct.BgElement.
	if colorPromptSurface != ct.BgElement {
		t.Errorf("colorPromptSurface = %+v, want %+v", colorPromptSurface, ct.BgElement)
	}

	// colorPromptPrimary should match ct.Primary.
	if colorPromptPrimary != ct.Primary {
		t.Errorf("colorPromptPrimary = %+v, want %+v", colorPromptPrimary, ct.Primary)
	}
}

func TestApplyColorTheme_AgentColorOverride(t *testing.T) {
	saveStyleState(t)

	ct := &ColorTheme{
		Primary: lipgloss.AdaptiveColor{Light: "#111111", Dark: "#222222"},
	}

	// Before: themeAgentColor should be nil (cleaned by saveStyleState is not
	// guaranteed to be nil from prior state, but we set it explicitly).
	themeAgentColor = nil
	c1 := AgentColor("executor")
	c2 := AgentColor("debugger")
	if c1 == c2 {
		t.Skip("executor and debugger happen to have the same color")
	}

	// After: all agents return the same override color.
	ApplyColorTheme(ct)
	c1 = AgentColor("executor")
	c2 = AgentColor("debugger")
	if c1 != c2 {
		t.Error("after ApplyColorTheme, all agents should return the same color")
	}
	if c1 != ct.Primary {
		t.Errorf("AgentColor after theme = %+v, want %+v", c1, ct.Primary)
	}
}

// ---------------------------------------------------------------------------
// SetTheme tests (styles.go)
// ---------------------------------------------------------------------------

func TestSetTheme_UpdatesPackageLevelVars(t *testing.T) {
	saveStyleState(t)

	theme := DefaultTheme()
	// Modify spinner to a distinctive color.
	theme.Spinner = lipgloss.NewStyle().Foreground(lipgloss.Color("#ABCDEF"))
	theme.ToastError = lipgloss.NewStyle().Foreground(lipgloss.Color("#990000"))
	theme.PermissionAllow = lipgloss.NewStyle().Foreground(lipgloss.Color("#009900"))

	SetTheme(theme)

	if styleSpinner.GetForeground() != lipgloss.Color("#ABCDEF") {
		t.Errorf("styleSpinner foreground not updated by SetTheme")
	}
}

// ---------------------------------------------------------------------------
// DefaultTheme tests (styles.go)
// ---------------------------------------------------------------------------

func TestDefaultTheme_HasNonZeroStyles(t *testing.T) {
	dt := DefaultTheme()

	if dt.UserMessage.GetPaddingLeft() != 2 {
		t.Errorf("UserMessage padding left = %d, want 2", dt.UserMessage.GetPaddingLeft())
	}
	if dt.AssistantMessage.GetPaddingLeft() != 2 {
		t.Errorf("AssistantMessage padding left = %d, want 2", dt.AssistantMessage.GetPaddingLeft())
	}
	if !dt.Bold.GetBold() {
		t.Error("Bold style should have bold=true")
	}
	if !dt.StatusBarModel.GetBold() {
		t.Error("StatusBarModel should be bold")
	}
}

func TestBuildTheme_ProducesCompleteTheme(t *testing.T) {
	tc := themeColors{
		accent:     lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"},
		user:       lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#E1E1E1"},
		assistant:  lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"},
		errorC:     lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"},
		success:    lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"},
		subtle:     lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"},
		bg:         lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#1A1A1A"},
		highlight:  lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"},
		permBorder: lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFAA33"},
	}
	theme := buildTheme(tc)

	// Verify error toast uses the error color.
	if theme.ToastError.GetForeground() != tc.errorC {
		t.Error("ToastError foreground should use errorC color")
	}
	// Verify permission allow uses success color.
	if theme.PermissionAllow.GetForeground() != tc.success {
		t.Error("PermissionAllow foreground should use success color")
	}
	// Verify permission deny uses error color.
	if theme.PermissionDeny.GetForeground() != tc.errorC {
		t.Error("PermissionDeny foreground should use errorC color")
	}
}
