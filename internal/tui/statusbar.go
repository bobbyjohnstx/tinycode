package tui

import (
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar renders a two-line bottom area: hints line + status bar.
type StatusBar struct {
	cwd      string
	model    string
	agent    string
	provider string
	working  bool
	spinner  spinner.Model
	width    int
}

// NewStatusBar creates a StatusBar with the given width.
func NewStatusBar(width int) StatusBar {
	sp := spinner.New()
	sp.Spinner = knightRiderSpinner()
	sp.Style = lipgloss.NewStyle()

	return StatusBar{
		spinner: sp,
		width:   width,
		agent:   "build",
	}
}

// knightRiderSpinner creates a Knight Rider-style bouncing block scanner.
func knightRiderSpinner() spinner.Spinner {
	const width = 8
	accentColor := lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#CC4444"}
	dimColor := lipgloss.AdaptiveColor{Light: "#552222", Dark: "#441111"}

	accent := lipgloss.NewStyle().Foreground(accentColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)

	// Forward (0→7) + backward (6→1) = 14 frames
	var frames []string
	positions := make([]int, 0, width*2-2)
	for i := 0; i < width; i++ {
		positions = append(positions, i)
	}
	for i := width - 2; i > 0; i-- {
		positions = append(positions, i)
	}

	for _, pos := range positions {
		var frame strings.Builder
		for i := 0; i < width; i++ {
			dist := pos - i
			if dist < 0 {
				dist = -dist
			}
			switch {
			case dist == 0:
				frame.WriteString(accent.Render("█"))
			case dist == 1:
				frame.WriteString(accent.Render("▓"))
			case dist == 2:
				frame.WriteString(dim.Render("▒"))
			default:
				frame.WriteString(dim.Render("·"))
			}
		}
		frames = append(frames, frame.String())
	}

	return spinner.Spinner{
		Frames: frames,
		FPS:    time.Second / 25,
	}
}

// SetSize updates the status bar width.
func (s *StatusBar) SetSize(width int) {
	s.width = width
}

// SetCwd updates the working directory display.
func (s *StatusBar) SetCwd(cwd string) {
	s.cwd = cwd
}

// SetModel updates the model/provider display.
func (s *StatusBar) SetModel(model, provider string) {
	s.model = model
	s.provider = provider
}

// SetAgent updates the agent display.
func (s *StatusBar) SetAgent(agent string) {
	s.agent = agent
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
	hintsRight := dim.Render("tab") + " agents  " + dim.Render("ctrl+p") + " commands"

	hintsGap := s.width - lipgloss.Width(hintsLeft) - lipgloss.Width(hintsRight)
	if hintsGap < 1 {
		hintsGap = 1
	}
	hintsLine := hintsLeft + strings.Repeat(" ", hintsGap) + hintsRight

	// Status bar: cwd left, model right.
	innerWidth := s.width - 2

	left := ""
	if s.cwd != "" {
		left = shortenCwd(s.cwd)
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
