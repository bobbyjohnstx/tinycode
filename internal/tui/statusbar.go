package tui

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleStatusDim    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})
	styleStatusAccent = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"})
	styleStatusInfo   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1A8A9A", Dark: "#56B6C2"})
)

// goalFadeMsg is sent after the goal completion success line should collapse.
type goalFadeMsg struct{}

// goalDisplayState tracks goal rendering in the status bar.
type goalDisplayState struct {
	text          string
	iteration     int
	maxIterations int
	complete      bool // true when showing success line
}

// StatusBar renders a multi-line bottom area: hints line + optional goal box + status bar.
type StatusBar struct {
	cwd           string
	model         string
	agent         string
	provider      string
	effort        string
	contextPct    int // 0-100
	goalDisplay   *goalDisplayState
	working       bool
	activity      string
	turnStart     time.Time
	turnTokens    int
	leaderPending bool
	safeMode      bool
	spinner       spinner.Model
	agentColor    lipgloss.AdaptiveColor
	width         int
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

// SetGoal updates the goal status display. Pass "" to clear.
// For the visual goal box, use SetGoalState instead.
func (s *StatusBar) SetGoal(text string) {
	if text == "" {
		s.goalDisplay = nil
	} else if s.goalDisplay == nil {
		s.goalDisplay = &goalDisplayState{text: text}
	}
}

// SetGoalState updates the full goal display state for the visual goal box.
func (s *StatusBar) SetGoalState(text string, iteration, maxIterations int) {
	if text == "" {
		s.goalDisplay = nil
		return
	}
	s.goalDisplay = &goalDisplayState{
		text:          text,
		iteration:     iteration,
		maxIterations: maxIterations,
	}
}

// SetGoalComplete marks the goal as complete and returns a tea.Cmd
// that will send a goalFadeMsg after 5 seconds to collapse the success line.
func (s *StatusBar) SetGoalComplete(text string, iterations int) tea.Cmd {
	s.goalDisplay = &goalDisplayState{
		text:          text,
		iteration:     iterations,
		maxIterations: iterations,
		complete:      true,
	}
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return goalFadeMsg{}
	})
}

// SetEffort updates the effort level display. Only shown when not "medium".
func (s *StatusBar) SetEffort(level string) {
	s.effort = level
}

// SetContextPercent updates the context usage percentage (0-100).
func (s *StatusBar) SetContextPercent(pct int) {
	s.contextPct = pct
}

// SetLeaderPending updates the leader-key pending state, which switches
// the hints line to show available follow-up keys.
func (s *StatusBar) SetLeaderPending(pending bool) {
	s.leaderPending = pending
}

// SetActivity updates the working-state activity verb (e.g. "thinking", "bash").
func (s *StatusBar) SetActivity(activity string) {
	s.activity = activity
}

// SetTurnTokens updates the per-turn token count shown while working.
func (s *StatusBar) SetTurnTokens(n int) {
	s.turnTokens = n
}

// SetWorking updates the working state and returns a command to restart
// the spinner tick chain when transitioning to working.
func (s *StatusBar) SetWorking(working bool) tea.Cmd {
	wasWorking := s.working
	s.working = working
	if !working {
		s.activity = ""
		s.turnTokens = 0
		s.turnStart = time.Time{}
		return nil
	}
	if working && !wasWorking {
		s.turnStart = time.Now()
		s.turnTokens = 0
		if s.activity == "" {
			s.activity = "thinking"
		}
		return s.spinner.Tick
	}
	return nil
}

// Init implements tea.Model. Spinner ticks start via SetWorking(true).
func (s StatusBar) Init() tea.Cmd {
	return nil
}

// Height returns the current height of the status bar area.
// Working: 1 line (spinner + activity + turn telemetry). Idle: 2 lines (hints + status).
// Goal active adds 3 lines (box) or 1 line (success).
func (s StatusBar) Height() int {
	base := 2
	if s.working {
		base = 1
	}
	if s.goalDisplay == nil {
		return base
	}
	if s.goalDisplay.complete {
		return base + 1
	}
	return base + 3 // top/mid/bottom border rows
}

// Update implements tea.Model.
func (s StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	switch msg.(type) {
	case goalFadeMsg:
		s.goalDisplay = nil
		return s, nil
	}
	if s.working {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}
	return s, nil
}

// View renders the hints line, optional goal box, and the status bar.
func (s StatusBar) View() string {
	dim := styleStatusDim
	accent := styleStatusAccent
	dot := dim.Render(" · ")
	meta := s.renderStatusMeta(dim, accent, dot)

	goalLine := s.renderGoal()

	var body string
	if s.working {
		// One row while busy: spinner + activity | elapsed · tokens (no shortcut hints).
		var left string
		if s.activity == "" {
			left = s.spinner.View() + "  " + dim.Render("esc") + " " + accent.Render("interrupt")
		} else {
			left = s.spinner.View() + "  " + accent.Render(s.activity)
		}
		elapsed := time.Duration(0)
		if !s.turnStart.IsZero() {
			elapsed = time.Since(s.turnStart)
		}
		right := dim.Render(formatElapsed(elapsed))
		if s.turnTokens > 0 {
			right = dim.Render(fmt.Sprintf("%s · %s", formatElapsed(elapsed), formatTokens(s.turnTokens)))
		}
		body = renderHintsLine(s.width, left, right)
	} else {
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
		hintsLine := renderHintsLine(s.width, "", hintsRight)
		// Avoid styleStatusBar Padding — it can push past width and wrap.
		statusLine := clampStatusLine(s.width, meta)
		body = hintsLine + "\n" + statusLine
	}

	if goalLine != "" {
		return goalLine + "\n" + body
	}
	return body
}

// renderStatusMeta builds the agent · model · provider · meter segment.
func (s StatusBar) renderStatusMeta(dim, accent lipgloss.Style, dot string) string {
	var leftParts []string
	if s.safeMode {
		warn := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#B35900", Dark: "#FFA500"}).
			Bold(true)
		leftParts = append(leftParts, warn.Render("SAFE MODE"))
	}
	if s.agent != "" {
		leftParts = append(leftParts, accent.Render(s.agent))
	}
	if s.model != "" {
		leftParts = append(leftParts, dim.Render(s.model))
	} else {
		leftParts = append(leftParts, dim.Render("no model")+" "+accent.Render("/connect"))
	}
	if s.provider != "" {
		leftParts = append(leftParts, dim.Render(s.provider))
	}
	if s.effort != "" && s.effort != "medium" {
		leftParts = append(leftParts, dim.Render(s.effort))
	}
	if s.contextPct > 0 {
		leftParts = append(leftParts, renderContextMeter(s.contextPct))
	}
	return strings.Join(leftParts, dot)
}

// formatElapsed formats a duration as a short elapsed string (e.g. "2.4s").
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := d.Seconds()
	if secs < 60 {
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := int(secs) / 60
	rem := secs - float64(mins*60)
	return fmt.Sprintf("%dm%.0fs", mins, rem)
}

// clampStatusLine forces a single row of exactly width columns.
func clampStatusLine(width int, content string) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Width(width).MaxHeight(1).Render(content)
}

// renderHintsLine places left and right on a single row of exactly width columns.
func renderHintsLine(width int, left, right string) string {
	if width < 1 {
		width = 1
	}
	if right == "" {
		return clampStatusLine(width, left)
	}
	leftW := lipgloss.Width(left)
	if leftW >= width {
		return clampStatusLine(width, left)
	}
	gap := width - leftW - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
		avail := width - leftW - gap
		if avail < 1 {
			return clampStatusLine(width, left)
		}
		right = lipgloss.NewStyle().Width(avail).Align(lipgloss.Right).MaxHeight(1).Render(right)
		gap = width - leftW - lipgloss.Width(right)
		if gap < 0 {
			gap = 0
		}
	}
	return clampStatusLine(width, left+strings.Repeat(" ", gap)+right)
}

// renderContextMeter draws a short bar + percent with warn/hot thresholds.
func renderContextMeter(pct int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	const width = 10
	filled := pct * width / 100
	if pct > 0 && filled == 0 {
		filled = 1
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	label := fmt.Sprintf("%s %d%%", bar, pct)
	style := styleStatusInfo
	switch {
	case pct >= 90:
		style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#e8607a"})
	case pct >= 70:
		style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B35900", Dark: "#FFA500"})
	}
	return style.Render(label)
}

// renderGoal returns the goal box or success line, or "" if no goal is active.
func (s StatusBar) renderGoal() string {
	if s.goalDisplay == nil {
		return ""
	}

	if s.goalDisplay.complete {
		// Single success line
		successStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"})
		return successStyle.Render(fmt.Sprintf(
			"  ✓ Goal met: %s (%d iterations)",
			s.goalDisplay.text, s.goalDisplay.iteration,
		))
	}

	// Bordered goal box using box-drawing characters.
	borderColor := accent()
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	progress := fmt.Sprintf("%d/%d", s.goalDisplay.iteration, s.goalDisplay.maxIterations)
	topContent := fmt.Sprintf(" Goal: %s (%s) ", s.goalDisplay.text, progress)
	bottomContent := fmt.Sprintf("  Iteration %d of %d", s.goalDisplay.iteration, s.goalDisplay.maxIterations)

	boxWidth := s.width - 2 // leave some margin
	if boxWidth < 20 {
		boxWidth = 20
	}

	// Top border
	topLen := lipgloss.Width(topContent)
	topPad := boxWidth - topLen - 2 // -2 for corners
	if topPad < 0 {
		topPad = 0
	}
	topLine := borderStyle.Render("┌─") + borderStyle.Render(topContent) + borderStyle.Render(strings.Repeat("─", topPad)+"┐")

	// Middle line
	midLen := lipgloss.Width(bottomContent)
	midPad := boxWidth - midLen - 2
	if midPad < 0 {
		midPad = 0
	}
	midLine := borderStyle.Render("│") + bottomContent + strings.Repeat(" ", midPad) + borderStyle.Render("│")

	// Bottom border
	botLine := borderStyle.Render("└" + strings.Repeat("─", boxWidth-2) + "┘")

	return topLine + "\n" + midLine + "\n" + botLine
}

// accent returns the themed primary color for goal borders.
func accent() lipgloss.TerminalColor {
	if themeAgentColor != nil {
		return *themeAgentColor
	}
	return lipgloss.AdaptiveColor{Light: "#C87898", Dark: "#C87898"}
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
			modelRunes := []rune(model)
			if len(modelRunes) > available {
				model = string(modelRunes[:available]) + "..."
			}
			return model + "  " + provider
		}
	}
	if lipgloss.Width(combined) > maxWidth && maxWidth > 3 {
		combinedRunes := []rune(combined)
		if len(combinedRunes) > maxWidth-3 {
			return string(combinedRunes[:maxWidth-3]) + "..."
		}
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
