package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DiffLine represents a single line in a unified diff.
type DiffLine struct {
	Type    DiffLineType
	Content string
}

// DiffLineType categorizes a diff line.
type DiffLineType int

const (
	DiffContext  DiffLineType = iota // unchanged line
	DiffAdded                       // added line
	DiffRemoved                     // removed line
	DiffHeader                      // diff header / hunk header
)

// DiffView renders a scrollable unified diff with color-coded lines.
type DiffView struct {
	lines    []DiffLine
	filePath string
	width    int
	height   int
	offset   int // scroll offset
}

// NewDiffView creates an empty DiffView.
func NewDiffView() DiffView {
	return DiffView{
		width:  80,
		height: 24,
	}
}

// SetDiff parses a unified diff string into colored lines.
func (d *DiffView) SetDiff(filePath, content string) {
	d.filePath = filePath
	d.offset = 0
	d.lines = parseDiffLines(content)
}

// SetSize updates the viewport dimensions.
func (d *DiffView) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	d.width = width
	d.height = height
}

// ScrollUp moves the viewport up by one line.
func (d *DiffView) ScrollUp() {
	if d.offset > 0 {
		d.offset--
	}
}

// ScrollDown moves the viewport down by one line.
func (d *DiffView) ScrollDown() {
	maxOffset := len(d.lines) - d.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if d.offset < maxOffset {
		d.offset++
	}
}

// View renders the visible portion of the diff.
func (d DiffView) View() string {
	if len(d.lines) == 0 {
		return ""
	}

	end := d.offset + d.height
	if end > len(d.lines) {
		end = len(d.lines)
	}

	visible := d.lines[d.offset:end]
	rendered := make([]string, 0, len(visible))

	addedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#66FF66"))
	removedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6666"))
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#58A6FF")).Bold(true)

	useSideBySide := d.width > 160

	if useSideBySide {
		rendered = d.renderSideBySide(visible, addedStyle, removedStyle, headerStyle)
	} else {
		for _, line := range visible {
			text := truncateLine(line.Content, d.width)
			switch line.Type {
			case DiffAdded:
				rendered = append(rendered, addedStyle.Render("+"+text))
			case DiffRemoved:
				rendered = append(rendered, removedStyle.Render("-"+text))
			case DiffHeader:
				rendered = append(rendered, headerStyle.Render(text))
			default:
				rendered = append(rendered, " "+text)
			}
		}
	}

	return strings.Join(rendered, "\n")
}

// renderSideBySide renders removed/added pairs in two columns.
func (d DiffView) renderSideBySide(lines []DiffLine, addedStyle, removedStyle, headerStyle lipgloss.Style) []string {
	colWidth := (d.width - 3) / 2 // 3 for separator " | "
	if colWidth < 1 {
		colWidth = 1
	}

	var result []string
	for _, line := range lines {
		text := truncateLine(line.Content, colWidth)
		switch line.Type {
		case DiffRemoved:
			left := removedStyle.Render(fmt.Sprintf("%-*s", colWidth, "-"+text))
			result = append(result, left+" | ")
		case DiffAdded:
			right := addedStyle.Render("+"+text)
			padding := strings.Repeat(" ", colWidth)
			result = append(result, padding+" | "+right)
		case DiffHeader:
			result = append(result, headerStyle.Render(text))
		default:
			left := fmt.Sprintf("%-*s", colWidth, " "+text)
			result = append(result, left+" | "+" "+text)
		}
	}
	return result
}

// LineCount returns the total number of diff lines.
func (d DiffView) LineCount() int {
	return len(d.lines)
}

// parseDiffLines splits a unified diff into typed lines.
func parseDiffLines(content string) []DiffLine {
	if content == "" {
		return nil
	}

	raw := strings.Split(content, "\n")
	lines := make([]DiffLine, 0, len(raw))

	for _, line := range raw {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
			strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "@@"):
			lines = append(lines, DiffLine{Type: DiffHeader, Content: line})
		case strings.HasPrefix(line, "+"):
			lines = append(lines, DiffLine{Type: DiffAdded, Content: line[1:]})
		case strings.HasPrefix(line, "-"):
			lines = append(lines, DiffLine{Type: DiffRemoved, Content: line[1:]})
		default:
			content := line
			if strings.HasPrefix(content, " ") {
				content = content[1:]
			}
			lines = append(lines, DiffLine{Type: DiffContext, Content: content})
		}
	}

	return lines
}

// truncateLine shortens a line to fit within maxWidth.
func truncateLine(s string, maxWidth int) string {
	if maxWidth < 4 {
		maxWidth = 4
	}
	if len(s) <= maxWidth {
		return s
	}
	return s[:maxWidth-3] + "..."
}

// DiffOpenMsg requests opening the diff viewer for a file.
type DiffOpenMsg struct {
	FilePath string
	Content  string
}

// DiffClosedMsg signals the diff viewer was dismissed.
type DiffClosedMsg struct{}
