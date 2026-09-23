package tui

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar renders a two-line bottom area: hints line + status bar.
type StatusBar struct {
	cwd            string
	model          string
	agent          string
	provider       string
	working        bool
	leaderPending  bool
	safeMode       bool
	spinner        spinner.Model
	agentColor     lipgloss.AdaptiveColor
	width          int
}

// NewStatusBar creates a StatusBar with the given width.
func NewStatusBar(width int) StatusBar {
	color := AgentColor("build")
	sp := spinner.New()
	sp.Spinner = brailleWaveSpinner(color)
	sp.Style = lipgloss.NewStyle()

	return StatusBar{
		spinner:    sp,
		agentColor: color,
		width:      width,
		agent:      "build",
	}
}

// brailleWaveSpinner creates a fluid sine wave using braille dot patterns,
// colored by the current agent.
func brailleWaveSpinner(agentColor lipgloss.AdaptiveColor) spinner.Spinner {
	const width = 16
	const totalFrames = 32

	heights := []rune{'⠀', '⡀', '⡄', '⡆', '⡇', '⣇', '⣧', '⣷', '⣿'}

	bright := lipgloss.NewStyle().Foreground(agentColor)
	mid := lipgloss.NewStyle().Foreground(agentColor).Faint(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#333333"})

	var frames []string
	for f := 0; f < totalFrames; f++ {
		var frame strings.Builder
		for i := 0; i < width; i++ {
			phase := float64(f) / float64(totalFrames) * 2 * math.Pi
			x := float64(i) / float64(width) * 2 * math.Pi
			val := (math.Sin(x+phase) + 1) / 2
			idx := int(val * float64(len(heights)-1))
			ch := heights[idx]

			switch {
			case val > 0.7:
				frame.WriteString(bright.Render(string(ch)))
			case val > 0.3:
				frame.WriteString(mid.Render(string(ch)))
			default:
				frame.WriteString(dim.Render(string(ch)))
			}
		}
		frames = append(frames, frame.String())
	}
	return spinner.Spinner{Frames: frames, FPS: time.Second / 20}
}

// SetSize updates the status bar width.
func (s *StatusBar) SetSize(width int) {
	s.width = width
}

// SetCwd updates the working directory display.
func (s *StatusBar) SetCwd(cwd string) {
	s.cwd = cwd
}

// Cwd returns the current working directory.
func (s StatusBar) Cwd() string {
	return s.cwd
}

// SetModel updates the model/provider display.
func (s *StatusBar) SetModel(model, provider string) {
	s.model = model
	s.provider = provider
}

// SetAgent updates the agent display and recolors the spinner.
func (s *StatusBar) SetAgent(agent string) {
	s.agent = agent
	color := AgentColor(agent)
	if color != s.agentColor {
		s.agentColor = color
		s.spinner.Spinner = brailleWaveSpinner(color)
	}
}

// SetSafeMode sets the safe mode indicator.
func (s *StatusBar) SetSafeMode(safe bool) {
	s.safeMode = safe
}

// SetLeaderPending updates the leader-key pending state, which switches
// the hints line to show available follow-up keys.
func (s *StatusBar) SetLeaderPending(pending bool) {
	s.leaderPending = pending
}

// SetWorking updates the working state and returns a command to restart
// the spinner tick chain when transitioning to working.
func (s *StatusBar) SetWorking(working bool) tea.Cmd {
	wasWorking := s.working
	s.working = working
	if working && !wasWorking {
		return s.spinner.Tick
	}
	return nil
}

// Init implements tea.Model.
func (s StatusBar) Init() tea.Cmd {
	return s.spinner.Tick
}

// Update implements tea.Model.
func (s StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	if s.working {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}
	return s, nil
}

// View renders the hints line and the status bar.
func (s StatusBar) View() string {
	dim := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})
	accent := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"})

	// Hints line
	var hintsLeft string
	if s.working {
		hintsLeft = s.spinner.View() + " " + dim.Render("esc interrupt")
	}

	var hintsRight string
	if s.leaderPending {
		hintsRight = dim.Render("ctrl+x →") + "  " +
			accent.Render("b") + " sidebar  " +
			accent.Render("a") + " agents  " +
			accent.Render("m") + " models  " +
			accent.Render("o") + " sessions  " +
			accent.Render("n") + " new  " +
			accent.Render("x") + " export"
	} else {
		hintsRight = dim.Render("tab") + " agents  " +
			dim.Render("ctrl+p") + " commands  " +
			dim.Render("/help") + " reference"
	}

	hintsGap := s.width - lipgloss.Width(hintsLeft) - lipgloss.Width(hintsRight)
	if hintsGap < 1 {
		hintsGap = 1
	}
	hintsLine := hintsLeft + strings.Repeat(" ", hintsGap) + hintsRight

	// Status bar: cwd left, model right.
	innerWidth := s.width - 2

	left := ""
	if s.safeMode {
		warn := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#B35900", Dark: "#FFA500"}).
			Bold(true)
		left = warn.Render("SAFE MODE") + "  "
	}
	if s.cwd != "" {
		left += shortenCwd(s.cwd)
	}

	var rightParts []string
	if s.model != "" {
		rightParts = append(rightParts, accent.Render(s.model))
	}
	if s.provider != "" {
		rightParts = append(rightParts, dim.Render(s.provider))
	}
	right := strings.Join(rightParts, "  ")

	statusGap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if statusGap < 1 {
		statusGap = 1
	}
	statusLine := left + strings.Repeat(" ", statusGap) + right

	return hintsLine + "\n" + styleStatusBar.Width(s.width).Render(statusLine)
}

// truncatedModelProvider builds a "model  provider" string that fits within
// maxWidth, truncating the model name with an ellipsis if needed.
func truncatedModelProvider(model, provider string, maxWidth int) string {
	var parts []string
	if model != "" {
		parts = append(parts, model)
	}
	if provider != "" {
		parts = append(parts, provider)
	}
	combined := strings.Join(parts, "  ")
	if maxWidth <= 0 || lipgloss.Width(combined) <= maxWidth {
		return combined
	}
	if provider != "" && model != "" {
		providerWidth := lipgloss.Width(provider)
		available := maxWidth - providerWidth - 5 // "  " separator + "..."
		if available > 3 {
			model = model[:available] + "..."
			return model + "  " + provider
		}
	}
	if lipgloss.Width(combined) > maxWidth && maxWidth > 3 {
		return combined[:maxWidth-3] + "..."
	}
	return combined
}

// shortenCwd returns a display-friendly path with ~ for home dir.
func shortenCwd(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if strings.HasPrefix(p, home) {
			p = "~" + p[len(home):]
		}
	}
	return p
}
