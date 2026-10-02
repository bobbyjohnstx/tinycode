package tui

import "testing"

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
