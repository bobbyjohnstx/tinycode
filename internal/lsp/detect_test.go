package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageForFile(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"/abs/path/file.go", "go"},
		{"app.ts", "typescript"},
		{"component.tsx", "typescript"},
		{"script.js", "typescript"},
		{"app.py", "python"},
		{"lib.rs", "rust"},
		{"deploy.sh", "shellscript"},
		{"init.bash", "shellscript"},
		{"setup.zsh", "shellscript"},
		{"config.yaml", "yaml"},
		{"ci.yml", "yaml"},
		{"data.json", "json"},
		{"settings.jsonc", "json"},
		{"app.dockerfile", "dockerfile"},
		{"Dockerfile", "dockerfile"},
		{"Dockerfile.dev", "dockerfile"},
		{"readme.md", ""},
		{"Makefile", ""},
	}
	for _, tt := range tests {
		got := languageForFile(tt.path)
		if got != tt.want {
			t.Errorf("languageForFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLanguageIDForFile(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.ts", "typescript"},
		{"component.tsx", "typescriptreact"},
		{"script.js", "javascript"},
		{"app.jsx", "javascriptreact"},
		{"main.py", "python"},
		{"lib.rs", "rust"},
		{"deploy.sh", "shellscript"},
		{"config.yaml", "yaml"},
		{"ci.yml", "yaml"},
		{"data.json", "json"},
		{"settings.jsonc", "jsonc"},
		{"app.dockerfile", "dockerfile"},
		{"Dockerfile", "dockerfile"},
		{"Dockerfile.prod", "dockerfile"},
		{"readme.md", ""},
	}
	for _, tt := range tests {
		got := languageIDForFile(tt.path)
		if got != tt.want {
			t.Errorf("languageIDForFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDetectServers(t *testing.T) {
	dir := t.TempDir()

	// No marker files — only always-available servers should be detected
	detected := detectServers(dir)
	for _, s := range detected {
		if s.MarkerFiles != nil {
			t.Errorf("marker-based server %q detected without marker files", s.Language)
		}
	}

	// Create go.mod — should detect Go if gopls is available
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644); err != nil {
		t.Fatal(err)
	}
	detected = detectServers(dir)
	for _, s := range detected {
		if s.MarkerFiles != nil && s.Language != "go" {
			t.Errorf("unexpected marker-based language %q with go.mod marker", s.Language)
		}
	}
}

func TestSpecForLanguage(t *testing.T) {
	// Default spec
	spec := specForLanguage("go", nil)
	if spec == nil {
		t.Fatal("expected Go spec")
	}
	if spec.Command != "gopls" {
		t.Errorf("expected gopls, got %s", spec.Command)
	}

	// User override
	overrides := map[string]ServerConfig{
		"go": {Command: "custom-gopls", Args: []string{"--custom"}},
	}
	spec = specForLanguage("go", overrides)
	if spec == nil {
		t.Fatal("expected Go spec with override")
	}
	if spec.Command != "custom-gopls" {
		t.Errorf("expected custom-gopls, got %s", spec.Command)
	}
	if len(spec.Args) != 1 || spec.Args[0] != "--custom" {
		t.Errorf("expected [--custom], got %v", spec.Args)
	}

	// Disabled
	disabled := true
	overrides = map[string]ServerConfig{
		"go": {Disabled: &disabled},
	}
	spec = specForLanguage("go", overrides)
	if spec != nil {
		t.Error("expected nil for disabled language")
	}

	// Unknown language
	spec = specForLanguage("cobol", nil)
	if spec != nil {
		t.Error("expected nil for unknown language")
	}
}

func TestExtForLanguage(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"go", ".go"},
		{"typescript", ".ts"},
		{"python", ".py"},
		{"rust", ".rs"},
		{"shellscript", ".sh"},
		{"yaml", ".yaml"},
		{"json", ".json"},
		{"dockerfile", ".dockerfile"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := extForLanguage(tt.lang)
		if got != tt.want {
			t.Errorf("extForLanguage(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}
