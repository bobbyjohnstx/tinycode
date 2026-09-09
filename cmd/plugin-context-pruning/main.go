package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// tracker detects duplicate tool calls within a sliding window.
type tracker struct {
	mu        sync.Mutex
	seen      map[string]int // hash -> sequence number of last occurrence
	seq       int
	threshold int
}

func newTracker(threshold int) *tracker {
	return &tracker{
		seen:      make(map[string]int),
		threshold: threshold,
	}
}

// record tracks a tool call and returns true if it is a duplicate of a recent call.
func (t *tracker) record(toolName, toolArgs string) bool {
	h := hashCall(toolName, toolArgs)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	prev, exists := t.seen[h]
	t.seen[h] = t.seq

	if !exists {
		return false
	}
	return (t.seq - prev) <= t.threshold
}

// hashCall produces a deterministic hash of the tool name and args.
func hashCall(toolName, toolArgs string) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0})
	h.Write([]byte(toolArgs))
	return hex.EncodeToString(h.Sum(nil))
}

// getThreshold reads the threshold from env or returns the default.
func getThreshold() int {
	val := os.Getenv("CONTEXT_PRUNE_THRESHOLD")
	if val == "" {
		return 20
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return 20
	}
	return n
}

// newPlugin returns the context-pruning plugin definition.
func newPlugin() plugin.Plugin {
	t := newTracker(getThreshold())
	return plugin.Plugin{
		ID: "context-pruning",
		Hooks: plugin.HookHandlers{
			ToolExecAfter: func(_ context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
				if t.record(input.ToolName, input.Output) {
					return &plugin.ToolExecAfterOutput{
						Output:  fmt.Sprintf("[note: duplicate of recent %s call]\n%s", input.ToolName, input.Output),
						IsError: input.IsError,
					}, nil
				}
				return nil, nil
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}
