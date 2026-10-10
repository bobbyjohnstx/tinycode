package llm

import (
	"errors"
	"time"
)

const (
	// DefaultStreamTokens is the completion cap for an OpenAI-compatible
	// stream when the caller did not set max_tokens. Reasoning and answer
	// text both count.
	DefaultStreamTokens = 4096
	// StreamWallClock is the longest one completion may run. A slow trickle
	// of tokens must not reset this.
	StreamWallClock = 5 * time.Minute
)

// ErrStreamBudget means the completion was closed because it passed the
// token cap or the wall clock. The events sent before this error are the
// partial completion.
var ErrStreamBudget = errors.New("stream budget exceeded")

// ApproxTokens estimates a token count from UTF-8 text. Four bytes is one token.
func ApproxTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

// StreamBudgetHit reports whether a completion should stop.
func StreamBudgetHit(tokens, limit int, elapsed, wall time.Duration) bool {
	if limit > 0 && tokens >= limit {
		return true
	}
	return wall > 0 && elapsed >= wall
}
