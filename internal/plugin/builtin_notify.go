package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type notifyPlugin struct{}

// NewNotifyPlugin creates a builtin plugin that provides a desktop notification tool.
func NewNotifyPlugin() BuiltinPlugin {
	return &notifyPlugin{}
}

func (p *notifyPlugin) ID() string { return "notify" }

func (p *notifyPlugin) Tools() []BuiltinTool {
	return []BuiltinTool{
		{
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
			Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var args struct {
					Title   string `json:"title"`
					Message string `json:"message"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
				if args.Title == "" || args.Message == "" {
					return "", fmt.Errorf("title and message are required")
				}

				var cmd *exec.Cmd
				switch runtime.GOOS {
				case "darwin":
					script := fmt.Sprintf(`display notification %s with title %s`,
						escapeAppleScript(args.Message), escapeAppleScript(args.Title))
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
			},
		},
	}
}

func (p *notifyPlugin) Hooks() BuiltinHooks { return BuiltinHooks{} }

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
