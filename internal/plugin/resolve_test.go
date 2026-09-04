package plugin

import (
	"strings"
	"testing"
)

func TestResolveBinary_UnknownPlugin(t *testing.T) {
	_, err := ResolveBinary("nonexistent-plugin-xyz")
	if err == nil {
		t.Fatal("expected error for unknown plugin")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolveBinary_RegistryPluginNotInstalled(t *testing.T) {
	_, err := ResolveBinary("notify")
	if err == nil {
		// If notify happens to be installed, this test is inconclusive.
		t.Skip("notify plugin is installed on this system")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected 'not installed' in error for registry plugin, got: %v", err)
	}
}
