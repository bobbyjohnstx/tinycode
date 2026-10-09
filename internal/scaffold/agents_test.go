package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAgentsMD_GoModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := GenerateAgentsMD(dir)
	for _, want := range []string{"Go", "go build", "go test", "internal/", "build", "plan", "architect"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in AGENTS.md, got:\n%s", want, got)
		}
	}
}

func TestGenerateAgentsMD_PackageJSONScripts(t *testing.T) {
	dir := t.TempDir()
	pkg := `{
  "name": "demo",
  "scripts": {
    "build": "vite build",
    "test": "vitest",
    "lint": "eslint ."
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}

	got := GenerateAgentsMD(dir)
	if !strings.Contains(got, "npm run build") || !strings.Contains(got, "npm run test") {
		t.Fatalf("expected npm scripts, got:\n%s", got)
	}
}

func TestEnsureRootAgentsMD_CreatesOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created, path, err := EnsureRootAgentsMD(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected create on first call")
	}
	if filepath.Base(path) != "AGENTS.md" {
		t.Fatalf("path = %q", path)
	}

	created, _, err = EnsureRootAgentsMD(dir)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected skip when AGENTS.md exists")
	}
}

func TestMakefileTargets_ParsesTargets(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected map[string]bool
	}{
		{
			name:     "simple targets",
			content:  "build:\n\tgo build\ntest:\n\tgo test\n",
			expected: map[string]bool{"build": true, "test": true},
		},
		{
			name:     "skips comments and dot-prefixed",
			content:  "# comment\n.PHONY: build\nbuild:\n\tgo build\n",
			expected: map[string]bool{"build": true},
		},
		{
			name:     "skips lines with spaces or tabs in name",
			content:  "good-target:\n\techo ok\nbad target:\n\techo skip\n",
			expected: map[string]bool{"good-target": true},
		},
		{
			name:     "skips variable assignments",
			content:  "VAR=value\nbuild:\n\techo ok\n",
			expected: map[string]bool{"build": true},
		},
		{
			name:     "empty Makefile",
			content:  "",
			expected: map[string]bool{},
		},
		{
			name:     "target with dependencies",
			content:  "all: build test\nbuild:\n\tgo build\ntest:\n\tgo test\n",
			expected: map[string]bool{"all": true, "build": true, "test": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "Makefile")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := makefileTargets(path)
			for k := range tt.expected {
				if !got[k] {
					t.Errorf("expected target %q not found", k)
				}
			}
			for k := range got {
				if !tt.expected[k] {
					t.Errorf("unexpected target %q found", k)
				}
			}
		})
	}
}

func TestMakefileTargets_NonexistentFile(t *testing.T) {
	got := makefileTargets("/nonexistent/Makefile")
	if got != nil {
		t.Errorf("expected nil for nonexistent file, got %v", got)
	}
}

func TestFirstTarget_ReturnsFirstMatch(t *testing.T) {
	targets := map[string]bool{"test": true, "lint": true}

	got := firstTarget(targets, "build", "test")
	if got != "test" {
		t.Errorf("expected test, got %q", got)
	}
}

func TestFirstTarget_ReturnsEmptyWhenNoMatch(t *testing.T) {
	targets := map[string]bool{"other": true}

	got := firstTarget(targets, "build", "test")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFirstTarget_NilMap(t *testing.T) {
	got := firstTarget(nil, "build", "test")
	if got != "" {
		t.Errorf("expected empty string for nil map, got %q", got)
	}
}

func TestDetectCommands_RustProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o644)

	build, test, lint := detectCommands(dir, "Rust")
	if build != "cargo build" {
		t.Errorf("expected cargo build, got %q", build)
	}
	if test != "cargo test" {
		t.Errorf("expected cargo test, got %q", test)
	}
	if lint != "cargo clippy" {
		t.Errorf("expected cargo clippy, got %q", lint)
	}
}

func TestDetectCommands_PythonProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n"), 0o644)

	build, test, lint := detectCommands(dir, "Python")
	if build != "" {
		t.Errorf("expected empty build for Python, got %q", build)
	}
	if test != "pytest" {
		t.Errorf("expected pytest, got %q", test)
	}
	if lint != "ruff check ." {
		t.Errorf("expected ruff check ., got %q", lint)
	}
}

func TestDetectCommands_MakefileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\tgo build ./cmd/...\ntest:\n\tgo test -race ./...\nlint:\n\tgolangci-lint run\n"), 0o644)

	build, test, lint := detectCommands(dir, "Go")
	if build != "make build" {
		t.Errorf("expected make build, got %q", build)
	}
	if test != "make test" {
		t.Errorf("expected make test, got %q", test)
	}
	if lint != "make lint" {
		t.Errorf("expected make lint, got %q", lint)
	}
}

func TestDetectCommands_MakefilePartialTargets(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)
	// Makefile only has build, test and lint should fall through to Go defaults.
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\tgo build\n"), 0o644)

	build, test, lint := detectCommands(dir, "Go")
	if build != "make build" {
		t.Errorf("expected make build, got %q", build)
	}
	if test != "go test ./..." {
		t.Errorf("expected go test ./... fallback, got %q", test)
	}
	if lint != "go vet ./..." {
		t.Errorf("expected go vet ./... fallback, got %q", lint)
	}
}

func TestDetectCommands_NoIndicators(t *testing.T) {
	dir := t.TempDir()
	build, test, lint := detectCommands(dir, "")
	if build != "" || test != "" || lint != "" {
		t.Errorf("expected empty commands for unknown lang, got build=%q test=%q lint=%q", build, test, lint)
	}
}

func TestOrDash_EmptyReturnsPlaceholder(t *testing.T) {
	got := orDash("")
	if !strings.Contains(got, "not detected") {
		t.Errorf("expected placeholder with 'not detected', got %q", got)
	}
}

func TestOrDash_NonEmptyReturnsInput(t *testing.T) {
	got := orDash("go build")
	if got != "go build" {
		t.Errorf("expected 'go build', got %q", got)
	}
}

func TestNpmScript_ReturnsCommandWhenFound(t *testing.T) {
	scripts := map[string]bool{"build": true, "test": true}
	got := npmScript(scripts, "build")
	if got != "npm run build" {
		t.Errorf("expected 'npm run build', got %q", got)
	}
}

func TestNpmScript_ReturnsEmptyWhenNotFound(t *testing.T) {
	scripts := map[string]bool{"build": true}
	got := npmScript(scripts, "lint")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGenerateAgentsMD_UnknownEcosystem(t *testing.T) {
	dir := t.TempDir()
	got := GenerateAgentsMD(dir)
	if !strings.Contains(got, "unknown") {
		t.Errorf("expected 'unknown' for empty dir, got:\n%s", got)
	}
	if !strings.Contains(got, "not detected") {
		t.Errorf("expected 'not detected' placeholder for commands, got:\n%s", got)
	}
}

func TestGenerateAgentsMD_RustProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o644)
	os.Mkdir(filepath.Join(dir, "src"), 0o755)

	got := GenerateAgentsMD(dir)
	if !strings.Contains(got, "Rust") {
		t.Errorf("expected Rust language, got:\n%s", got)
	}
	if !strings.Contains(got, "cargo build") {
		t.Errorf("expected cargo build, got:\n%s", got)
	}
	if !strings.Contains(got, "src/") {
		t.Errorf("expected src/ in key directories, got:\n%s", got)
	}
}

func TestDetectCommands_PackageJSONWithMakefile(t *testing.T) {
	dir := t.TempDir()
	// Makefile has build, package.json has test and lint.
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\techo build\n"), 0o644)
	pkg := `{
  "name": "demo",
  "scripts": {
    "test": "vitest",
    "lint": "eslint ."
  }
}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644)

	build, test, lint := detectCommands(dir, "Make")
	if build != "make build" {
		t.Errorf("expected make build from Makefile, got %q", build)
	}
	if test != "npm run test" {
		t.Errorf("expected npm run test from package.json, got %q", test)
	}
	if lint != "npm run lint" {
		t.Errorf("expected npm run lint from package.json, got %q", lint)
	}
}
