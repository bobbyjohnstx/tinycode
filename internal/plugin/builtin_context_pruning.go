package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync"
)

type contextPruningPlugin struct {
	mu        sync.Mutex
	seen      map[string]int // hash -> sequence number of last occurrence
	seq       int
	threshold int
}

// NewContextPruningPlugin creates a builtin plugin that detects duplicate tool
// calls within a sliding window and replaces the repeated output with a note.
func NewContextPruningPlugin() BuiltinPlugin {
	threshold := 20
	if val := os.Getenv("CONTEXT_PRUNE_THRESHOLD"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			threshold = n
		}
	}
	return &contextPruningPlugin{
		seen:      make(map[string]int),
		threshold: threshold,
	}
}

func (p *contextPruningPlugin) ID() string { return "context-pruning" }

func (p *contextPruningPlugin) Tools() []BuiltinTool { return nil }

func (p *contextPruningPlugin) Hooks() BuiltinHooks {
	return BuiltinHooks{
		ToolExecAfter: func(_ context.Context, toolName, output string, isError bool) (string, bool, bool) {
			h := sha256.New()
			h.Write([]byte(toolName))
			h.Write([]byte{0})
			h.Write([]byte(output))
			hash := hex.EncodeToString(h.Sum(nil))

			p.mu.Lock()
			defer p.mu.Unlock()

			p.seq++

			// Sliding window eviction: keep the map bounded to ~threshold entries.
			if p.seq > 2*p.threshold {
				cutoff := p.seq - p.threshold
				for k, v := range p.seen {
					if v < cutoff {
						delete(p.seen, k)
					}
				}
			}

			prev, exists := p.seen[hash]
			p.seen[hash] = p.seq

			if exists && (p.seq-prev) <= p.threshold {
				return fmt.Sprintf("[note: duplicate of recent %s call]", toolName), isError, true
			}
			return "", false, false
		},
	}
}
