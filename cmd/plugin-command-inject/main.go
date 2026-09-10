package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

var descriptionRE = regexp.MustCompile(`(?i)^(?:#|//)\s*description:\s*(.+)$`)

// deriveToolName sanitizes a filename into a valid tool name.
func deriveToolName(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return strings.ToLower(sb.String())
}

// extractDescription reads the first 5 lines of a file looking for a description comment.
func extractDescription(filePath, filename string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Sprintf("Run %s", filename)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 5 && scanner.Scan(); i++ {
		m := descriptionRE.FindStringSubmatch(scanner.Text())
		if m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return fmt.Sprintf("Run %s", filename)
}

// scriptInfo holds metadata about a discovered script.
type scriptInfo struct {
	Path     string
	Filename string
}

// discoverScripts finds executable files in a directory.
func discoverScripts(dir string) ([]scriptInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var scripts []scriptInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		// Check executable bit
		if info.Mode().Perm()&0o111 == 0 {
			continue
		}
		scripts = append(scripts, scriptInfo{
			Path:     filepath.Join(dir, entry.Name()),
			Filename: entry.Name(),
		})
	}
	return scripts, nil
}

// injectArgs is the input schema for injected command tools.
type injectArgs struct {
	Args string `json:"args"`
}

// buildTools creates tool definitions from discovered scripts.
func buildTools(scripts []scriptInfo) []plugin.ToolDef {
	tools := make([]plugin.ToolDef, 0, len(scripts))
	for _, s := range scripts {
		script := s // capture
		toolName := deriveToolName(script.Filename)
		description := extractDescription(script.Path, script.Filename)

		tools = append(tools, plugin.ToolDef{
			Name:        toolName,
			Description: description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type":        "string",
						"description": "Raw arguments passed to the script",
					},
				},
			},
			Execute: func(ctx context.Context, raw json.RawMessage, tc plugin.ToolContext) (string, error) {
				var args injectArgs
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}

				var cmd *exec.Cmd
				if args.Args != "" {
					cmd = exec.CommandContext(ctx, script.Path, strings.Split(args.Args, " ")...)
				} else {
					cmd = exec.CommandContext(ctx, script.Path)
				}
				if tc.Directory != "" {
					cmd.Dir = tc.Directory
				}

				out, err := cmd.CombinedOutput()
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						return fmt.Sprintf("Error (exit code %d): %s", exitErr.ExitCode(), strings.TrimSpace(string(out))), nil
					}
					return "", fmt.Errorf("execution failed: %w", err)
				}
				return strings.TrimSpace(string(out)), nil
			},
		})
	}
	return tools
}

// newPlugin returns the command-inject plugin definition.
func newPlugin(dir string) plugin.Plugin {
	p := plugin.Plugin{
		ID: "command-inject",
	}

	if dir == "" {
		return p
	}

	scripts, err := discoverScripts(dir)
	if err != nil || len(scripts) == 0 {
		return p
	}

	p.Tools = buildTools(scripts)
	return p
}

func main() {
	dir := os.Getenv("COMMAND_INJECT_DIR")
	plugin.Run(newPlugin(dir))
}
