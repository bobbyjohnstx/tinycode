package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChatView_ClickCopy(t *testing.T) {
	cv := NewChatView(80, 40)
	cv.messages = []MessageView{{
		Info:  MessageInfo{ID: "m1", Role: "assistant", Agent: "build", ModelID: "m"},
		Parts: []PartView{{Type: "text", Text: "copy me please"}},
	}}
	cv.rebuildContent()
	if len(cv.copyLines) != 1 {
		t.Fatalf("expected 1 copy hit line, got %d (content=%q)", len(cv.copyLines), cv.viewport.View())
	}
	var line int
	var text string
	for ln, ttext := range cv.copyLines {
		line = ln
		text = ttext
	}
	if text != "copy me please" {
		t.Fatalf("copy text = %q, want %q", text, "copy me please")
	}
	y := line - cv.viewport.YOffset
	if y < 0 {
		t.Fatalf("copy line %d not visible (yOffset=%d)", line, cv.viewport.YOffset)
	}
	_, cmd := cv.Update(tea.MouseMsg{
		X: 1, Y: y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if cmd == nil {
		t.Fatal("expected clipboard cmd from copy click")
	}
	msg := cmd()
	if _, ok := msg.(CopiedToClipboardMsg); !ok {
		t.Fatalf("expected CopiedToClipboardMsg, got %T", msg)
	}
}

func TestSelectionCoordinates_NoScroll(t *testing.T) {
	cv := NewChatView(80, 20)

	// Add 10 lines of known content
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("x", 80)
	}
	cv.viewport.SetContent(strings.Join(lines, "\n"))

	// Simulate click at Y=3 (should select content line 3)
	cv, _ = cv.Update(tea.MouseMsg{
		X: 5, Y: 3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if cv.dragStart[0] != 3 {
		t.Errorf("expected dragStart line 3, got %d", cv.dragStart[0])
	}

	// Release at same line
	cv, _ = cv.Update(tea.MouseMsg{
		X: 20, Y: 3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	if cv.dragEnd[0] != 3 {
		t.Errorf("expected dragEnd line 3, got %d", cv.dragEnd[0])
	}
}

func TestSelectionCoordinates_WithScroll(t *testing.T) {
	cv := NewChatView(80, 10) // 10 line viewport

	// Add 30 lines so we can scroll
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = strings.Repeat("x", 80)
	}
	cv.viewport.SetContent(strings.Join(lines, "\n"))

	// Scroll down by 15 lines
	cv.viewport.SetYOffset(15)

	// Click at Y=5 on screen → should map to content line 20 (15+5)
	cv, _ = cv.Update(tea.MouseMsg{
		X: 5, Y: 5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if cv.dragStart[0] != 20 {
		t.Errorf("expected dragStart line 20 (offset 15 + Y 5), got %d", cv.dragStart[0])
	}
}

func TestSelectionHighlight_CorrectLine(t *testing.T) {
	cv := NewChatView(80, 10)

	lines := []string{
		"Line zero content here",
		"Line one content here",
		"Line two content here",
		"Line three TARGET LINE",
		"Line four content here",
	}
	cv.viewport.SetContent(strings.Join(lines, "\n"))

	// Select line 3
	cv, _ = cv.Update(tea.MouseMsg{
		X: 0, Y: 3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	cv.dragEnd = [2]int{3, 22}
	cv.dragging = true

	view := cv.View()
	viewLines := strings.Split(view, "\n")

	// The highlight (reverse video \x1b[7m) should be on the line containing "TARGET"
	foundHighlightLine := -1
	for i, vl := range viewLines {
		if strings.Contains(vl, "\x1b[7m") {
			foundHighlightLine = i
			break
		}
	}

	if foundHighlightLine == -1 {
		t.Fatal("no highlight found in view output")
	}

	// Check that the highlighted line contains "TARGET"
	if !strings.Contains(stripAnsi(viewLines[foundHighlightLine]), "TARGET") {
		t.Errorf("highlight on line %d but TARGET is elsewhere. Highlighted: %q",
			foundHighlightLine, stripAnsi(viewLines[foundHighlightLine]))
	}
}
