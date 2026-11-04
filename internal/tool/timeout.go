package tool

import (
	"encoding/json"
	"time"
)

// defaultBodyTimeout is the per-tool body deadline after Ask/hooks.
var defaultBodyTimeout = 60 * time.Second

// bodyTimeout returns (duration, true) to wrap def.Execute, or (_, false)
// to pass the parent ctx through unchanged (interactive question wait).
func bodyTimeout(name string, args json.RawMessage) (time.Duration, bool) {
	if name == "question" {
		return 0, false
	}
	// Subagent tasks manage their own lifetime via iteration limits and
	// step caps. The 60s body timeout is too short for remote LLM endpoints
	// (MAAS, OpenRouter) where a single completion can take 30-60s.
	if name == "task" {
		return 0, false
	}
	if name == "bash" {
		own := defaultShellTimeout
		var parsed struct {
			Timeout *int `json:"timeout"`
		}
		_ = json.Unmarshal(args, &parsed)
		if parsed.Timeout != nil {
			t := time.Duration(*parsed.Timeout) * time.Millisecond
			if t > maxShellTimeout {
				t = maxShellTimeout
			}
			if t > 0 {
				own = t
			}
		}
		if own < defaultBodyTimeout {
			return defaultBodyTimeout, true
		}
		return own, true
	}
	return defaultBodyTimeout, true
}
