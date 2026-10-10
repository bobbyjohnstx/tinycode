package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"sync"
)

type recentCall struct {
	seq     int
	output  string
	isError bool
}

type contextPruningPlugin struct {
	mu        sync.Mutex
	seen      map[string]recentCall // request key -> last result
	seq       int
	threshold int
}

// NewContextPruningPlugin creates a builtin plugin that remembers recent tool
// calls. A repeat of the same tool and arguments returns the earlier result
// instead of a note that drops it.
func NewContextPruningPlugin() BuiltinPlugin {
	threshold := 20
	if val := os.Getenv("CONTEXT_PRUNE_THRESHOLD"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			threshold = n
		}
	}
	return &contextPruningPlugin{
		seen:      make(map[string]recentCall),
		threshold: threshold,
	}
}

func (p *contextPruningPlugin) ID() string { return "context-pruning" }

func (p *contextPruningPlugin) Tools() []BuiltinTool { return nil }

func (p *contextPruningPlugin) Hooks() BuiltinHooks {
	return BuiltinHooks{
		ToolExecAfter: func(_ context.Context, toolName, toolArgs, output string, isError bool) (string, bool, bool) {
			key := requestKey(toolName, toolArgs)

			p.mu.Lock()
			defer p.mu.Unlock()

			p.seq++

			// Sliding window eviction: keep the map bounded to ~threshold entries.
			if p.seq > 2*p.threshold {
				cutoff := p.seq - p.threshold
				for k, v := range p.seen {
					if v.seq < cutoff {
						delete(p.seen, k)
					}
				}
			}

			prev, exists := p.seen[key]
			if exists && (p.seq-prev.seq) <= p.threshold {
				prev.seq = p.seq
				p.seen[key] = prev
				return prev.output, prev.isError, true
			}
			p.seen[key] = recentCall{seq: p.seq, output: output, isError: isError}
			return "", false, false
		},
	}
}

func requestKey(toolName, toolArgs string) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0})
	h.Write([]byte(toolArgs))
	return hex.EncodeToString(h.Sum(nil))
}
