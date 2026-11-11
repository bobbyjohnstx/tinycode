package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// notifyMu protects lastNotifyTime for concurrent access.
var notifyMu sync.Mutex

// lastNotifyTime tracks the last notification for rate limiting.
var lastNotifyTime time.Time

// notifyMinInterval is the minimum interval between notifications.
const notifyMinInterval = 5 * time.Second

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
					"urgency": map[string]any{"type": "string", "enum": []string{"low", "normal", "critical"}, "description": "Notification urgency level"},
				},
				"required": []string{"title", "message"},
			},
			Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var args struct {
					Title   string `json:"title"`
					Message string `json:"message"`
					Urgency string `json:"urgency"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
				if args.Title == "" || args.Message == "" {
					return "", fmt.Errorf("title and message are required")
				}
				if args.Urgency == "" {
					args.Urgency = "normal"
				}
				return SendNotification(ctx, args.Title, args.Message, args.Urgency)
			},
		},
	}
}

func (p *notifyPlugin) Hooks() BuiltinHooks { return BuiltinHooks{} }

// defaultRunNotifyCommand executes a notification command for real.
func defaultRunNotifyCommand(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// runNotifyCommand, when non-nil, replaces defaultRunNotifyCommand. Tests
// install a fake here to record the command without running it.
var runNotifyCommand func(ctx context.Context, name string, args ...string) error

// ringBell prints the terminal bell. A variable so tests can silence it.
var ringBell = func() { fmt.Print("\a") }

// SendNotification sends a desktop notification with rate limiting.
// Exported so TUI auto-notification hooks can call it directly.
func SendNotification(ctx context.Context, title, message, urgency string) (string, error) {
	notifyMu.Lock()
	if time.Since(lastNotifyTime) < notifyMinInterval {
		notifyMu.Unlock()
		return "Notification rate-limited (max 1 per 5 seconds)", nil
	}
	lastNotifyTime = time.Now()
	notifyMu.Unlock()

	// Safety net: under `go test` do not run any real command or bell unless a
	// test has explicitly installed a fake runner. This keeps notifications from
	// popping up on a developer's desktop or ringing the terminal bell.
	if testing.Testing() && runNotifyCommand == nil {
		return "Notification suppressed (test)", nil
	}

	var cmdName string
	var cmdArgs []string
	switch {
	case runtime.GOOS == "darwin":
		script := fmt.Sprintf(`display notification %s with title %s`,
			escapeAppleScript(message), escapeAppleScript(title))
		if urgency == "critical" {
			script += ` sound name "Ping"`
		}
		cmdName = "osascript"
		cmdArgs = []string{"-e", script}

	case runtime.GOOS == "linux" && isWSL():
		// WSL: try notify-send first (works on many WSL setups with WSLg),
		// fall back to terminal bell.
		if path, err := exec.LookPath("notify-send"); err == nil {
			cmdName = path
			cmdArgs = []string{"--urgency=" + urgency, title, message}
		} else {
			ringBell()
			return "Notification sent (terminal bell — WSL without notify-send)", nil
		}

	case runtime.GOOS == "linux":
		cmdName = "notify-send"
		cmdArgs = []string{"--urgency=" + urgency, title, message}

	default:
		ringBell()
		return "Notification sent (terminal bell)", nil
	}

	runner := runNotifyCommand
	if runner == nil {
		runner = defaultRunNotifyCommand
	}
	if err := runner(ctx, cmdName, cmdArgs...); err != nil {
		// If the notification command fails, fall back to terminal bell.
		ringBell()
		return "Notification sent (terminal bell — command failed)", nil
	}
	return "Notification sent", nil
}

// isWSL returns true when running inside Windows Subsystem for Linux.
func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "'", "'\\''")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return `"` + s + `"`
}
