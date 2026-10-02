package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

const defaultShellHookTimeout = 10 * time.Second

// ShellHookRunner executes shell commands configured as hooks in settings.json.
type ShellHookRunner struct {
	hooks  map[string][]config.HookConfig
	logger *slog.Logger
}

// NewShellHookRunner creates a runner from the hooks config map.
// Returns nil if hooks is empty.
func NewShellHookRunner(hooks map[string][]config.HookConfig, logger *slog.Logger) *ShellHookRunner {
	if len(hooks) == 0 {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ShellHookRunner{hooks: hooks, logger: logger}
}

// RunBefore executes shell hooks for a "before" event synchronously.
// Returns an error if any hook exits non-zero (abort signal).
// The vars map provides variable substitutions ($TOOL, $SESSION_ID, etc.).
func (r *ShellHookRunner) RunBefore(eventName string, vars map[string]string) error {
	if r == nil {
		return nil
	}
	hooks, ok := r.hooks[eventName]
	if !ok || len(hooks) == 0 {
		return nil
	}

	for _, h := range hooks {
		if !matchesFilter(h.Match, vars) {
			continue
		}
		cmd := substituteVars(h.Command, vars)
		timeout := shellHookTimeout(h.Timeout)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		out, err := execShellCommand(ctx, cmd)
		cancel()

		if err != nil {
			r.logger.Warn("shell hook aborted",
				"event", eventName,
				"command", cmd,
				"output", out,
				"error", err,
			)
			return fmt.Errorf("shell hook %q aborted: %w", cmd, err)
		}
		if out != "" {
			r.logger.Debug("shell hook output", "event", eventName, "command", cmd, "output", out)
		}
	}
	return nil
}

// RunAfter executes shell hooks for an "after" event asynchronously.
// Errors are logged but do not block.
func (r *ShellHookRunner) RunAfter(eventName string, vars map[string]string) {
	if r == nil {
		return
	}
	hooks, ok := r.hooks[eventName]
	if !ok || len(hooks) == 0 {
		return
	}

	for _, h := range hooks {
		if !matchesFilter(h.Match, vars) {
			continue
		}
		cmd := substituteVars(h.Command, vars)
		timeout := shellHookTimeout(h.Timeout)
		logger := r.logger

		go func(cmd string, timeout time.Duration) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			out, err := execShellCommand(ctx, cmd)
			if err != nil {
				logger.Warn("shell hook failed",
					"event", eventName,
					"command", cmd,
					"output", out,
					"error", err,
				)
			} else if out != "" {
				logger.Debug("shell hook output", "event", eventName, "command", cmd, "output", out)
			}
		}(cmd, timeout)
	}
}

// Hooks returns the configured hooks map.
func (r *ShellHookRunner) Hooks() map[string][]config.HookConfig {
	if r == nil {
		return nil
	}
	return r.hooks
}

// substituteVars replaces $VAR placeholders in the command string.
func substituteVars(command string, vars map[string]string) string {
	result := command
	for k, v := range vars {
		result = strings.ReplaceAll(result, "$"+k, v)
	}
	return result
}

// matchesFilter checks whether all match filter keys are satisfied by vars.
func matchesFilter(match map[string]string, vars map[string]string) bool {
	for k, want := range match {
		got, ok := vars[strings.ToUpper(k)]
		if !ok {
			// Also check lowercase key directly.
			got, ok = vars[k]
		}
		if !ok || got != want {
			return false
		}
	}
	return true
}

// shellHookTimeout returns the timeout duration for a hook.
func shellHookTimeout(seconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultShellHookTimeout
}

// execShellCommand runs a command via sh -c and returns combined output.
func execShellCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
