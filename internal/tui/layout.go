package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	defaultStatusBarHeight = 2 // idle: hints + status
	defaultPromptHeight    = 5 // top rule + textarea(3) + bottom (meta often empty after dedupe)
	minSidebarWidth        = 28
	sidebarThreshold       = 120
)

// layout holds calculated dimensions for the TUI components.
type layout struct {
	chatWidth    int
	chatHeight   int
	promptWidth  int
	promptHeight int
	sidebarWidth int
	statusWidth  int
	statusHeight int
	totalWidth   int
	totalHeight  int
	hasSidebar   bool
}

// calculateLayout computes component dimensions from terminal size.
// Optional trailing heights: promptHeight, statusHeight (in that order).
func calculateLayout(width, height int, sidebarOpen bool, heights ...int) layout {
	pH := defaultPromptHeight
	sH := defaultStatusBarHeight
	if len(heights) >= 1 && heights[0] > 0 {
		pH = heights[0]
	}
	if len(heights) >= 2 && heights[1] > 0 {
		sH = heights[1]
	}

	l := layout{
		totalWidth:   width,
		totalHeight:  height,
		statusWidth:  width,
		promptHeight: pH,
		statusHeight: sH,
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
	l.chatHeight = height - pH - sH
	if l.chatHeight < 1 {
		l.chatHeight = 1
	}

	return l
}

// composeView assembles the full TUI view from component views.
// Every region is width- and height-clamped so resize reflows without wrap ghosts.
func composeView(chat, prompt, status, sidebar string, l layout) string {
	var mainArea string
	if l.hasSidebar {
		mainArea = lipgloss.JoinHorizontal(
			lipgloss.Top,
			clampBlock(sidebar, l.sidebarWidth, l.chatHeight),
			clampBlock(chat, l.chatWidth, l.chatHeight),
		)
	} else {
		mainArea = clampBlock(chat, l.chatWidth, l.chatHeight)
	}

	prompt = clampBlock(prompt, l.promptWidth, l.promptHeight)
	status = clampBlock(status, l.statusWidth, l.statusHeight)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		mainArea,
		prompt,
		status,
	)
}

// clampBlock forces content into exactly width×height cells (truncates wrap/overflow).
func clampBlock(content string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height).
		MaxWidth(width).
		Render(content)
}

// fitTerminal pads or truncates a full frame to the terminal size.
// Prevents alt-screen ghost rows when chrome height changes or lines wrap.
func fitTerminal(view string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, ln := range lines {
		lines[i] = lipgloss.NewStyle().Width(width).MaxWidth(width).MaxHeight(1).Render(ln)
	}
	return strings.Join(lines, "\n")
}
