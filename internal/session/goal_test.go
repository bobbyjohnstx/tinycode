package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGoalCommand_KnownPatterns(t *testing.T) {
	tests := []struct {
		condition string
		wantCmd   string
		wantOK    bool
	}{
		{"all tests pass", "go test ./... -count=1", true},
		{"tests pass", "go test ./... -count=1", true},
		{"test passes", "go test ./... -count=1", true},
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
			cmd, ok := ResolveGoalCommand(tt.condition, "")
			if ok != tt.wantOK {
				t.Errorf("ResolveGoalCommand(%q) ok = %v, want %v", tt.condition, ok, tt.wantOK)
			}
			if ok && cmd != tt.wantCmd {
				t.Errorf("ResolveGoalCommand(%q) = %q, want %q", tt.condition, cmd, tt.wantCmd)
			}
		})
	}
}

func TestResolveGoalCommand_EcosystemDetection(t *testing.T) {
	tests := []struct {
		name      string
		indicator string
		condition string
		wantCmd   string
	}{
		{"node tests", "package.json", "tests pass", "npm test"},
		{"node build", "package.json", "build succeeds", "npm run build"},
		{"node lint", "package.json", "lint clean", "npm run lint"},
		{"rust tests", "Cargo.toml", "tests pass", "cargo test"},
		{"rust build", "Cargo.toml", "build succeeds", "cargo build"},
		{"rust lint", "Cargo.toml", "lint clean", "cargo clippy"},
		{"python tests", "pyproject.toml", "tests pass", "pytest"},
		{"python tests setup.py", "setup.py", "tests pass", "pytest"},
		{"python tests requirements.txt", "requirements.txt", "tests pass", "pytest"},
		{"make tests", "Makefile", "tests pass", "make test"},
		{"make build", "Makefile", "build succeeds", "make build"},
		{"make lint", "Makefile", "lint clean", "make lint"},
		{"go tests", "go.mod", "tests pass", "go test ./... -count=1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tt.indicator), []byte("{}"), 0644); err != nil {
				t.Fatal(err)
			}
			cmd, ok := ResolveGoalCommand(tt.condition, dir)
			if !ok {
				t.Fatalf("ResolveGoalCommand(%q) ok = false, want true", tt.condition)
			}
			if cmd != tt.wantCmd {
				t.Errorf("ResolveGoalCommand(%q) = %q, want %q", tt.condition, cmd, tt.wantCmd)
			}
		})
	}
}

func TestResolveGoalCommand_EcosystemPriority(t *testing.T) {
	// When multiple indicators exist, first match wins (package.json before go.mod).
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	cmd, ok := ResolveGoalCommand("tests pass", dir)
	if !ok {
		t.Fatal("expected ok = true")
	}
	if cmd != "npm test" {
		t.Errorf("got %q, want %q (package.json should take priority over go.mod)", cmd, "npm test")
	}
}

func TestResolveGoalCommand_VetGoOnly(t *testing.T) {
	// "vet clean" should only match for Go projects.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)

	_, ok := ResolveGoalCommand("vet clean", dir)
	if ok {
		t.Error("vet clean should not match for node projects")
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

func TestResolveGoalCommand_UnrecognizedReturnsEmpty(t *testing.T) {
	tests := []string{
		"make the auth module work",
		"fix all lint errors",
		"refactor the database layer",
	}
	for _, cond := range tests {
		cmd, ok := ResolveGoalCommand(cond, "")
		if ok {
			t.Errorf("ResolveGoalCommand(%q) ok = true, want false", cond)
		}
		if cmd != "" {
			t.Errorf("ResolveGoalCommand(%q) = %q, want empty", cond, cmd)
		}
	}
}
