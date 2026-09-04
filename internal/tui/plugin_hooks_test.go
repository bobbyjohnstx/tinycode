package tui

import "testing"

func TestPluginHooks_NilClientNoOp(t *testing.T) {
	hooks := NewPluginHooks(nil)

	// All methods should be safe with nil client.
	hooks.OnSessionStart("sess-1")
	hooks.OnSessionEnd("sess-1")
	hooks.OnSessionSwitch("sess-2")
	hooks.OnModelChange("sess-1", "model-a")
}

func TestPluginLoadedMsg_String(t *testing.T) {
	msg := PluginLoadedMsg{Name: "test-plugin"}
	s := msg.String()
	if s != "plugin loaded: test-plugin" {
		t.Errorf("expected 'plugin loaded: test-plugin', got %q", s)
	}
}

func TestPluginEventMsg_Fields(t *testing.T) {
	msg := PluginEventMsg{Name: "session.start", Data: map[string]string{"id": "1"}}
	if msg.Name != "session.start" {
		t.Errorf("expected name 'session.start', got %q", msg.Name)
	}
	data, ok := msg.Data.(map[string]string)
	if !ok {
		t.Fatal("expected data to be map[string]string")
	}
	if data["id"] != "1" {
		t.Errorf("expected id '1', got %q", data["id"])
	}
}
