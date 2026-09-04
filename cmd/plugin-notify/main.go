package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// notifyArgs is the input schema for the notify tool.
type notifyArgs struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// newPlugin returns the notify plugin definition.
func newPlugin() plugin.Plugin {
	return plugin.Plugin{
		ID: "notify",
		Tools: []plugin.ToolDef{{
			Name:        "notify",
			Description: "Send a desktop notification",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":   map[string]any{"type": "string", "description": "Notification title"},
					"message": map[string]any{"type": "string", "description": "Notification body"},
				},
				"required": []string{"title", "message"},
			},
			Execute: executeNotify,
		}},
	}
}

// executeNotify sends a desktop notification using platform-specific tools.
func executeNotify(ctx context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
	var args notifyArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Title == "" || args.Message == "" {
		return "", fmt.Errorf("title and message are required")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(
			`display notification %s with title %s`,
			escapeAppleScript(args.Message),
			escapeAppleScript(args.Title),
		)
		cmd = exec.CommandContext(ctx, "osascript", "-e", script)
	case "linux":
		cmd = exec.CommandContext(ctx, "notify-send", args.Title, args.Message)
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("notification failed: %w", err)
	}
	return "Notification sent", nil
}

// escapeAppleScript returns s as a quoted AppleScript string literal, escaping
// backslashes and double-quotes to prevent injection.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func main() {
	plugin.Run(newPlugin())
}
