package tui

// Lipgloss styles for the TUI.
// NOTE: The foundation layer (styles.go) is being created by another agent.
// These definitions should be consolidated with the canonical versions once available.

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7C3AED")
	colorSecondary = lipgloss.Color("#6B7280")
	colorMuted     = lipgloss.Color("#4B5563")
	colorError     = lipgloss.Color("#EF4444")
	colorSuccess   = lipgloss.Color("#10B981")
	colorWarning   = lipgloss.Color("#F59E0B")
	colorUser      = lipgloss.Color("#3B82F6")
	colorAssistant = lipgloss.Color("#8B5CF6")

	// Base styles
	styleApp = lipgloss.NewStyle()

	styleUserMsg = lipgloss.NewStyle().
			Foreground(colorUser).
			Bold(true)

	styleAssistantMsg = lipgloss.NewStyle().
				Foreground(colorAssistant)

	styleToolName = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Italic(true)

	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Foreground(lipgloss.Color("#D1D5DB")).
			Padding(0, 1)

	styleMetadata = lipgloss.NewStyle().
			Foreground(colorMuted)

	stylePromptBorder = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorSecondary)

	styleDialogBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(1, 2)

	styleSelected = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	styleSpinner = lipgloss.NewStyle().
			Foreground(colorWarning)
)
