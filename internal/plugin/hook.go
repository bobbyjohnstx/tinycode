package plugin

import (
	"encoding/json"
	"fmt"

	pkgplugin "github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

// sendHook sends a hook/invoke JSON-RPC call with the given hook name and input.
// Returns the unwrapped Output from the HookResult.
func (p *pluginProcess) sendHook(hookName string, input any) (json.RawMessage, error) {
	var inputJSON json.RawMessage
	if input != nil {
		var err error
		inputJSON, err = json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("marshal hook input: %w", err)
		}
	}
	raw, err := p.sendRPC("hook/invoke", pkgplugin.HookParams{
		Name:  hookName,
		Input: inputJSON,
	}, hookTimeout)
	if err != nil {
		return nil, err
	}
	var result pkgplugin.HookResult
	if raw != nil {
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("unmarshal hook result: %w", err)
		}
	}
	return result.Output, nil
}

// SessionStartEvent is emitted when a session is created.
// JSON tags match pkg/plugin.SessionStartEvent for wire compatibility.
type SessionStartEvent struct {
	SessionID string `json:"sessionId"`
	Directory string `json:"directory"`
}

// SessionEndEvent is emitted when a session is deleted.
// JSON tags match pkg/plugin.SessionEndEvent for wire compatibility.
type SessionEndEvent struct {
	SessionID string `json:"sessionId"`
}

// PermissionInput is the input for a permission hook.
// JSON tags match pkg/plugin.PermissionInput for wire compatibility.
type PermissionInput struct {
	SessionID  string `json:"sessionId"`
	ToolName   string `json:"toolName"`
	ToolArgs   string `json:"toolArgs"`
	Permission string `json:"permission"`
}

// PermissionOutput is the aggregated result of permission hooks.
type PermissionOutput struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// ShellEnvInput is the input for a shell environment hook.
// JSON tags match pkg/plugin.ShellEnvInput for wire compatibility.
type ShellEnvInput struct {
	SessionID string            `json:"sessionId"`
	Directory string            `json:"directory"`
	Env       map[string]string `json:"env,omitempty"`
}

// ShellEnvOutput is the aggregated result of shell environment hooks.
type ShellEnvOutput struct {
	Env map[string]string `json:"env"`
}

// ToolExecBeforeEvent is emitted before a tool executes.
// JSON tags match pkg/plugin.ToolExecBeforeInput for wire compatibility.
type ToolExecBeforeEvent struct {
	SessionID string `json:"sessionId"`
	ToolName  string `json:"toolName"`
	ToolArgs  string `json:"toolArgs"`
}

// ToolExecAfterEvent is emitted after a tool executes.
// JSON tags match pkg/plugin.ToolExecAfterInput for wire compatibility.
type ToolExecAfterEvent struct {
	SessionID string `json:"sessionId"`
	ToolName  string `json:"toolName"`
	Output    string `json:"output"`
	IsError   bool   `json:"isError"`
}

// permissionResult is the JSON structure returned by a permission.ask hook.
type permissionResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// shellEnvResult is the JSON structure returned by a shell.env hook.
type shellEnvResult struct {
	Env map[string]string `json:"env"`
}

// DispatchSessionStart notifies all loaded plugins of a session start,
// then fires any configured shell hooks synchronously.
// Returns collected additionalContext from both plugin and shell hooks.
func DispatchSessionStart(mgr *Manager, evt SessionStartEvent, shellRunner ...*ShellHookRunner) ([]string, error) {
	var collected []string

	if mgr != nil {
		procs := mgr.pluginsWithHook("session.start")
		for _, proc := range procs {
			raw, err := proc.sendHook("session.start", evt)
			if err != nil {
				mgr.logger.Warn("session.start hook failed", "plugin", proc.info.Name, "error", err)
				continue
			}
			if ctx := parsePluginAdditionalContext(raw); len(ctx) > 0 {
				collected = append(collected, ctx...)
			}
		}
	}

	if len(shellRunner) > 0 && shellRunner[0] != nil {
		vars := map[string]string{"SESSION_ID": evt.SessionID}
		if ctx := shellRunner[0].RunAfterSync("session.start", vars); len(ctx) > 0 {
			collected = append(collected, ctx...)
		}
	}
	return collected, nil
}

// DispatchCustomEvent invokes hook/invoke for plugins that declared hookName.
// Used by POST /plugin/event for custom event names. Failures are logged.
func DispatchCustomEvent(mgr *Manager, hookName string, input any) {
	if mgr == nil || hookName == "" {
		return
	}
	for _, proc := range mgr.pluginsWithHook(hookName) {
		if _, err := proc.sendHook(hookName, input); err != nil {
			mgr.logger.Warn("custom plugin event failed", "plugin", proc.info.Name, "event", hookName, "error", err)
		}
	}
}

// DispatchSessionEnd notifies all loaded plugins of a session end,
// then fires any configured shell hooks for the same event.
func DispatchSessionEnd(mgr *Manager, evt SessionEndEvent, shellRunner ...*ShellHookRunner) error {
	if mgr != nil {
		procs := mgr.pluginsWithHook("session.end")
		for _, proc := range procs {
			_, err := proc.sendHook("session.end", evt)
			if err != nil {
				mgr.logger.Warn("session.end hook failed", "plugin", proc.info.Name, "error", err)
			}
		}
	}

	if len(shellRunner) > 0 && shellRunner[0] != nil {
		vars := map[string]string{"SESSION_ID": evt.SessionID}
		shellRunner[0].RunAfter("session.end", vars)
	}
	return nil
}

// DispatchPermissionAsk asks all loaded plugins whether a tool invocation is
// allowed. Returns nil output when no plugins are loaded. If ANY plugin denies,
// the result is denied with that plugin's reason.
func DispatchPermissionAsk(mgr *Manager, input PermissionInput) (*PermissionOutput, error) {
	if mgr == nil {
		return nil, nil
	}
	procs := mgr.pluginsWithHook("permission.ask")
	if len(procs) == 0 {
		return nil, nil
	}

	for _, proc := range procs {
		raw, err := proc.sendHook("permission.ask", input)
		if err != nil {
			return nil, fmt.Errorf("plugin %q permission.ask: %w", proc.info.Name, err)
		}
		if len(raw) == 0 {
			continue
		}

		var result permissionResult
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("plugin %q permission.ask invalid response: %w", proc.info.Name, err)
		}

		if !result.Allowed {
			return &PermissionOutput{Allowed: false, Reason: result.Reason}, nil
		}
	}

	return &PermissionOutput{Allowed: true}, nil
}

// DispatchShellEnv asks all loaded plugins to contribute environment variables.
// Returns nil output when no plugins are loaded. Later plugins override earlier ones.
func DispatchShellEnv(mgr *Manager, input ShellEnvInput) (*ShellEnvOutput, error) {
	if mgr == nil {
		return nil, nil
	}
	procs := mgr.pluginsWithHook("shell.env")
	if len(procs) == 0 {
		return nil, nil
	}

	merged := make(map[string]string, len(input.Env))
	for k, v := range input.Env {
		merged[k] = v
	}

	for _, proc := range procs {
		raw, err := proc.sendHook("shell.env", ShellEnvInput{
			SessionID: input.SessionID,
			Directory: input.Directory,
			Env:       merged,
		})
		if err != nil {
			mgr.logger.Warn("shell.env hook failed", "plugin", proc.info.Name, "error", err)
			continue
		}

		var result shellEnvResult
		if err := json.Unmarshal(raw, &result); err != nil {
			mgr.logger.Warn("shell.env invalid response", "plugin", proc.info.Name, "error", err)
			continue
		}

		for k, v := range result.Env {
			merged[k] = v
		}
	}

	return &ShellEnvOutput{Env: merged}, nil
}

// DispatchToolExecBefore notifies plugins that a tool is about to execute,
// then runs any configured shell hooks. A plugin hook error or shell hook
// non-zero exit aborts the tool. Returns collected additionalContext from
// both plugin and shell hooks.
func DispatchToolExecBefore(mgr *Manager, evt ToolExecBeforeEvent, shellRunner ...*ShellHookRunner) ([]string, error) {
	var collected []string
	current := evt

	if mgr != nil {
		procs := mgr.pluginsWithHook("tool.execute.before")
		for _, proc := range procs {
			raw, err := proc.sendHook("tool.execute.before", current)
			if err != nil {
				return nil, fmt.Errorf("plugin %q tool.execute.before: %w", proc.info.Name, err)
			}
			extra, toolArgs, hasArgs := parseBeforeHookOutput(raw)
			if hasArgs {
				current.ToolArgs = toolArgs
			}
			if len(extra) > 0 {
				collected = append(collected, extra...)
			}
		}
	}

	if len(shellRunner) > 0 && shellRunner[0] != nil {
		vars := map[string]string{
			"TOOL":       current.ToolName,
			"ARGS":       current.ToolArgs,
			"SESSION_ID": current.SessionID,
		}
		ctx, err := shellRunner[0].RunBefore("tool.execute.before", vars)
		if err != nil {
			return nil, err
		}
		if len(ctx) > 0 {
			collected = append(collected, ctx...)
		}
	}
	return collected, nil
}

// ToolExecAfterOutput is the aggregated result of tool.execute.after hooks.
type ToolExecAfterOutput struct {
	Output            string   `json:"output"`
	IsError           bool     `json:"isError"`
	AdditionalContext []string `json:"additionalContext,omitempty"`
}

// toolExecAfterResult is the JSON structure returned by a tool.execute.after hook.
type toolExecAfterResult struct {
	Output            string   `json:"output"`
	IsError           bool     `json:"isError"`
	AdditionalContext []string `json:"additionalContext,omitempty"`
}

// DispatchToolExecAfter sends tool output through all plugins that handle
// tool.execute.after. Each plugin can transform the output; the result chains
// through so later plugins see earlier plugins' modifications.
// Shell hooks fire synchronously to capture additionalContext.
func DispatchToolExecAfter(mgr *Manager, evt ToolExecAfterEvent, shellRunner ...*ShellHookRunner) (*ToolExecAfterOutput, error) {
	current := evt
	modified := false
	var collected []string

	if mgr != nil {
		procs := mgr.pluginsWithHook("tool.execute.after")
		for _, proc := range procs {
			raw, err := proc.sendHook("tool.execute.after", current)
			if err != nil {
				mgr.logger.Warn("tool.execute.after hook failed", "plugin", proc.info.Name, "error", err)
				continue
			}
			if raw == nil {
				continue
			}
			var result toolExecAfterResult
			if err := json.Unmarshal(raw, &result); err != nil {
				mgr.logger.Warn("tool.execute.after invalid response", "plugin", proc.info.Name, "error", err)
				continue
			}
			if result.Output != "" {
				current.Output = result.Output
				current.IsError = result.IsError
				modified = true
			}
			if len(result.AdditionalContext) > 0 {
				collected = append(collected, capAdditionalContext(result.AdditionalContext)...)
			}
		}
	}

	if len(shellRunner) > 0 && shellRunner[0] != nil {
		isError := "false"
		if current.IsError {
			isError = "true"
		}
		vars := map[string]string{
			"TOOL":       evt.ToolName,
			"IS_ERROR":   isError,
			"SESSION_ID": evt.SessionID,
		}
		if ctx := shellRunner[0].RunAfterSync("tool.execute.after", vars); len(ctx) > 0 {
			collected = append(collected, ctx...)
		}
	}

	if !modified && len(collected) == 0 {
		return nil, nil
	}
	return &ToolExecAfterOutput{
		Output:            current.Output,
		IsError:           current.IsError,
		AdditionalContext: collected,
	}, nil
}

// parsePluginAdditionalContext extracts additionalContext strings from a
// plugin's JSON-RPC response. Applies the per-string character cap.
// Returns nil if raw is nil or doesn't contain the field.
func parsePluginAdditionalContext(raw json.RawMessage) []string {
	extra, _, _ := parseBeforeHookOutput(raw)
	return extra
}

// parseBeforeHookOutput reads additionalContext and an optional replacement
// for logged tool arguments. hasArgs is true only when the plugin set toolArgs.
func parseBeforeHookOutput(raw json.RawMessage) (extra []string, toolArgs string, hasArgs bool) {
	if len(raw) == 0 {
		return nil, "", false
	}
	var parsed struct {
		AdditionalContext []string `json:"additionalContext"`
		ToolArgs          *string  `json:"toolArgs"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, "", false
	}
	if parsed.ToolArgs != nil {
		return capAdditionalContext(parsed.AdditionalContext), *parsed.ToolArgs, true
	}
	return capAdditionalContext(parsed.AdditionalContext), "", false
}

// capAdditionalContext enforces the 10,000 character cap on each string.
func capAdditionalContext(ctx []string) []string {
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
