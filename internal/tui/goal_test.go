package tui

import (
	"strings"
	"testing"
)

func TestGoalTracker_StatusText(t *testing.T) {
	g := newGoalTracker("all tests pass", "go test ./... -count=1")
	g.state.Iteration = 2

	got := g.statusText()
	want := "Goal: 2/10 — all tests pass"
	if got != want {
		t.Errorf("statusText() = %q, want %q", got, want)
	}
}

func TestGoalTracker_StatusText_Nil(t *testing.T) {
	var g *goalTracker
	if got := g.statusText(); got != "" {
		t.Errorf("nil goalTracker statusText() = %q, want empty", got)
	}
}

func TestGoalTracker_IsStuck_NotEnoughSamples(t *testing.T) {
	g := newGoalTracker("tests pass", "go test ./...")
	if g.isStuck("output1") {
		t.Error("expected not stuck with 1 sample")
	}
	if g.isStuck("output1") {
		t.Error("expected not stuck with 2 samples")
	}
}

func TestGoalTracker_IsStuck_SameOutputTriggersStuck(t *testing.T) {
	g := newGoalTracker("tests pass", "go test ./...")
	g.isStuck("same output")
	g.isStuck("same output")
	if !g.isStuck("same output") {
		t.Error("expected stuck after 3 identical outputs")
	}
}

func TestGoalTracker_IsStuck_DifferentOutputNotStuck(t *testing.T) {
	g := newGoalTracker("tests pass", "go test ./...")
	g.isStuck("output1")
	g.isStuck("output2")
	if g.isStuck("output3") {
		t.Error("expected not stuck with different outputs")
	}
}

func TestGoalTracker_IsStuck_ResetAfterDifferent(t *testing.T) {
	g := newGoalTracker("tests pass", "go test ./...")
	g.isStuck("same")
	g.isStuck("same")
	g.isStuck("different") // breaks the streak
	if g.isStuck("same") {
		t.Error("expected not stuck after streak broken")
	}
}

func TestNewGoalTracker_Defaults(t *testing.T) {
	g := newGoalTracker("build succeeds", "go build ./...")
	if g.state.Text != "build succeeds" {
		t.Errorf("Text = %q, want %q", g.state.Text, "build succeeds")
	}
	if g.state.Command != "go build ./..." {
		t.Errorf("Command = %q, want %q", g.state.Command, "go build ./...")
	}
	if g.state.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", g.state.MaxIterations)
	}
	if g.state.Iteration != 0 {
		t.Errorf("Iteration = %d, want 0", g.state.Iteration)
	}
}

func TestNewGoalTracker_EmptyCommand(t *testing.T) {
	g := newGoalTracker("make the auth module work", "")
	if g.state.Text != "make the auth module work" {
		t.Errorf("Text = %q, want %q", g.state.Text, "make the auth module work")
	}
	if g.state.Command != "" {
		t.Errorf("Command = %q, want empty", g.state.Command)
	}
	if g.state.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", g.state.MaxIterations)
	}
}

func TestGoalTracker_EmptyCommandSkipsShellEval(t *testing.T) {
	// A goalTracker with empty command should not produce a shell evaluation.
	// The evaluateGoal function requires a non-empty command; an empty command
	// means the model self-assesses via its prompt protocol.
	g := newGoalTracker("fix the auth module", "")
	if g.state.Command != "" {
		t.Fatal("expected empty command for self-assessment goal")
	}
	// Iteration tracking still works.
	g.state.Iteration++
	if g.state.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1", g.state.Iteration)
	}
}

func TestEvaluateGoal_SuccessfulCommand(t *testing.T) {
	msg := evaluateGoal("true", t.TempDir(), 1, 0)()
	got, ok := msg.(GoalEvalMsg)
	if !ok {
		t.Fatalf("message = %T, want GoalEvalMsg", msg)
	}
	if !got.Met {
		t.Error("expected Met=true for exit-0 command")
	}
	if got.Err != nil {
		t.Errorf("unexpected Err: %v", got.Err)
	}
	if got.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1", got.Iteration)
	}
}

func TestEvaluateGoal_FailingCommand(t *testing.T) {
	msg := evaluateGoal("false", t.TempDir(), 3, 0)()
	got, ok := msg.(GoalEvalMsg)
	if !ok {
		t.Fatalf("message = %T, want GoalEvalMsg", msg)
	}
	if got.Met {
		t.Error("expected Met=false for exit-nonzero command")
	}
	if got.Iteration != 3 {
		t.Errorf("Iteration = %d, want 3", got.Iteration)
	}
}

func TestEvaluateGoal_CapturesStderr(t *testing.T) {
	msg := evaluateGoal("echo err >&2", t.TempDir(), 1, 0)()
	got, ok := msg.(GoalEvalMsg)
	if !ok {
		t.Fatalf("message = %T, want GoalEvalMsg", msg)
	}
	if !strings.Contains(got.Output, "err") {
		t.Errorf("Output = %q, want it to contain stderr text 'err'", got.Output)
	}
}

func TestEvaluateGoal_CombinesStdoutAndStderr(t *testing.T) {
	msg := evaluateGoal("echo out && echo err >&2", t.TempDir(), 2, 0)()
	got, ok := msg.(GoalEvalMsg)
	if !ok {
		t.Fatalf("message = %T, want GoalEvalMsg", msg)
	}
	if !strings.Contains(got.Output, "out") {
		t.Errorf("Output = %q, missing stdout 'out'", got.Output)
	}
	if !strings.Contains(got.Output, "err") {
		t.Errorf("Output = %q, missing stderr 'err'", got.Output)
	}
}

func TestGoalTracker_IsStuck_WindowSlides(t *testing.T) {
	g := newGoalTracker("tests pass", "go test ./...")
	// Fill window with identical outputs to trigger stuck.
	g.isStuck("same")
	g.isStuck("same")
	if !g.isStuck("same") {
		t.Fatal("expected stuck after 3 identical")
	}
	// Break the streak with a different output.
	if g.isStuck("different") {
		t.Error("expected not stuck after different output")
	}
	// Two more identical — should NOT trigger stuck yet (need 3 fresh).
	if g.isStuck("new") {
		t.Error("expected not stuck with only 2 in window after break")
	}
	if g.isStuck("new") {
		t.Error("expected not stuck with 2 identical — window is [different, new, new]")
	}
	// Third identical makes it stuck again.
	if !g.isStuck("new") {
		t.Error("expected stuck after 3 consecutive identical 'new'")
	}
}

func TestGoalTracker_IterationTracking(t *testing.T) {
	g := newGoalTracker("tests pass", "go test ./... -count=1")
	if got := g.statusText(); got != "Goal: 0/10 — tests pass" {
		t.Errorf("initial statusText = %q", got)
	}
	g.state.Iteration = 5
	if got := g.statusText(); got != "Goal: 5/10 — tests pass" {
		t.Errorf("mid-run statusText = %q", got)
	}
	g.state.Iteration = 10
	if got := g.statusText(); got != "Goal: 10/10 — tests pass" {
		t.Errorf("final statusText = %q", got)
	}
}

func TestGoalTracker_GenerationIncrementsAcrossTrackers(t *testing.T) {
	g1 := newGoalTracker("tests pass", "go test ./...")
	g2 := newGoalTracker("build succeeds", "go build ./...")
	if g2.generation <= g1.generation {
		t.Errorf("g2.generation (%d) should be greater than g1.generation (%d)", g2.generation, g1.generation)
	}
}

func TestEvaluateGoal_PropagatesGeneration(t *testing.T) {
	msg := evaluateGoal("true", t.TempDir(), 1, 42)()
	got, ok := msg.(GoalEvalMsg)
	if !ok {
		t.Fatalf("message = %T, want GoalEvalMsg", msg)
	}
	if got.Generation != 42 {
		t.Errorf("Generation = %d, want 42", got.Generation)
	}
}

func TestEvaluateGoal_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	msg := evaluateGoal(`printf 'ran:%s' "$OPENROUTER_API_KEY"`, t.TempDir(), 1, 0)()
	got, ok := msg.(GoalEvalMsg)
	if !ok {
		t.Fatalf("message = %T", msg)
	}
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if strings.Contains(got.Output, "super-secret") || got.Output != "ran:" {
		t.Fatalf("goal output = %q", got.Output)
	}
}
