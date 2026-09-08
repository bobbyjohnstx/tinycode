package tui

import (
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var logo = []string{
	`████████╗██╗███╗   ██╗██╗   ██╗`,
	`╚══██╔══╝██║████╗  ██║╚██╗ ██╔╝`,
	`   ██║   ██║██╔██╗ ██║ ╚████╔╝ `,
	`   ██║   ██║██║╚██╗██║  ╚██╔╝  `,
	`   ██║   ██║██║ ╚████║   ██║   `,
	`   ╚═╝   ╚═╝╚═╝  ╚═══╝   ╚═╝   `,
	`     ██████╗ ██████╗ ██████╗ ███████╗`,
	`    ██╔════╝██╔═══██╗██╔══██╗██╔════╝`,
	`    ██║     ██║   ██║██║  ██║█████╗  `,
	`    ██║     ██║   ██║██║  ██║██╔══╝  `,
	`    ╚██████╗╚██████╔╝██████╔╝███████╗`,
	`     ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝`,
}

var tips = []string{
	"Use pgup , ctrl+alt+b/pgdn , ctrl+alt+f to navigate through conversation history",
	"Press ctrl+g , home to jump to the beginning of the conversation",
	"Show keyboard shortcuts with ctrl+alt+k",
	"Type / to see available commands",
	"Press tab to switch between agents",
	"Use ctrl+p to open the command palette",
}

// WelcomeView renders the centered welcome screen with logo and a random tip.
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
	logoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#555555"})

	logoBlock := logoStyle.Render(strings.Join(logo, "\n"))

	tipStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	tipLabel := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFAA33"}).
		Bold(true).
		Render("● Tip")

	tip := tipLabel + " " + tipStyle.Render(tips[w.tipIndex])

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoBlock,
		"",
		"",
		tip,
	)

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
