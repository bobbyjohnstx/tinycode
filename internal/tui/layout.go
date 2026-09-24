package tui

import "github.com/charmbracelet/lipgloss"

const (
	statusBarHeight = 2 // hints line + status bar
	promptHeight    = 6 // spacer (1) + textarea (3 lines) + metadata (1) + bottom border ╹▀▀▀ (1)
	minSidebarWidth = 30
	sidebarThreshold = 120
)

// layout holds calculated dimensions for the TUI components.
type layout struct {
	chatWidth    int
	chatHeight   int
	promptWidth  int
	sidebarWidth int
	statusWidth  int
	totalWidth   int
	totalHeight  int
	hasSidebar   bool
}

// calculateLayout computes component dimensions from terminal size.
func calculateLayout(width, height int, sidebarOpen bool) layout {
	l := layout{
		totalWidth:  width,
		totalHeight: height,
		statusWidth: width,
	}

	// Sidebar: show when wide enough and enabled.
	if sidebarOpen && width >= sidebarThreshold {
		l.hasSidebar = true
		l.sidebarWidth = minSidebarWidth
		l.chatWidth = width - l.sidebarWidth
		l.promptWidth = width - l.sidebarWidth
	} else {
		l.chatWidth = width
		l.promptWidth = width
	}

	// Chat area fills remaining vertical space.
	l.chatHeight = height - promptHeight - statusBarHeight
	if l.chatHeight < 1 {
		l.chatHeight = 1
	}

	return l
}

// composeView assembles the full TUI view from component views.
func composeView(chat, prompt, status, sidebar string, l layout) string {
	var mainArea string
	if l.hasSidebar {
		mainArea = lipgloss.JoinHorizontal(
			lipgloss.Top,
			lipgloss.NewStyle().Width(l.chatWidth).Height(l.chatHeight).Render(chat),
			lipgloss.NewStyle().Width(l.sidebarWidth).Height(l.chatHeight).Render(sidebar),
		)
	} else {
		mainArea = lipgloss.NewStyle().Width(l.chatWidth).Height(l.chatHeight).Render(chat)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		mainArea,
		prompt,
		status,
	)
}
