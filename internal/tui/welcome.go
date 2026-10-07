package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logoRevealRows is the monogram chip + wordmark (3 lines), revealed on boot ticks.
const logoRevealRows = 3

type bootCheck struct {
	label string
	key   string
}

var bootChecks = []bootCheck{
	{"Loading configuration", "config"},
	{"Connecting to server", "sse"},
	{"Discovering providers", "providers"},
	{"Loading agents", "agents"},
	{"Loading plugins", "plugins"},
	{"Loading sessions", "sessions"},
	{"Checking MCP servers", "mcp"},
}

type bootStatus int

const (
	bootPending bootStatus = iota
	bootRunning
	bootDone
	bootFailed
)

// WelcomeView renders the boot sequence welcome screen with logo and system checks.
type WelcomeView struct {
	checks   map[string]bootStatus
	order    []string
	bootDone bool
	logoShow int
	startAt  time.Time
}

func NewWelcomeView() WelcomeView {
	order := make([]string, len(bootChecks))
	for i, c := range bootChecks {
		order[i] = c.key
	}
	checks := make(map[string]bootStatus, len(bootChecks))
	checks["config"] = bootDone
	return WelcomeView{
		checks:  checks,
		order:   order,
		startAt: time.Now(),
	}
}

func (w *WelcomeView) MarkDone(key string) {
	if _, ok := w.checks[key]; ok || key == "" {
		w.checks[key] = bootDone
	} else {
		w.checks[key] = bootDone
	}
	w.updateBootDone()
}

func (w *WelcomeView) MarkFailed(key string) {
	w.checks[key] = bootFailed
	w.updateBootDone()
}

func (w *WelcomeView) updateBootDone() {
	for _, k := range w.order {
		s := w.checks[k]
		if s == bootPending || s == bootRunning {
			return
		}
	}
	w.bootDone = true
}

func (w *WelcomeView) Tick() tea.Cmd {
	if w.bootDone && w.logoShow >= logoRevealRows {
		return nil
	}
	w.logoShow++
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return bootTickMsg{}
	})
}

type bootTickMsg struct{}

func (w WelcomeView) View(width, height int, providerName, modelName string, sessionCount, agentCount, pluginCount, mcpCount int) string {
	leftColor := lipgloss.AdaptiveColor{Light: "#4488CC", Dark: "#38bdf8"}
	rightColor := lipgloss.AdaptiveColor{Light: "#333333", Dark: "#f8fafc"}
	checkColor := lipgloss.AdaptiveColor{Light: "#22AA44", Dark: "#7fd88f"}
	failColor := lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#e8607a"}
	dimColor := lipgloss.AdaptiveColor{Light: "#999999", Dark: "#555555"}
	labelColor := lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}
	accentColor := lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#c87898"}

	leftStyle := lipgloss.NewStyle().Foreground(leftColor)
	rightStyle := lipgloss.NewStyle().Foreground(rightColor)
	checkStyle := lipgloss.NewStyle().Foreground(checkColor)
	failStyle := lipgloss.NewStyle().Foreground(failColor)
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)
	labelStyle := lipgloss.NewStyle().Foreground(labelColor)
	accentStyle := lipgloss.NewStyle().Foreground(accentColor)

	var lines []string

	// Monogram chip + wordmark — reveal row by row
	chipBorder := leftStyle
	chipInner := leftStyle.Bold(true)
	nameStyle := rightStyle.Bold(true)
	subtitleStyle := dimStyle
	logoRows := []string{
		chipBorder.Render("┌────┐"),
		chipBorder.Render("│") + chipInner.Render(" tc ") + chipBorder.Render("│") +
			"   " + nameStyle.Render("tinycode"),
		chipBorder.Render("└────┘") + "   " + subtitleStyle.Render("local-first coding agent"),
	}
	showRows := w.logoShow
	if showRows > len(logoRows) {
		showRows = len(logoRows)
	}
	for i := 0; i < len(logoRows); i++ {
		if i < showRows {
			lines = append(lines, logoRows[i])
		}
	}
	lines = append(lines, "")

	// Boot checks
	for _, check := range bootChecks {
		status := w.checks[check.key]
		var icon, detail string

		switch status {
		case bootDone:
			icon = checkStyle.Render("✓")
			suffix := ""
			switch check.key {
			case "providers":
				if providerName != "" {
					suffix = dimStyle.Render(fmt.Sprintf(" · %s / %s", providerName, modelName))
				}
			case "agents":
				if agentCount > 0 {
					suffix = dimStyle.Render(fmt.Sprintf(" · %d agents", agentCount))
				}
			case "plugins":
				if pluginCount > 0 {
					suffix = dimStyle.Render(fmt.Sprintf(" · %d plugins", pluginCount))
				}
			case "sessions":
				if sessionCount > 0 {
					suffix = dimStyle.Render(fmt.Sprintf(" · %d sessions", sessionCount))
				}
			case "mcp":
				if mcpCount > 0 {
					suffix = dimStyle.Render(fmt.Sprintf(" · %d servers", mcpCount))
				}
			}
			detail = labelStyle.Render(check.label) + suffix
		case bootFailed:
			icon = failStyle.Render("✗")
			detail = failStyle.Render(check.label)
		default:
			icon = dimStyle.Render("·")
			detail = dimStyle.Render(check.label)
		}
		lines = append(lines, fmt.Sprintf("   %s  %s", icon, detail))
	}

	lines = append(lines, "")

	// Tips lines — only show after boot
	if w.bootDone {
		tips1 := []string{
			"/ commands",
			"@ files",
			"tab agents",
			"ctrl+p palette",
		}
		if providerName == "" && modelName == "" {
			tips1 = []string{
				"/connect",
				"/ commands",
				"@ files",
				"tab agents",
			}
		}
		tips2 := []string{
			"ctrl+x sidebar/sessions",
			"shift+enter newline",
			"/help reference",
		}
		tipLine1 := accentStyle.Render("→") + "  " + labelStyle.Render(strings.Join(tips1, "  ·  "))
		tipLine2 := "   " + labelStyle.Render(strings.Join(tips2, "  ·  "))
		lines = append(lines, "   "+tipLine1)
		lines = append(lines, tipLine2)
	}

	blockWidth := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > blockWidth {
			blockWidth = w
		}
	}
	content := lipgloss.NewStyle().
		Width(blockWidth).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
