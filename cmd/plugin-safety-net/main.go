package main

import (
	"context"
	"regexp"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// dangerousPatterns defines shell commands that should be blocked.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-\w*\s+)*-r\w*\s+(-\w+\s+)*/\s*$`),
	regexp.MustCompile(`\brm\s+(-\w*\s+)*-r\w*\s+(-\w+\s+)*/\b`),
	regexp.MustCompile(`\bchmod\s+777\b`),
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

func main() {
	plugin.Run(plugin.Plugin{
		ID: "safety-net",
		Hooks: plugin.HookHandlers{
			PermissionAsk: func(_ context.Context, input plugin.PermissionInput) (*plugin.PermissionOutput, error) {
				if p := matchDangerous(input.ToolArgs); p != nil {
					return &plugin.PermissionOutput{
						Allowed: false,
						Reason:  "blocked by safety-net: command matches dangerous pattern " + p.String(),
					}, nil
				}
				return nil, nil
			},
		},
	})
}
