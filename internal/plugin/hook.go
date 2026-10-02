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
	})
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

// DispatchSessionStart notifies all loaded plugins of a session start.
func DispatchSessionStart(mgr *Manager, evt SessionStartEvent) error {
	if mgr == nil {
		return nil
	}
	procs := mgr.pluginsWithHook("session.start")
	if len(procs) == 0 {
		return nil
	}

	for _, proc := range procs {
		_, err := proc.sendHook("session.start", evt)
		if err != nil {
			mgr.logger.Warn("session.start hook failed", "plugin", proc.info.Name, "error", err)
		}
	}
	return nil
}

// DispatchSessionEnd notifies all loaded plugins of a session end.
func DispatchSessionEnd(mgr *Manager, evt SessionEndEvent) error {
	if mgr == nil {
		return nil
	}
	procs := mgr.pluginsWithHook("session.end")
	if len(procs) == 0 {
		return nil
	}

	for _, proc := range procs {
		_, err := proc.sendHook("session.end", evt)
		if err != nil {
			mgr.logger.Warn("session.end hook failed", "plugin", proc.info.Name, "error", err)
		}
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
			mgr.logger.Warn("permission.ask hook failed", "plugin", proc.info.Name, "error", err)
			continue
		}

		var result permissionResult
		if err := json.Unmarshal(raw, &result); err != nil {
			mgr.logger.Warn("permission.ask invalid response", "plugin", proc.info.Name, "error", err)
			continue
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

// DispatchToolExecBefore notifies plugins that a tool is about to execute.
func DispatchToolExecBefore(mgr *Manager, evt ToolExecBeforeEvent) error {
	if mgr == nil {
		return nil
	}
	procs := mgr.pluginsWithHook("tool.execute.before")
	if len(procs) == 0 {
		return nil
	}

	for _, proc := range procs {
		_, err := proc.sendHook("tool.execute.before", evt)
		if err != nil {
			mgr.logger.Warn("tool.execute.before hook failed", "plugin", proc.info.Name, "error", err)
		}
	}
	return nil
}

// ToolExecAfterOutput is the aggregated result of tool.execute.after hooks.
type ToolExecAfterOutput struct {
	Output  string `json:"output"`
	IsError bool   `json:"isError"`
}

// toolExecAfterResult is the JSON structure returned by a tool.execute.after hook.
type toolExecAfterResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"isError"`
}

// DispatchToolExecAfter sends tool output through all plugins that handle
// tool.execute.after. Each plugin can transform the output; the result chains
// through so later plugins see earlier plugins' modifications.
func DispatchToolExecAfter(mgr *Manager, evt ToolExecAfterEvent) (*ToolExecAfterOutput, error) {
	if mgr == nil {
		return nil, nil
	}
	procs := mgr.pluginsWithHook("tool.execute.after")
	if len(procs) == 0 {
		return nil, nil
	}

	current := evt
	modified := false
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
	}

	if !modified {
		return nil, nil
	}
	return &ToolExecAfterOutput{Output: current.Output, IsError: current.IsError}, nil
}
