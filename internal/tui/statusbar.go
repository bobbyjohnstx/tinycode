package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar renders the bottom status line showing cwd, model, agent, and hints.
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
	sp.Spinner = spinner.Dot
	sp.Style = styleSpinner

	return StatusBar{
		spinner: sp,
		width:   width,
		agent:   "build",
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

// SetWorking updates the working state.
func (s *StatusBar) SetWorking(working bool) {
	s.working = working
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

// View implements tea.Model.
func (s StatusBar) View() string {
	var parts []string

	// Working indicator
	if s.working {
		parts = append(parts, s.spinner.View())
	}

	// CWD (shortened)
	if s.cwd != "" {
		dir := shortenPath(s.cwd)
		parts = append(parts, dir)
	}

	// Model info
	if s.model != "" {
		modelStr := s.model
		if s.provider != "" {
			modelStr = fmt.Sprintf("%s %s", s.model, s.provider)
		}
		parts = append(parts, modelStr)
	}

	// Agent
	if s.agent != "" {
		parts = append(parts, s.agent)
	}

	// Hints
	hints := "ctrl+p commands"

	left := strings.Join(parts, " │ ")
	right := hints

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	line := left + strings.Repeat(" ", gap) + right
	return styleStatusBar.Width(s.width).Render(line)
}

// shortenPath returns a display-friendly path (last 2 components).
func shortenPath(p string) string {
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	parent := filepath.Base(dir)
	if parent == "." || parent == "/" {
		return base
	}
	return parent + "/" + base
}
