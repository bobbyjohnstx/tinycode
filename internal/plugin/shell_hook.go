package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const maxAdditionalContextLen = 10000

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
// Returns collected additionalContext strings and an error if any hook exits
// non-zero (abort signal). Non-JSON stdout is treated as no context (backward compat).
func (r *ShellHookRunner) RunBefore(eventName string, vars map[string]string) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	hooks, ok := r.hooks[eventName]
	if !ok || len(hooks) == 0 {
		return nil, nil
	}

	var collected []string
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
			return nil, fmt.Errorf("shell hook %q aborted: %w", cmd, err)
		}
		if out != "" {
			r.logger.Debug("shell hook output", "event", eventName, "command", cmd, "output", out)
			if ctx := parseAdditionalContext([]byte(out)); len(ctx) > 0 {
				collected = append(collected, ctx...)
			}
		}
	}
	return collected, nil
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

		safego.Go(func() {
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
		})
	}
}

// RunAfterSync executes shell hooks for an "after" event synchronously,
// capturing stdout and parsing additionalContext from JSON output.
// Unlike RunAfter, errors are logged but do not abort — this is for events
// where context capture is needed but failure is non-fatal.
func (r *ShellHookRunner) RunAfterSync(eventName string, vars map[string]string) []string {
	if r == nil {
		return nil
	}
	hooks, ok := r.hooks[eventName]
	if !ok || len(hooks) == 0 {
		return nil
	}

	var collected []string
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
			r.logger.Warn("shell hook failed",
				"event", eventName,
				"command", cmd,
				"output", out,
				"error", err,
			)
		}
		if out != "" {
			r.logger.Debug("shell hook output", "event", eventName, "command", cmd, "output", out)
			if ctx := parseAdditionalContext([]byte(out)); len(ctx) > 0 {
				collected = append(collected, ctx...)
			}
		}
	}
	return collected
}

// parseAdditionalContext attempts to extract additionalContext strings from
// JSON output matching the hookSpecificOutput.additionalContext structure.
// Returns nil for non-JSON or JSON without the expected field (backward compat).
// Each string is capped at 10,000 characters.
func parseAdditionalContext(output []byte) []string {
	var parsed struct {
		HookSpecificOutput struct {
			AdditionalContext []string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil
	}
	ctx := parsed.HookSpecificOutput.AdditionalContext
	if len(ctx) == 0 {
		return nil
	}
	for i, s := range ctx {
		if len(s) > maxAdditionalContextLen {
			ctx[i] = s[:maxAdditionalContextLen] + "... [truncated]"
		}
	}
	return ctx
}

// Hooks returns the configured hooks map.
func (r *ShellHookRunner) Hooks() map[string][]config.HookConfig {
	if r == nil {
		return nil
	}
	return r.hooks
}

// shellQuote wraps s in POSIX single quotes, escaping any internal single
// quotes with the '\'' idiom. This prevents shell metacharacter injection
// when substituting untrusted values into sh -c commands.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// substituteVars replaces $VAR placeholders in the command string.
// All values are shell-quoted to prevent command injection.
func substituteVars(command string, vars map[string]string) string {
	result := command
	for k, v := range vars {
		result = strings.ReplaceAll(result, "$"+k, shellQuote(v))
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
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
