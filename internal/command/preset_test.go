package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresetStore_SaveAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewPresetStoreAt(filepath.Join(dir, "presets.json"))

	if err := store.Save("greet", "Hello world", "", ""); err != nil {
		t.Fatalf("save error: %v", err)
	}
	if err := store.Save("fix", "Fix all issues", "", ""); err != nil {
		t.Fatalf("save error: %v", err)
	}

	names, err := store.List()
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 presets, got %d", len(names))
	}
	if names[0] != "fix" || names[1] != "greet" {
		t.Errorf("names = %v, expected [fix, greet]", names)
	}
}

func TestPresetStore_Get(t *testing.T) {
	dir := t.TempDir()
	store := NewPresetStoreAt(filepath.Join(dir, "presets.json"))

	if err := store.Save("greet", "Hello world", "", ""); err != nil {
		t.Fatal(err)
	}
	body, ok := store.Get("greet")
	if !ok {
		t.Fatal("expected preset found")
	}
	if body != "Hello world" {
		t.Errorf("body = %q", body)
	}

	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestPresetStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewPresetStoreAt(filepath.Join(dir, "presets.json"))

	if err := store.Save("greet", "Hello world", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("greet"); err != nil {
		t.Fatalf("delete error: %v", err)
	}
	_, ok := store.Get("greet")
	if ok {
		t.Error("expected deleted")
	}
}

func TestPresetStore_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewPresetStoreAt(filepath.Join(dir, "presets.json"))

	err := store.Delete("nope")
	if err == nil {
		t.Fatal("expected error for missing preset")
	}
}

func TestPresetStore_RejectBuiltinName(t *testing.T) {
	dir := t.TempDir()
	store := NewPresetStoreAt(filepath.Join(dir, "presets.json"))

	builtins := []string{"plan", "review", "exit", "help", "compact", "preset"}
	for _, name := range builtins {
		err := store.Save(name, "body", "", "")
		if err == nil {
			t.Errorf("expected rejection for builtin name %q", name)
		}
		if !strings.Contains(err.Error(), "conflicts") {
			t.Errorf("error for %q = %v, expected conflict message", name, err)
		}
	}
}

func TestExpandSlashCommand_PresetExpands(t *testing.T) {
	configDir := t.TempDir()
	presetPath := filepath.Join(configDir, "presets.json")
	if err := os.WriteFile(presetPath, []byte(`{"greet":"Hello world"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ExpandSlashCommand("/greet", configDir, "")
	if result.Text != "Hello world" {
		t.Errorf("expected preset body, got %q", result.Text)
	}
	if result.DisplayText != "/greet" {
		t.Errorf("DisplayText = %q", result.DisplayText)
	}
}

func TestExpandSlashCommand_PresetWithSuffix(t *testing.T) {
	configDir := t.TempDir()
	presetPath := filepath.Join(configDir, "presets.json")
	if err := os.WriteFile(presetPath, []byte(`{"greet":"Hello world"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ExpandSlashCommand("/greet extra text", configDir, "")
	if result.Text != "Hello world\nextra text" {
		t.Errorf("expected preset body with suffix, got %q", result.Text)
	}
}

func TestPresetStore_EmptyNameRejected(t *testing.T) {
	dir := t.TempDir()
	store := NewPresetStoreAt(filepath.Join(dir, "presets.json"))

	err := store.Save("", "body", "", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}
