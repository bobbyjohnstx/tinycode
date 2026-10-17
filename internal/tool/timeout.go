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
