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
// Returns nil when no plugins are loaded.
func DispatchSessionStart(mgr *Manager, evt SessionStartEvent) error {
	if mgr == nil {
		return nil
	}
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	// No-op when no plugins are loaded; future implementations will
	// call each plugin's session.start hook via RPC.
	_ = evt
	return nil
}

// DispatchSessionEnd notifies all loaded plugins of a session end.
// Returns nil when no plugins are loaded.
func DispatchSessionEnd(mgr *Manager, evt SessionEndEvent) error {
	if mgr == nil {
		return nil
	}
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	_ = evt
	return nil
}

// DispatchPermissionAsk asks all loaded plugins whether a tool invocation is
// allowed. Returns nil output when no plugins are loaded.
func DispatchPermissionAsk(mgr *Manager, input PermissionInput) (*PermissionOutput, error) {
	if mgr == nil {
		return nil, nil
	}
	mgr.mu.RLock()
	count := len(mgr.plugins)
	mgr.mu.RUnlock()

	if count == 0 {
		return nil, nil
	}

	// Default: allowed when no plugin vetoes.
	return &PermissionOutput{Allowed: true}, nil
}

// DispatchShellEnv asks all loaded plugins to contribute environment variables.
// Returns nil output when no plugins are loaded.
func DispatchShellEnv(mgr *Manager, input ShellEnvInput) (*ShellEnvOutput, error) {
	if mgr == nil {
		return nil, nil
	}
	mgr.mu.RLock()
	count := len(mgr.plugins)
	mgr.mu.RUnlock()

	if count == 0 {
		return nil, nil
	}

	// Start with a copy of the input env; plugins will merge into this.
	merged := make(map[string]string, len(input.Env))
	for k, v := range input.Env {
		merged[k] = v
	}
	return &ShellEnvOutput{Env: merged}, nil
}
