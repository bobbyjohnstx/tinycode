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
	"github.com/bobbyjohnstx/tinycode/internal/procenv"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const maxAdditionalContextLen = 10000

const maxContinueOutputLen = 4096

const defaultShellHookTimeout = 10 * time.Second

// ShellHookRunner executes shell commands configured as hooks in settings.json.
// When agent hooks are set via SetAgentHooks, they run after config hooks.
type ShellHookRunner struct {
	hooks      map[string][]config.HookConfig
	agentHooks map[string][]config.HookConfig
	agentName  string
	logger     *slog.Logger
	trustStore *HookTrustStore
	projectDir string
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

// WithTrust attaches a trust store and project directory to the runner.
// Project hooks are checked against the store before execution.
// User-config hooks (no trust store) always run.
func (r *ShellHookRunner) WithTrust(store *HookTrustStore, projectDir string) *ShellHookRunner {
	if r == nil {
		return nil
	}
	r.trustStore = store
	r.projectDir = projectDir
	return r
}

// SetAgentHooks installs agent-scoped hooks that run after config hooks.
// Pass nil hooks and empty agentName to clear agent hooks (e.g. on agent switch).
func (r *ShellHookRunner) SetAgentHooks(hooks map[string][]config.HookConfig, agentName string) {
	if r == nil {
		return
	}
	r.agentHooks = hooks
	r.agentName = agentName
}

// shouldRun checks whether a hook is explicitly disabled in the trust store.
// Untrusted hooks are allowed to run — the trust prompt UI is not yet
// implemented, so blocking untrusted hooks would silently break all
// existing project hooks. Once the approval prompt lands, this will
// also return false for hooks that are neither trusted nor disabled.
func (r *ShellHookRunner) shouldRun(command, eventName string) bool {
	if r.trustStore == nil || r.projectDir == "" {
		return true
	}
	if r.trustStore.IsDisabled(command, eventName, r.projectDir) {
		return false
	}
	return true
}

// shouldRunAgent checks whether an agent hook is disabled using an agent-aware hash.
func (r *ShellHookRunner) shouldRunAgent(command, eventName string) bool {
	if r.trustStore == nil || r.projectDir == "" {
		return true
	}
	if r.trustStore.IsDisabledAgent(command, eventName, r.projectDir, r.agentName) {
		return false
	}
	return true
}

// RunBefore executes shell hooks for a "before" event synchronously.
// Returns collected additionalContext strings and an error if any hook exits
// non-zero (abort signal). Non-JSON stdout is treated as no context (backward compat).
//
// When a hook has Continue set and exits non-zero, the tool call is still
// skipped but the error includes the hook's stdout (truncated to 4 KiB)
// so the model can adapt.
func (r *ShellHookRunner) RunBefore(eventName string, vars map[string]string) ([]string, error) {
	if r == nil {
		return nil, nil
	}

	// Run config hooks first.
	collected, err := r.runBeforeHooks(r.hooks[eventName], eventName, vars, false)
	if err != nil {
		return nil, err
	}

	// Run agent hooks after config hooks.
	agentCtx, err := r.runBeforeHooks(r.agentHooks[eventName], eventName, vars, true)
	if err != nil {
		return nil, err
	}
	collected = append(collected, agentCtx...)

	return collected, nil
}

// runBeforeHooks runs a slice of hooks for a before event. When isAgent is
// true, the agent-aware trust check is used.
func (r *ShellHookRunner) runBeforeHooks(hooks []config.HookConfig, eventName string, vars map[string]string, isAgent bool) ([]string, error) {
	if len(hooks) == 0 {
		return nil, nil
	}

	var collected []string
	for _, h := range hooks {
		if !matchesFilter(h.Match, vars) {
			continue
		}
		id := hookIdentifier(h)
		if isAgent {
			if !r.shouldRunAgent(id, eventName) {
				continue
			}
		} else {
			if !r.shouldRun(id, eventName) {
				continue
			}
		}

		timeout := shellHookTimeout(h.Timeout)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		out, err := execHook(ctx, h, vars)
		cancel()

		displayName := hookDisplayName(h)

		if err != nil {
			if h.Continue {
				reason := out
				if len(reason) > maxContinueOutputLen {
					reason = reason[:maxContinueOutputLen] + "... [truncated]"
				}
				msg := reason
				if msg != "" {
					msg += "\n"
				}
				msg += fmt.Sprintf("[hook blocked: %s]", displayName)
				r.logger.Info("shell hook blocked (continue)",
					"event", eventName,
					"command", displayName,
					"output", out,
				)
				return nil, fmt.Errorf("%s", msg)
			}
			r.logger.Warn("shell hook aborted",
				"event", eventName,
				"command", displayName,
				"output", out,
				"error", err,
			)
			return nil, fmt.Errorf("shell hook %q aborted: %w", displayName, err)
		}
		if out != "" {
			r.logger.Debug("shell hook output", "event", eventName, "command", displayName, "output", out)
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
	r.runAfterHooks(r.hooks[eventName], eventName, vars, false)
	r.runAfterHooks(r.agentHooks[eventName], eventName, vars, true)
}

// runAfterHooks fires a slice of hooks asynchronously.
func (r *ShellHookRunner) runAfterHooks(hooks []config.HookConfig, eventName string, vars map[string]string, isAgent bool) {
	for _, h := range hooks {
		if !matchesFilter(h.Match, vars) {
			continue
		}
		id := hookIdentifier(h)
		if isAgent {
			if !r.shouldRunAgent(id, eventName) {
				continue
			}
		} else {
			if !r.shouldRun(id, eventName) {
				continue
			}
		}
		hook := h
		timeout := shellHookTimeout(h.Timeout)
		logger := r.logger

		safego.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			displayName := hookDisplayName(hook)
			out, err := execHook(ctx, hook, vars)
			if err != nil {
				logger.Warn("shell hook failed",
					"event", eventName,
					"command", displayName,
					"output", out,
					"error", err,
				)
			} else if out != "" {
				logger.Debug("shell hook output", "event", eventName, "command", displayName, "output", out)
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

	collected := r.runAfterSyncHooks(r.hooks[eventName], eventName, vars, false)
	agentCtx := r.runAfterSyncHooks(r.agentHooks[eventName], eventName, vars, true)
	return append(collected, agentCtx...)
}

// runAfterSyncHooks runs a slice of hooks synchronously, collecting context.
func (r *ShellHookRunner) runAfterSyncHooks(hooks []config.HookConfig, eventName string, vars map[string]string, isAgent bool) []string {
	if len(hooks) == 0 {
		return nil
	}

	var collected []string
	for _, h := range hooks {
		if !matchesFilter(h.Match, vars) {
			continue
		}
		id := hookIdentifier(h)
		if isAgent {
			if !r.shouldRunAgent(id, eventName) {
				continue
			}
		} else {
			if !r.shouldRun(id, eventName) {
				continue
			}
		}

		timeout := shellHookTimeout(h.Timeout)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		out, err := execHook(ctx, h, vars)
		cancel()

		displayName := hookDisplayName(h)
		if err != nil {
			r.logger.Warn("shell hook failed",
				"event", eventName,
				"command", displayName,
				"output", out,
				"error", err,
			)
		}
		if out != "" {
			r.logger.Debug("shell hook output", "event", eventName, "command", displayName, "output", out)
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
// quotes with the '\” idiom. This prevents shell metacharacter injection
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

// hookIdentifier returns a stable string key for a hook, used for trust
// hashing. Command hooks return the command string; args hooks return the
// arguments joined with null bytes (unambiguous separator).
func hookIdentifier(h config.HookConfig) string {
	if h.Command != "" {
		return h.Command
	}
	return strings.Join(h.Args, "\x00")
}

// hookDisplayName returns a human-readable label for log messages.
func hookDisplayName(h config.HookConfig) string {
	if h.Command != "" {
		return h.Command
	}
	return strings.Join(h.Args, " ")
}

// execHook runs a hook using either the shell (Command) or direct exec (Args).
func execHook(ctx context.Context, h config.HookConfig, vars map[string]string) (string, error) {
	if h.Command != "" {
		return execShellCommand(ctx, substituteVars(h.Command, vars))
	}
	return execArgsCommand(ctx, substituteVarsArgs(h.Args, vars))
}

// execShellCommand runs a command via sh -c and returns combined output.
func execShellCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = procenv.Child(nil)
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// execArgsCommand runs a command directly via exec (no shell) and returns
// combined output. This preserves arguments with spaces, quotes, and other
// shell metacharacters.
func execArgsCommand(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = procenv.Child(nil)
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// substituteVarsArgs replaces $VAR placeholders in each argument string.
// Unlike substituteVars, values are NOT shell-quoted because args are passed
// directly to exec without a shell.
func substituteVarsArgs(args []string, vars map[string]string) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = arg
		for k, v := range vars {
			result[i] = strings.ReplaceAll(result[i], "$"+k, v)
		}
	}
	return result
}
