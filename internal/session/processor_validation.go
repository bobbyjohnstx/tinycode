package session

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
)

const (
	defaultDoomThreshold   = 3
	defaultAutoContinueMax = 0
)

// toolCallSignature captures the identity of a tool call for doom-loop detection.
type toolCallSignature struct {
	Name string
	Args string
}

func (p *Processor) doomThreshold() int {
	if p.config.DoomThreshold > 0 {
		return p.config.DoomThreshold
	}
	return defaultDoomThreshold
}

func (p *Processor) autoContinueLimit() int {
	if p.config.AutoContinueMax < 0 {
		return 0 // explicitly disabled
	}
	if p.config.AutoContinueMax > 0 {
		return p.config.AutoContinueMax
	}
	return defaultAutoContinueMax
}

// isDoomLoop checks if the last N tool call signatures are all identical.
func isDoomLoop(recent []toolCallSignature, threshold int) bool {
	if len(recent) < threshold {
		return false
	}
	tail := recent[len(recent)-threshold:]
	first := tail[0]
	for _, sig := range tail[1:] {
		if sig.Name != first.Name || sig.Args != first.Args {
			return false
		}
	}
	return true
}

// checkExternalDirectory returns a denial message if the tool call targets a
// path outside the configured directory and the permission service denies it.
// Returns "" if allowed.
func (p *Processor) checkExternalDirectory(ctx context.Context, call Part) string {
	if p.config.Directory == "" || p.config.Perms == nil {
		return ""
	}

	paths := extractPathsFromArgs(call.ToolArgs)
	for _, path := range paths {
		if !isInsideDirectory(path, p.config.Directory) {
			askID, _ := id.Ascending("perm")
			err := p.config.Perms.Ask(ctx, permission.AskInput{
				ID:         askID,
				SessionID:  p.config.SessionID,
				Permission: "external_directory",
				Patterns:   []string{path},
				Metadata: map[string]any{
					"tool": call.ToolName,
					"path": path,
				},
				Ruleset: p.config.Ruleset,
			})
			if err != nil {
				return fmt.Sprintf("permission denied: tool %q targets path %q outside project directory %q", call.ToolName, path, p.config.Directory)
			}
		}
	}
	return ""
}

// extractPathsFromArgs extracts file path values from a tool call's JSON args.
func extractPathsFromArgs(argsJSON string) []string {
	var paths []string
	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil
	}
	for _, key := range []string{"file_path", "path", "file", "directory", "dir", "folder", "target", "destination", "source", "src", "dest", "location", "root", "base_path", "working_directory", "cwd"} {
		raw, ok := args[key]
		if !ok {
			continue
		}
		var val string
		if json.Unmarshal(raw, &val) == nil && val != "" {
			paths = append(paths, val)
		}
	}
	return paths
}

// isInsideDirectory reports whether path is inside the directory (or is the
// directory itself). Both paths are cleaned before comparison.
func isInsideDirectory(path, directory string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}
