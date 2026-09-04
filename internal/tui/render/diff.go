package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleDiffAdd     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"})
	styleDiffDel     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"})
	styleDiffContext = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	styleDiffHeader  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}).Bold(true)
)

// RenderDiff renders a unified diff with colored additions, deletions, and context lines.
func RenderDiff(diff string, width int) string {
	if diff == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	lines := strings.Split(diff, "\n")
	var sb strings.Builder

	for _, line := range lines {
		if len(line) > width {
			line = line[:width]
		}
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			sb.WriteString(styleDiffHeader.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(styleDiffHeader.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(styleDiffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(styleDiffDel.Render(line))
		default:
			sb.WriteString(styleDiffContext.Render(line))
		}
		sb.WriteString("\n")
	}

	// Trim trailing newline to avoid extra blank line.
	result := sb.String()
	return strings.TrimRight(result, "\n")
}
