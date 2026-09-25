package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestLipglossHeightRendering verifies how lipgloss Height/Width padding
// affects the line count and whether leading newlines are introduced.
func TestLipglossHeightRendering(t *testing.T) {
	content := "line0\nline1\nline2\nline3\nline4"

	// Without any style
	rawLines := strings.Split(content, "\n")
	t.Logf("raw content: %d lines", len(rawLines))

	// With Height
	styled := lipgloss.NewStyle().Width(20).Height(10).Render(content)
	styledLines := strings.Split(styled, "\n")
	t.Logf("Width(20).Height(10).Render(5 lines): %d lines", len(styledLines))
	for i, l := range styledLines {
		t.Logf("  line[%d] = %q", i, l)
	}

	// Check for leading empty line
	if styledLines[0] == "" || strings.TrimSpace(styledLines[0]) == "" {
		t.Log("LEADING EMPTY LINE detected in Height-styled output")
	}

	// Width only
	wOnly := lipgloss.NewStyle().Width(20).Render(content)
	wLines := strings.Split(wOnly, "\n")
	t.Logf("Width(20).Render(5 lines): %d lines", len(wLines))

	// Height only
	hOnly := lipgloss.NewStyle().Height(10).Render(content)
	hLines := strings.Split(hOnly, "\n")
	t.Logf("Height(10).Render(5 lines): %d lines", len(hLines))

	// JoinVertical behavior
	block1 := lipgloss.NewStyle().Width(20).Height(5).Render("a\nb\nc\nd\ne")
	block2 := "prompt"
	block3 := "status"
	joined := lipgloss.JoinVertical(lipgloss.Left, block1, block2, block3)
	joinedLines := strings.Split(joined, "\n")
	t.Logf("JoinVertical (5-line block + prompt + status): %d lines", len(joinedLines))
	for i, l := range joinedLines {
		t.Logf("  line[%d] = %q", i, l)
	}

	// JoinHorizontal behavior
	left := lipgloss.NewStyle().Width(20).Height(5).Render("a\nb\nc\nd\ne")
	right := lipgloss.NewStyle().Width(10).Height(5).Render("1\n2\n3\n4\n5")
	horiz := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	horizLines := strings.Split(horiz, "\n")
	t.Logf("JoinHorizontal: %d lines", len(horizLines))
	for i, l := range horizLines {
		t.Logf("  line[%d] = %q", i, l)
	}

	// Compare: same content through Height vs JoinHorizontal
	t.Logf("\n--- Comparing no-sidebar vs sidebar rendering ---")
	noSidebar := lipgloss.NewStyle().Width(80).Height(5).Render("a\nb\nc\nd\ne")
	noSidebarLines := strings.Split(noSidebar, "\n")
	t.Logf("No sidebar (Width(80).Height(5)): %d lines, first=%q", len(noSidebarLines), noSidebarLines[0])

	sidebarChat := lipgloss.NewStyle().Width(50).Height(5).Render("a\nb\nc\nd\ne")
	sidebarPanel := lipgloss.NewStyle().Width(30).Height(5).Render("x\ny\nz")
	withSidebar := lipgloss.JoinHorizontal(lipgloss.Top, sidebarChat, sidebarPanel)
	withSidebarLines := strings.Split(withSidebar, "\n")
	t.Logf("With sidebar (JoinHorizontal): %d lines, first=%q", len(withSidebarLines), withSidebarLines[0])

	_ = fmt.Sprintf("prevent unused import")
}
