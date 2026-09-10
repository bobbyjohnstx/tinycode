package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleUserMsg      = lipgloss.NewStyle().Bold(true).PaddingLeft(2)
	styleAssistantMsg = lipgloss.NewStyle().PaddingLeft(2)
	styleToolName     = lipgloss.NewStyle().Bold(true)
	styleSpinner      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"})
	colorError   lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"}
	colorSuccess lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"}
	styleSelected     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"})
	styleMetadata     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	styleDialogBorder = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}).Padding(1, 2)
	stylePromptBorder = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}).BorderTop(true).BorderBottom(false).BorderLeft(false).BorderRight(false)
	stylePromptAccent = lipgloss.NewStyle().BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderTop(false).BorderRight(false).BorderBottom(false).BorderForeground(lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#CC4444"})
	styleStatusBar    = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#1A1A1A"}).Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"}).Padding(0, 1)
)

var agentColorMap = map[string]lipgloss.AdaptiveColor{
	"build":             {Light: "#CC0000", Dark: "#CC4444"},
	"architect":         {Light: "#0055AA", Dark: "#58A6FF"},
	"debugger":          {Light: "#CC6600", Dark: "#FFAA33"},
	"executor":          {Light: "#006600", Dark: "#66FF66"},
	"planner":           {Light: "#8833AA", Dark: "#C882E7"},
	"plan":              {Light: "#8833AA", Dark: "#C882E7"},
	"code-reviewer":     {Light: "#997700", Dark: "#FFD700"},
	"code-simplifier":   {Light: "#997700", Dark: "#FFD700"},
	"test-engineer":     {Light: "#007777", Dark: "#00CED1"},
	"explore":           {Light: "#007755", Dark: "#2ED8A3"},
	"writer":            {Light: "#AA7700", Dark: "#FCB239"},
	"critic":            {Light: "#AA3366", Dark: "#FF6699"},
	"security-reviewer": {Light: "#CC3300", Dark: "#FF6633"},
	"scientist":         {Light: "#335599", Dark: "#6699CC"},
	"git-master":        {Light: "#664400", Dark: "#CC8844"},
	"verifier":          {Light: "#336644", Dark: "#55AA77"},
}

var agentPalette = []lipgloss.AdaptiveColor{
	{Light: "#CC0000", Dark: "#CC4444"},
	{Light: "#0055AA", Dark: "#58A6FF"},
	{Light: "#CC6600", Dark: "#FFAA33"},
	{Light: "#006600", Dark: "#66FF66"},
	{Light: "#8833AA", Dark: "#C882E7"},
	{Light: "#997700", Dark: "#FFD700"},
	{Light: "#007777", Dark: "#00CED1"},
	{Light: "#AA3366", Dark: "#FF6699"},
	{Light: "#335599", Dark: "#6699CC"},
	{Light: "#664400", Dark: "#CC8844"},
	{Light: "#007755", Dark: "#2ED8A3"},
	{Light: "#AA7700", Dark: "#FCB239"},
}

// AgentColor returns the color for a given agent name.
func AgentColor(name string) lipgloss.AdaptiveColor {
	lower := strings.ToLower(name)
	if c, ok := agentColorMap[lower]; ok {
		return c
	}
	var hash uint32
	for _, ch := range lower {
		hash = hash*31 + uint32(ch)
	}
	return agentPalette[hash%uint32(len(agentPalette))]
}

// Theme holds all lipgloss styles for the TUI.
type Theme struct {
	// Message roles
	UserMessage      lipgloss.Style
	AssistantMessage lipgloss.Style

	// Borders and containers
	ChatBorder   lipgloss.Style
	SidebarBox   lipgloss.Style
	PromptBorder lipgloss.Style

	// Status bar
	StatusBar      lipgloss.Style
	StatusBarModel lipgloss.Style
	StatusBarAgent lipgloss.Style
	StatusBarCwd   lipgloss.Style

	// Toast
	ToastInfo  lipgloss.Style
	ToastError lipgloss.Style

	// Dialog
	DialogOverlay lipgloss.Style
	DialogTitle   lipgloss.Style
	DialogItem    lipgloss.Style
	DialogActive  lipgloss.Style

	// Permission
	PermissionBorder lipgloss.Style
	PermissionAllow  lipgloss.Style
	PermissionDeny   lipgloss.Style

	// Palette
	PaletteInput  lipgloss.Style
	PaletteItem   lipgloss.Style
	PaletteActive lipgloss.Style

	// Misc
	Spinner lipgloss.Style
	Dim     lipgloss.Style
	Bold    lipgloss.Style
}

// SetTheme updates the package-level style variables from a Theme,
// connecting the theme system to actual rendering.
func SetTheme(t Theme) {
	styleUserMsg = t.UserMessage
	styleAssistantMsg = t.AssistantMessage
	styleSpinner = t.Spinner
	styleSelected = t.DialogActive
	styleMetadata = t.Dim
	styleDialogBorder = t.DialogOverlay
	stylePromptBorder = t.PromptBorder
	styleStatusBar = t.StatusBar
	colorError = t.ToastError.GetForeground()
	colorSuccess = t.PermissionAllow.GetForeground()
}

type themeColors struct {
	accent     lipgloss.TerminalColor
	user       lipgloss.TerminalColor
	assistant  lipgloss.TerminalColor
	errorC     lipgloss.TerminalColor
	success    lipgloss.TerminalColor
	subtle     lipgloss.TerminalColor
	bg         lipgloss.TerminalColor
	highlight  lipgloss.TerminalColor
	permBorder lipgloss.TerminalColor
}

func buildTheme(c themeColors) Theme {
	return Theme{
		UserMessage:      lipgloss.NewStyle().Foreground(c.user).Bold(true).PaddingLeft(2),
		AssistantMessage: lipgloss.NewStyle().Foreground(c.assistant).PaddingLeft(2),
		ChatBorder:       lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(c.subtle),
		SidebarBox:       lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(c.subtle).Padding(0, 1),
		PromptBorder:     lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(c.accent).BorderTop(true),
		StatusBar:        lipgloss.NewStyle().Background(c.bg).Foreground(c.subtle).Padding(0, 1),
		StatusBarModel:   lipgloss.NewStyle().Foreground(c.accent).Bold(true),
		StatusBarAgent:   lipgloss.NewStyle().Foreground(c.success),
		StatusBarCwd:     lipgloss.NewStyle().Foreground(c.subtle).Italic(true),
		ToastInfo:        lipgloss.NewStyle().Foreground(c.accent).Padding(0, 1),
		ToastError:       lipgloss.NewStyle().Foreground(c.errorC).Padding(0, 1),
		DialogOverlay:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(c.accent).Padding(1, 2),
		DialogTitle:      lipgloss.NewStyle().Foreground(c.accent).Bold(true).MarginBottom(1),
		DialogItem:       lipgloss.NewStyle().PaddingLeft(2),
		DialogActive:     lipgloss.NewStyle().Foreground(c.accent).Bold(true).PaddingLeft(2),
		PermissionBorder: lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(c.permBorder),
		PermissionAllow:  lipgloss.NewStyle().Foreground(c.success).Bold(true),
		PermissionDeny:   lipgloss.NewStyle().Foreground(c.errorC).Bold(true),
		PaletteInput:     lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(c.accent).Padding(0, 1),
		PaletteItem:      lipgloss.NewStyle().PaddingLeft(2),
		PaletteActive:    lipgloss.NewStyle().Background(c.highlight).Foreground(c.accent).Bold(true).PaddingLeft(2),
		Spinner:          lipgloss.NewStyle().Foreground(c.accent),
		Dim:              lipgloss.NewStyle().Foreground(c.subtle),
		Bold:             lipgloss.NewStyle().Bold(true),
	}
}

// DefaultTheme returns the default TUI theme.
func DefaultTheme() Theme {
	return buildTheme(themeColors{
		accent:     lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"},
		user:       lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#E1E1E1"},
		assistant:  lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"},
		errorC:     lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"},
		success:    lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"},
		subtle:     lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"},
		bg:         lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#1A1A1A"},
		highlight:  lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"},
		permBorder: lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFAA33"},
	})
}
