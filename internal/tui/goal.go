package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/procenv"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// goalTracker manages the goal evaluation loop on the TUI side.
// It tracks the condition, command, iteration count, and recent outputs
// for stuck detection.
type goalTracker struct {
	state           *session.GoalState
	recentOutputs   []string // last N evaluation outputs for stuck detection (command goals)
	recentNextSteps []string // last N next_step values for stuck detection (judge goals)
	generation      uint64
}

const goalStuckThreshold = 3

var goalGenCounter uint64

func newGoalTracker(text, command string) *goalTracker {
	goalGenCounter++
	return &goalTracker{
		state: &session.GoalState{
			Text:          text,
			Command:       command,
			MaxIterations: session.DefaultGoalMaxIterations(),
		},
		generation: goalGenCounter,
	}
}

// GoalEvalMsg carries the result of a goal condition evaluation.
type GoalEvalMsg struct {
	Met        bool   // true if exit code 0
	Output     string // combined stdout+stderr
	Err        error  // exec error (non-zero exit is NOT an error here)
	Iteration  int
	Generation uint64
}

// evaluateGoal runs the goal command and returns a GoalEvalMsg.
func evaluateGoal(command, dir string, iteration int, generation uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = dir
		cmd.Env = procenv.Child(nil)
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
			Met:        met,
			Output:     output,
			Iteration:  iteration,
			Generation: generation,
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

// isNextStepStuck checks if the last N judge next_step values are identical,
// indicating the judge is returning the same guidance repeatedly.
func (g *goalTracker) isNextStepStuck(nextStep string) bool {
	g.recentNextSteps = append(g.recentNextSteps, nextStep)
	if len(g.recentNextSteps) > goalStuckThreshold {
		g.recentNextSteps = g.recentNextSteps[len(g.recentNextSteps)-goalStuckThreshold:]
	}
	if len(g.recentNextSteps) < goalStuckThreshold {
		return false
	}
	first := g.recentNextSteps[0]
	for _, s := range g.recentNextSteps[1:] {
		if s != first {
			return false
		}
	}
	return true
}

// GoalJudgeMsg carries the result of an LLM-as-judge goal evaluation.
type GoalJudgeMsg struct {
	Verdict    string // "done", "continue", "blocked", "paused"
	Reason     string
	NextStep   string
	Err        error
	Generation uint64
}

// judgeGoal makes an LLM-as-judge call to evaluate whether a goal objective is met.
func judgeGoal(client *api.Client, sessionID, objective string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		result, err := client.GoalJudge(sessionID, objective)
		if err != nil {
			return GoalJudgeMsg{
				Verdict:    "continue",
				Reason:     "Judge call failed: " + err.Error(),
				NextStep:   "Continue working on the objective.",
				Err:        err,
				Generation: generation,
			}
		}
		return GoalJudgeMsg{
			Verdict:    result.Verdict,
			Reason:     result.Reason,
			NextStep:   result.NextStep,
			Generation: generation,
		}
	}
}

// statusText returns a short status string for the status bar.
func (g *goalTracker) statusText() string {
	if g == nil || g.state == nil {
		return ""
	}
	return fmt.Sprintf("Goal: %d/%d — %s", g.state.Iteration, g.state.MaxIterations, g.state.Text)
}
