package plugin

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/id"
)

var ErrPluginNotFound = errors.New("plugin not found")

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Manager manages loaded plugins and the curated plugin registry.
type Manager struct {
	mu       sync.RWMutex
	plugins  map[string]*PluginInfo
	registry []RegistryEntry
	logger   *slog.Logger
}

// NewManager creates an empty plugin manager using the built-in registry.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		plugins:  make(map[string]*PluginInfo),
		registry: Registry(),
		logger:   logger,
	}
}

// NewManagerWithRegistry creates a plugin manager with a custom registry.
// This is intended for tests that need to control the registry entries.
func NewManagerWithRegistry(entries []RegistryEntry) *Manager {
	if entries == nil {
		entries = []RegistryEntry{}
	}
	return &Manager{
		plugins:  make(map[string]*PluginInfo),
		registry: entries,
		logger:   slog.Default(),
	}
}

// List returns all loaded plugins.
func (m *Manager) List() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]PluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, *p)
	}
	return out
}

// Load registers a plugin by name and returns its info.
// Returns ErrPluginNotFound if the name is not in the registry.
func (m *Manager) Load(name string) (*PluginInfo, error) {
	if name == "" {
		return nil, errors.New("plugin name is required")
	}

	m.mu.RLock()
	found := false
	for _, entry := range m.registry {
		if entry.Name == name {
			found = true
			break
		}
	}
	m.mu.RUnlock()

	if !found {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}

	pid, err := id.Ascending("plugin")
	if err != nil {
		return nil, err
	}

	info := &PluginInfo{
		ID:   pid,
		Name: name,
	}

	m.mu.Lock()
	m.plugins[pid] = info
	m.mu.Unlock()

	m.logger.Info("plugin loaded", "id", pid, "name", name)
	return info, nil
}

// Unload removes a plugin by ID.
func (m *Manager) Unload(pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.plugins[pluginID]; !ok {
		return errors.New("plugin not found")
	}
	delete(m.plugins, pluginID)
	m.logger.Info("plugin unloaded", "id", pluginID)
	return nil
}

// Registry returns available registry entries.
func (m *Manager) Registry() []RegistryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.registry == nil {
		return []RegistryEntry{}
	}
	out := make([]RegistryEntry, len(m.registry))
	copy(out, m.registry)
	return out
}

// Plugins returns info about all loaded plugins.
// Deprecated: Use List instead.
func (m *Manager) Plugins() []PluginInfo {
	return m.List()
}

// UnloadPlugin removes a plugin by ID.
// Deprecated: Use Unload instead.
func (m *Manager) UnloadPlugin(id string) error {
	return m.Unload(id)
}

// Shutdown unloads all plugins.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	count := len(m.plugins)
	m.plugins = make(map[string]*PluginInfo)
	m.mu.Unlock()

	m.logger.Info("plugin manager shut down", "count", count)
}
