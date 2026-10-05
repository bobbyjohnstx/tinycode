package session

import (
	"os"
	"path/filepath"
	"strings"
)

// GoalState tracks an active goal condition for autonomous multi-turn execution.
type GoalState struct {
	Text          string // natural language condition
	Command       string // shell command to evaluate (exit 0 = met)
	MaxIterations int    // cap per goal (default 10)
	Iteration     int    // current iteration count
}

const defaultGoalMaxIterations = 10

// DefaultGoalMaxIterations returns the default max iterations for a goal.
func DefaultGoalMaxIterations() int {
	return defaultGoalMaxIterations
}

// goalCategory groups related natural language patterns under a single intent.
type goalCategory struct {
	patterns []string
	commands map[string]string // ecosystem -> command
}

// goalCategories defines pattern-to-command mappings per ecosystem.
// The ecosystem is detected from project indicator files in the working directory.
var goalCategories = []goalCategory{
	{
		patterns: []string{"tests pass", "all tests pass", "test passes"},
		commands: map[string]string{
			"node":   "npm test",
			"python": "pytest",
			"rust":   "cargo test",
			"make":   "make test",
			"go":     "go test ./... -count=1",
		},
	},
	{
		patterns: []string{"build succeeds", "build passes", "builds"},
		commands: map[string]string{
			"node":   "npm run build",
			"rust":   "cargo build",
			"make":   "make build",
			"go":     "go build ./...",
		},
	},
	{
		patterns: []string{"no lint errors", "lint passes", "lint clean"},
		commands: map[string]string{
			"node":   "npm run lint",
			"rust":   "cargo clippy",
			"make":   "make lint",
			"go":     "go vet ./...",
		},
	},
	{
		patterns: []string{"no vet errors", "vet passes", "vet clean"},
		commands: map[string]string{
			"go": "go vet ./...",
		},
	},
}

// ecosystemIndicators maps indicator filenames to ecosystem keys.
// Order matters: first match wins.
var ecosystemIndicators = []struct {
	file      string
	ecosystem string
}{
	{"package.json", "node"},
	{"Cargo.toml", "rust"},
	{"requirements.txt", "python"},
	{"pyproject.toml", "python"},
	{"setup.py", "python"},
	{"Makefile", "make"},
	{"go.mod", "go"},
}

// detectEcosystem checks for indicator files in dir and returns the ecosystem key.
// Returns "go" if no indicators are found.
func detectEcosystem(dir string) string {
	if dir == "" {
		return "go"
	}
	for _, ind := range ecosystemIndicators {
		if _, err := os.Stat(filepath.Join(dir, ind.file)); err == nil {
			return ind.ecosystem
		}
	}
	return "go"
}

// ResolveGoalCommand maps a natural language condition to a shell command,
// detecting the project ecosystem from indicator files in dir.
// Returns the command and true if a known pattern matched, or ("", false)
// if the condition is unrecognized and needs LLM resolution.
func ResolveGoalCommand(condition, dir string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(condition))
	eco := detectEcosystem(dir)
	for _, cat := range goalCategories {
		for _, pattern := range cat.patterns {
			if strings.Contains(lower, pattern) {
				if cmd, ok := cat.commands[eco]; ok {
					return cmd, true
				}
				return "", false
			}
		}
	}
	return "", false
}

// GoalDoomThreshold returns the raised doom loop threshold when a goal is active.
// The safety net stays on — it is raised, not disabled.
func GoalDoomThreshold(base int) int {
	return base * 2
}
