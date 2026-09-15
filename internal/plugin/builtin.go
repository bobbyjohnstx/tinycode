package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// BuiltinPlugin is the interface for in-process plugins that provide tools
// and lifecycle hooks without requiring an external process.
type BuiltinPlugin interface {
	ID() string
	Tools() []BuiltinTool
	Hooks() BuiltinHooks
}

// BuiltinTool defines a tool provided by a built-in plugin.
type BuiltinTool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args json.RawMessage) (string, error)
}

// BuiltinHooks holds optional callback functions for built-in plugin lifecycle hooks.
// Each field is nil when the plugin does not handle that hook.
type BuiltinHooks struct {
	SessionStart  func(ctx context.Context, sessionID string) error
	SessionEnd    func(ctx context.Context, sessionID string) error
	Dispose       func(ctx context.Context) error
	ToolExecAfter func(ctx context.Context, toolName, output string, isError bool) (modifiedOutput string, modifiedIsError bool, modified bool)
}

// BuiltinManager manages registered built-in plugins and dispatches
// tool calls and hook events to them.
type BuiltinManager struct {
	mu      sync.RWMutex
	plugins []BuiltinPlugin
	tools   map[string]BuiltinTool
}

// NewBuiltinManager creates a new BuiltinManager.
func NewBuiltinManager() *BuiltinManager {
	return &BuiltinManager{
		tools: make(map[string]BuiltinTool),
	}
}

// Register adds a built-in plugin and indexes its tools.
func (m *BuiltinManager) Register(p BuiltinPlugin) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.plugins = append(m.plugins, p)
	for _, t := range p.Tools() {
		m.tools[t.Name] = t
	}
}

// CallTool dispatches a tool call by name to the appropriate plugin tool.
func (m *BuiltinManager) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	m.mu.RLock()
	t, ok := m.tools[name]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown built-in tool: %s", name)
	}
	return t.Execute(ctx, args)
}

// DispatchHook calls the named hook on all registered plugins that handle it.
// Supported hook names: "session.start", "session.end", "dispose".
func (m *BuiltinManager) DispatchHook(name string, input any) error {
	m.mu.RLock()
	plugins := make([]BuiltinPlugin, len(m.plugins))
	copy(plugins, m.plugins)
	m.mu.RUnlock()

	for _, p := range plugins {
		hooks := p.Hooks()
		switch name {
		case "session.start":
			if hooks.SessionStart == nil {
				break
			}
			sessionID, _ := input.(string)
			if err := hooks.SessionStart(context.Background(), sessionID); err != nil {
				return fmt.Errorf("plugin %s hook %s: %w", p.ID(), name, err)
			}
		case "session.end":
			if hooks.SessionEnd == nil {
				break
			}
			sessionID, _ := input.(string)
			if err := hooks.SessionEnd(context.Background(), sessionID); err != nil {
				return fmt.Errorf("plugin %s hook %s: %w", p.ID(), name, err)
			}
		case "dispose":
			if hooks.Dispose == nil {
				break
			}
			if err := hooks.Dispose(context.Background()); err != nil {
				return fmt.Errorf("plugin %s hook %s: %w", p.ID(), name, err)
			}
		}
	}
	return nil
}

// DispatchToolExecAfter sends tool output through all builtin plugins that handle it.
func (m *BuiltinManager) DispatchToolExecAfter(ctx context.Context, toolName, output string, isError bool) (string, bool, bool) {
	m.mu.RLock()
	plugins := make([]BuiltinPlugin, len(m.plugins))
	copy(plugins, m.plugins)
	m.mu.RUnlock()

	modified := false
	for _, p := range plugins {
		hooks := p.Hooks()
		if hooks.ToolExecAfter == nil {
			continue
		}
		modOut, modErr, changed := hooks.ToolExecAfter(ctx, toolName, output, isError)
		if changed {
			output = modOut
			isError = modErr
			modified = true
		}
	}
	return output, isError, modified
}

// AllTools returns all registered builtin tools.
func (m *BuiltinManager) AllTools() []BuiltinTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var all []BuiltinTool
	for _, p := range m.plugins {
		all = append(all, p.Tools()...)
	}
	return all
}
