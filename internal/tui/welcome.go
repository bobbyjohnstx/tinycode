package tui

import (
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Logo rows from the TS version's logo.ts. Each row has a left (muted) part
// and right (bright) part separated at splitCol. The cell renderer converts
// marker characters: _ → space (filled block via bg color), ^ → ▀ (half-block
// with shadow bg), ~ → ▀ (half-block with shadow fg).
type logoRow struct {
	left  string
	right string
}

func renderLogoTemplate(template string) string {
	var sb strings.Builder
	for _, ch := range template {
		switch ch {
		case '_':
			sb.WriteRune(' ')
		case '^', '~':
			sb.WriteRune('▀')
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

var logoData = []struct {
	leftTpl  string
	rightTpl string
}{
	{"▀█▀ ▄_ █▀▀▄ █__█", "╲"},
	{"_█_ █_ █__█ _▀▀█", "  ╲"},
	{"_▀_ ▀_ ▀~~▀ ___▀", "    ╲    █▀▀▀ █▀▀█ █▀▀█ █▀▀█"},
	{"", "                      ╲  █___ █__█ █__█ █^^^"},
	{"", "                        ╲▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀"},
}

var tips = []string{
	"Type @ to attach files, /ask to invoke agents",
	"Start with / for commands, ctrl+p for palette",
	"Press tab to switch agents, F1 for help",
	"Show keyboard shortcuts with ctrl+alt+k",
}

// WelcomeView renders the centered welcome screen with logo and tips.
type WelcomeView struct {
	tipIndex int
}

// NewWelcomeView creates a new WelcomeView with a random tip.
func NewWelcomeView() WelcomeView {
	return WelcomeView{
		tipIndex: rand.Intn(len(tips)),
	}
}

// View renders the welcome screen centered in the given dimensions.
func (w WelcomeView) View(width, height int) string {
	leftColor := lipgloss.AdaptiveColor{Light: "#4488CC", Dark: "#38bdf8"}
	rightColor := lipgloss.AdaptiveColor{Light: "#333333", Dark: "#f8fafc"}

	leftStyle := lipgloss.NewStyle().Foreground(leftColor)
	rightStyle := lipgloss.NewStyle().Foreground(rightColor)

	// Find the widest rendered logo line to pad shorter lines for alignment.
	type renderedRow struct {
		left, right string
		plainLen    int
	}
	var rows []renderedRow
	maxLen := 0
	for _, row := range logoData {
		left := renderLogoTemplate(row.leftTpl)
		right := renderLogoTemplate(row.rightTpl)
		plainLen := len([]rune(left)) + len([]rune(right))
		if plainLen > maxLen {
			maxLen = plainLen
		}
		rows = append(rows, renderedRow{left, right, plainLen})
	}

	var logoLines []string
	for _, row := range rows {
		pad := strings.Repeat(" ", maxLen-row.plainLen)
		line := leftStyle.Render(row.left) + rightStyle.Render(row.right) + pad
		logoLines = append(logoLines, line)
	}
	logoBlock := strings.Join(logoLines, "\n")

	tipStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})

	gettingStarted := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFAA33"}).
		Bold(true).
		Render("● Getting Started")

	var tipLines []string
	tipLines = append(tipLines, gettingStarted)
	tipLines = append(tipLines, "")
	for _, t := range tips {
		tipLines = append(tipLines, "   "+tipStyle.Render(t))
		tipLines = append(tipLines, "")
	}

	tipsBlock := strings.Join(tipLines, "\n")

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoBlock,
		"",
		tipsBlock,
	)

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
