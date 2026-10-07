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
