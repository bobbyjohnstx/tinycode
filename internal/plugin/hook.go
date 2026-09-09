package plugin

import (
	"encoding/json"
	"fmt"
)

// hookInvokeParams matches the wire format of pkg/plugin.HookParams.
type hookInvokeParams struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
}

// hookResult matches the wire format of pkg/plugin.HookResult.
type hookResult struct {
	Output json.RawMessage `json:"output,omitempty"`
}

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
	raw, err := p.sendRPC("hook/invoke", hookInvokeParams{
		Name:  hookName,
		Input: inputJSON,
	})
	if err != nil {
		return nil, err
	}
	var result hookResult
	if raw != nil {
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("unmarshal hook result: %w", err)
		}
	}
	return result.Output, nil
}

// SessionStartEvent is emitted when a session is created.
type SessionStartEvent struct {
	SessionID string
}

// SessionEndEvent is emitted when a session is deleted.
type SessionEndEvent struct {
	SessionID string
}

// PermissionInput is the input for a permission hook.
type PermissionInput struct {
	SessionID string
	ToolName  string
	Args      map[string]any
}

// PermissionOutput is the aggregated result of permission hooks.
type PermissionOutput struct {
	Allowed bool
	Reason  string
}

// ShellEnvInput is the input for a shell environment hook.
type ShellEnvInput struct {
	SessionID string
	Env       map[string]string
}

// ShellEnvOutput is the aggregated result of shell environment hooks.
type ShellEnvOutput struct {
	Env map[string]string
}

// ToolExecBeforeEvent is emitted before a tool executes.
type ToolExecBeforeEvent struct {
	SessionID string
	Tool      string
	Args      string
}

// ToolExecAfterEvent is emitted after a tool executes.
type ToolExecAfterEvent struct {
	SessionID string
	Tool      string
	Success   bool
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
		_, err := proc.sendHook("session.start", map[string]string{
			"sessionID": evt.SessionID,
		})
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
		_, err := proc.sendHook("session.end", map[string]string{
			"sessionID": evt.SessionID,
		})
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
		raw, err := proc.sendHook("permission.ask", map[string]any{
			"sessionID": input.SessionID,
			"toolName":  input.ToolName,
			"args":      input.Args,
		})
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
		raw, err := proc.sendHook("shell.env", map[string]any{
			"sessionID": input.SessionID,
			"env":       merged,
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
		_, err := proc.sendHook("tool.execute.before", map[string]any{
			"sessionID": evt.SessionID,
			"tool":      evt.Tool,
			"args":      evt.Args,
		})
		if err != nil {
			mgr.logger.Warn("tool.execute.before hook failed", "plugin", proc.info.Name, "error", err)
		}
	}
	return nil
}

// DispatchToolExecAfter notifies plugins that a tool has finished executing.
func DispatchToolExecAfter(mgr *Manager, evt ToolExecAfterEvent) error {
	if mgr == nil {
		return nil
	}
	procs := mgr.pluginsWithHook("tool.execute.after")
	if len(procs) == 0 {
		return nil
	}

	for _, proc := range procs {
		_, err := proc.sendHook("tool.execute.after", map[string]any{
			"sessionID": evt.SessionID,
			"tool":      evt.Tool,
			"success":   evt.Success,
		})
		if err != nil {
			mgr.logger.Warn("tool.execute.after hook failed", "plugin", proc.info.Name, "error", err)
		}
	}
	return nil
}
