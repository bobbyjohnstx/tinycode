package main

import (
	"context"
	"regexp"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

// dangerousPatterns defines shell commands that should be blocked.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-\w*\s+)*-r\w*\s+(-\w+\s+)*/\s*$`),
	regexp.MustCompile(`\brm\s+(-\w*\s+)*-r\w*\s+(-\w+\s+)*/\b`),
	regexp.MustCompile(`\brm\s+[^\n]*--recursive\b[^\n]*\s/(?:\b|\s*$)`),
	regexp.MustCompile(`\bchmod\s+(?:-+\S+\s+)*777\b`),
	regexp.MustCompile(`\bfind\b[^\n]*\s-delete\b`),
	regexp.MustCompile(`\bgit\s+clean\b[^\n]*\s-[a-zA-Z]*f`),
	regexp.MustCompile(`\bgit\s+clean\b[^\n]*--force\b`),
	regexp.MustCompile(`\bmkfs\b`),
	regexp.MustCompile(`\bdd\s+if=`),
	regexp.MustCompile(`\bshutdown\b`),
	regexp.MustCompile(`\breboot\b`),
	regexp.MustCompile(`:\(\)\{\s*:\|:&\s*\};:`),
}

// matchDangerous returns the first dangerous pattern matched in cmd, or nil.
func matchDangerous(cmd string) *regexp.Regexp {
	for _, p := range dangerousPatterns {
		if p.MatchString(cmd) {
			return p
		}
	}
	return nil
}

func isShellTool(name string) bool {
	switch name {
	case "bash", "shell":
		return true
	default:
		return false
	}
}

func checkPermission(toolName, toolArgs string) *plugin.PermissionOutput {
	if !isShellTool(toolName) {
		return &plugin.PermissionOutput{Allowed: true}
	}
	if p := matchDangerous(toolArgs); p != nil {
		return &plugin.PermissionOutput{
			Allowed: false,
			Reason:  "blocked by safety-net: command matches dangerous pattern " + p.String(),
		}
	}
	return &plugin.PermissionOutput{Allowed: true}
}

func main() {
	plugin.Run(plugin.Plugin{
		ID: "safety-net",
		Hooks: plugin.HookHandlers{
			PermissionAsk: func(_ context.Context, input plugin.PermissionInput) (*plugin.PermissionOutput, error) {
				return checkPermission(input.ToolName, input.ToolArgs), nil
			},
		},
	})
}
