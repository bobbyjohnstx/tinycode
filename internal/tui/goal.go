package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// goalTracker manages the goal evaluation loop on the TUI side.
// It tracks the condition, command, iteration count, and recent outputs
// for stuck detection.
type goalTracker struct {
	state         *session.GoalState
	recentOutputs []string // last N evaluation outputs for stuck detection
}

const goalStuckThreshold = 3

func newGoalTracker(text, command string) *goalTracker {
	return &goalTracker{
		state: &session.GoalState{
			Text:          text,
			Command:       command,
			MaxIterations: session.DefaultGoalMaxIterations(),
		},
	}
}

// GoalEvalMsg carries the result of a goal condition evaluation.
type GoalEvalMsg struct {
	Met      bool   // true if exit code 0
	Output   string // combined stdout+stderr
	Err      error  // exec error (non-zero exit is NOT an error here)
	Iteration int
}

// evaluateGoal runs the goal command and returns a GoalEvalMsg.
func evaluateGoal(command, dir string, iteration int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}

		met := err == nil // exit code 0 = condition met
		return GoalEvalMsg{
			Met:       met,
			Output:    output,
			Iteration: iteration,
		}
	}
}

// isStuck checks if the last N evaluation outputs are identical,
// indicating the goal is not making progress.
func (g *goalTracker) isStuck(output string) bool {
	g.recentOutputs = append(g.recentOutputs, output)
	if len(g.recentOutputs) > goalStuckThreshold {
		g.recentOutputs = g.recentOutputs[len(g.recentOutputs)-goalStuckThreshold:]
	}
	if len(g.recentOutputs) < goalStuckThreshold {
		return false
	}
	first := g.recentOutputs[0]
	for _, o := range g.recentOutputs[1:] {
		if o != first {
			return false
		}
	}
	return true
}

// statusText returns a short status string for the status bar.
func (g *goalTracker) statusText() string {
	if g == nil || g.state == nil {
		return ""
	}
	return fmt.Sprintf("Goal: %d/%d — %s", g.state.Iteration, g.state.MaxIterations, g.state.Text)
}
