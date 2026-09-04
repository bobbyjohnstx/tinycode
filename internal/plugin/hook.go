package plugin

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

// DispatchSessionStart notifies all loaded plugins of a session start.
func DispatchSessionStart(mgr *Manager, evt SessionStartEvent) error {
	if mgr == nil {
		return nil
	}
	_, err := mgr.DispatchHook("session.start", evt)
	return err
}

// DispatchSessionEnd notifies all loaded plugins of a session end.
func DispatchSessionEnd(mgr *Manager, evt SessionEndEvent) error {
	if mgr == nil {
		return nil
	}
	_, err := mgr.DispatchHook("session.end", evt)
	return err
}

// DispatchPermissionAsk asks all loaded plugins whether a tool invocation is
// allowed. Returns nil output when no plugins are loaded.
func DispatchPermissionAsk(mgr *Manager, input PermissionInput) (*PermissionOutput, error) {
	if mgr == nil {
		return nil, nil
	}

	results, err := mgr.DispatchHook("permission.ask", input)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	return &PermissionOutput{Allowed: true}, nil
}

// DispatchShellEnv asks all loaded plugins to contribute environment variables.
// Returns nil output when no plugins are loaded.
func DispatchShellEnv(mgr *Manager, input ShellEnvInput) (*ShellEnvOutput, error) {
	if mgr == nil {
		return nil, nil
	}

	results, err := mgr.DispatchHook("shell.env", input)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	merged := make(map[string]string, len(input.Env))
	for k, v := range input.Env {
		merged[k] = v
	}
	return &ShellEnvOutput{Env: merged}, nil
}
