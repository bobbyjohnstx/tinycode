package tui

import "github.com/charmbracelet/lipgloss"

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

// DefaultTheme returns the default TUI theme.
func DefaultTheme() Theme {
	subtle := lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"}
	accent := lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}
	userColor := lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#E1E1E1"}
	assistantColor := lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"}
	errorColor := lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"}
	successColor := lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"}
	bgDim := lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#1A1A1A"}
	bgHighlight := lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}

	return Theme{
		UserMessage: lipgloss.NewStyle().
			Foreground(userColor).
			Bold(true).
			PaddingLeft(2),

		AssistantMessage: lipgloss.NewStyle().
			Foreground(assistantColor).
			PaddingLeft(2),

		ChatBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(subtle),

		SidebarBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(subtle).
			Padding(0, 1),

		PromptBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(accent).
			BorderTop(true),

		StatusBar: lipgloss.NewStyle().
			Background(bgDim).
			Foreground(subtle).
			Padding(0, 1),

		StatusBarModel: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),

		StatusBarAgent: lipgloss.NewStyle().
			Foreground(successColor),

		StatusBarCwd: lipgloss.NewStyle().
			Foreground(subtle).
			Italic(true),

		ToastInfo: lipgloss.NewStyle().
			Foreground(accent).
			Padding(0, 1),

		ToastError: lipgloss.NewStyle().
			Foreground(errorColor).
			Padding(0, 1),

		DialogOverlay: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2),

		DialogTitle: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			MarginBottom(1),

		DialogItem: lipgloss.NewStyle().
			PaddingLeft(2),

		DialogActive: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			PaddingLeft(2),

		PermissionBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFAA33"}),

		PermissionAllow: lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true),

		PermissionDeny: lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true),

		PaletteInput: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(accent).
			Padding(0, 1),

		PaletteItem: lipgloss.NewStyle().
			PaddingLeft(2),

		PaletteActive: lipgloss.NewStyle().
			Background(bgHighlight).
			Foreground(accent).
			Bold(true).
			PaddingLeft(2),

		Spinner: lipgloss.NewStyle().
			Foreground(accent),

		Dim: lipgloss.NewStyle().
			Foreground(subtle),

		Bold: lipgloss.NewStyle().
			Bold(true),
	}
}
