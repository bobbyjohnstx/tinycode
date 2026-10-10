package llm

import (
	"errors"
	"strings"
	"time"
)

const (
	// DefaultStreamTokens is the completion cap for an OpenAI-compatible
	// stream when the caller did not set max_tokens. Reasoning and answer
	// text both count.
	DefaultStreamTokens = 8192
	// StreamWallClock is the longest one completion may run. A slow trickle
	// of tokens must not reset this.
	StreamWallClock = 8 * time.Minute

	// reasoningStallCount is how many times "but wait" may appear in one
	// reasoning trace before that trace is treated as stuck.
	reasoningStallCount = 4
	// reasoningRepeatMin is the shortest sentence that can count as a repeat.
	reasoningRepeatMin = 48
	// reasoningRepeatCount is how many identical sentences end the trace.
	reasoningRepeatCount = 3
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

// ReasoningLoop reports whether a reasoning trace is repeating itself.
// A few second guesses are normal. The same stall phrase, or the same
// long sentence, over and over is a loop.
func ReasoningLoop(text string) bool {
	norm := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if norm == "" {
		return false
	}
	if strings.Count(norm, "but wait") >= reasoningStallCount {
		return true
	}
	counts := map[string]int{}
	for _, sentence := range reasoningSentences(norm) {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < reasoningRepeatMin {
			continue
		}
		counts[sentence]++
		if counts[sentence] >= reasoningRepeatCount {
			return true
		}
	}
	return false
}

func reasoningSentences(norm string) []string {
	return strings.FieldsFunc(norm, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
}

// StreamBudgetHit reports whether a completion should stop.
func StreamBudgetHit(tokens, limit int, elapsed, wall time.Duration) bool {
	if limit > 0 && tokens >= limit {
		return true
	}
	return wall > 0 && elapsed >= wall
}
