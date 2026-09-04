package plugin

import (
	"log/slog"
	"testing"
)

func TestNewManager_Empty(t *testing.T) {
	m := NewManager(slog.Default())
	if m.plugins == nil {
		t.Fatal("expected non-nil plugins map")
	}
	if len(m.plugins) != 0 {
		t.Errorf("expected empty plugins map, got %d entries", len(m.plugins))
	}
}

func TestPlugins_EmptyInitially(t *testing.T) {
	m := NewManager(slog.Default())
	infos := m.Plugins()
	if len(infos) != 0 {
		t.Errorf("expected empty plugin list, got %d entries", len(infos))
	}
}

func TestUnloadPlugin_UnknownID(t *testing.T) {
	m := NewManager(slog.Default())
	err := m.UnloadPlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin id")
	}
}

func TestShutdown_EmptyManager(t *testing.T) {
	m := NewManager(slog.Default())
	// Should not panic or error on empty manager.
	m.Shutdown()
}
