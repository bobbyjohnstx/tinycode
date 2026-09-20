package lsp

import (
	"path/filepath"
	"testing"
)

func TestNewManager_NilConfig(t *testing.T) {
	// nil config should use defaults without panicking.
	m := NewManager(t.TempDir(), nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	defer m.Close()

	if m.clients == nil {
		t.Error("clients map should be initialized")
	}
	if m.unavail == nil {
		t.Error("unavail map should be initialized")
	}
	if m.overrides == nil {
		t.Error("overrides map should be initialized")
	}
}

func TestNewManager_Disabled(t *testing.T) {
	enabled := false
	cfg := &Config{
		Enabled: &enabled,
	}

	m := NewManager(t.TempDir(), cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	defer m.Close()

	// When disabled, the wildcard "*" should be in unavail.
	if !m.unavail["*"] {
		t.Error("expected wildcard unavail when disabled")
	}

	// All languages should be unavailable.
	langs := m.AvailableLanguages()
	if len(langs) != 0 {
		t.Errorf("AvailableLanguages() = %v, want empty", langs)
	}
}

func TestNewManager_Enabled(t *testing.T) {
	enabled := true
	cfg := &Config{
		Enabled: &enabled,
	}

	m := NewManager(t.TempDir(), cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	defer m.Close()

	if m.unavail["*"] {
		t.Error("wildcard should not be unavail when enabled")
	}
}

func TestNewManager_CustomTimeout(t *testing.T) {
	timeout := 120
	cfg := &Config{
		Timeout: &timeout,
	}

	m := NewManager(t.TempDir(), cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	defer m.Close()

	// 120 seconds.
	if m.timeout.Seconds() != 120 {
		t.Errorf("timeout = %v, want 120s", m.timeout)
	}
}

func TestResolvePath_Relative(t *testing.T) {
	dir := "/project/root"
	got := resolvePath("src/main.go", dir)
	want := filepath.Join(dir, "src/main.go")
	if got != want {
		t.Errorf("resolvePath(%q, %q) = %q, want %q", "src/main.go", dir, got, want)
	}
}

func TestResolvePath_Absolute(t *testing.T) {
	dir := "/project/root"
	abs := "/other/path/file.go"
	got := resolvePath(abs, dir)
	if got != abs {
		t.Errorf("resolvePath(%q, %q) = %q, want %q", abs, dir, got, abs)
	}
}

func TestResolvePath_DotRelative(t *testing.T) {
	dir := "/project/root"
	got := resolvePath("./file.go", dir)
	want := filepath.Join(dir, "./file.go")
	if got != want {
		t.Errorf("resolvePath(%q, %q) = %q, want %q", "./file.go", dir, got, want)
	}
}

func TestNoServerMessage_WithLanguage(t *testing.T) {
	msg := noServerMessage("go", "/project/main.go")
	if msg == "" {
		t.Fatal("message is empty")
	}
	// Should include the language name.
	if !containsStr(msg, "go") {
		t.Errorf("message %q should mention language 'go'", msg)
	}
	// Go has an install hint — should include it.
	if !containsStr(msg, "gopls") {
		t.Errorf("message %q should include install hint for go", msg)
	}
}

func TestNoServerMessage_WithoutLanguage(t *testing.T) {
	msg := noServerMessage("", "/project/file.xyz")
	if msg == "" {
		t.Fatal("message is empty")
	}
	// Should fall back to file extension message.
	if !containsStr(msg, ".xyz") {
		t.Errorf("message %q should mention file extension", msg)
	}
}

func TestNoServerMessage_UnknownLanguage(t *testing.T) {
	msg := noServerMessage("cobol", "/project/main.cob")
	if msg == "" {
		t.Fatal("message is empty")
	}
	// Should mention the language without an install hint.
	if !containsStr(msg, "cobol") {
		t.Errorf("message %q should mention 'cobol'", msg)
	}
}

func TestNewManager_WithServerOverrides(t *testing.T) {
	cfg := &Config{
		Servers: map[string]ServerConfig{
			"go": {
				Command: "custom-gopls",
				Args:    []string{"--debug"},
			},
		},
	}

	m := NewManager(t.TempDir(), cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	defer m.Close()

	if len(m.overrides) != 1 {
		t.Errorf("overrides count = %d, want 1", len(m.overrides))
	}
	if m.overrides["go"].Command != "custom-gopls" {
		t.Errorf("override command = %q, want custom-gopls", m.overrides["go"].Command)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && findStr(s, sub)
}

func findStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
