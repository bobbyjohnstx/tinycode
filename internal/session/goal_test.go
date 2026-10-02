package session

import "testing"

func TestResolveGoalCommand_KnownPatterns(t *testing.T) {
	tests := []struct {
		condition string
		wantCmd   string
		wantOK    bool
	}{
		{"all tests pass", "go test ./... -count=1", true},
		{"tests pass", "go test ./... -count=1", true},
		{"build succeeds", "go build ./...", true},
		{"no lint errors", "go vet ./...", true},
		{"lint passes", "go vet ./...", true},
		{"vet clean", "go vet ./...", true},
		// Case insensitive
		{"All Tests Pass", "go test ./... -count=1", true},
		{"BUILD SUCCEEDS", "go build ./...", true},
		// Substring match
		{"make sure all tests pass please", "go test ./... -count=1", true},
		// Unknown
		{"deploy to staging", "", false},
		{"fix the bug", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.condition, func(t *testing.T) {
			cmd, ok := ResolveGoalCommand(tt.condition)
			if ok != tt.wantOK {
				t.Errorf("ResolveGoalCommand(%q) ok = %v, want %v", tt.condition, ok, tt.wantOK)
			}
			if ok && cmd != tt.wantCmd {
				t.Errorf("ResolveGoalCommand(%q) = %q, want %q", tt.condition, cmd, tt.wantCmd)
			}
		})
	}
}

func TestGoalDoomThreshold(t *testing.T) {
	if got := GoalDoomThreshold(3); got != 6 {
		t.Errorf("GoalDoomThreshold(3) = %d, want 6", got)
	}
	if got := GoalDoomThreshold(5); got != 10 {
		t.Errorf("GoalDoomThreshold(5) = %d, want 10", got)
	}
}

func TestGoalState_Defaults(t *testing.T) {
	g := GoalState{
		Text:    "all tests pass",
		Command: "go test ./... -count=1",
	}
	if g.MaxIterations != 0 {
		t.Errorf("zero value MaxIterations = %d, want 0", g.MaxIterations)
	}
	if g.Iteration != 0 {
		t.Errorf("zero value Iteration = %d, want 0", g.Iteration)
	}
}

func TestDefaultGoalMaxIterations(t *testing.T) {
	if defaultGoalMaxIterations != 10 {
		t.Errorf("defaultGoalMaxIterations = %d, want 10", defaultGoalMaxIterations)
	}
}
