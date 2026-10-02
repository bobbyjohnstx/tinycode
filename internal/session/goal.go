package session

import "strings"

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

// goalPatterns maps known natural language conditions to shell commands.
var goalPatterns = map[string]string{
	"tests pass":      "go test ./... -count=1",
	"all tests pass":  "go test ./... -count=1",
	"build succeeds":  "go build ./...",
	"build passes":    "go build ./...",
	"no lint errors":  "go vet ./...",
	"lint passes":     "go vet ./...",
	"lint clean":      "go vet ./...",
	"no vet errors":   "go vet ./...",
	"vet passes":      "go vet ./...",
	"vet clean":       "go vet ./...",
}

// ResolveGoalCommand maps a natural language condition to a shell command.
// Returns the command and true if a known pattern matched, or ("", false)
// if the condition is unrecognized and needs LLM resolution.
func ResolveGoalCommand(condition string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(condition))
	for pattern, cmd := range goalPatterns {
		if strings.Contains(lower, pattern) {
			return cmd, true
		}
	}
	return "", false
}

// GoalDoomThreshold returns the raised doom loop threshold when a goal is active.
// The safety net stays on — it is raised, not disabled.
func GoalDoomThreshold(base int) int {
	return base * 2
}
