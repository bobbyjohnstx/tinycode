package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func findGopls() string {
	if path, err := exec.LookPath("gopls"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	candidate := filepath.Join(home, "go", "bin", "gopls")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func TestClientHoverIntegration(t *testing.T) {
	goplsPath := findGopls()
	if goplsPath == "" {
		t.Skip("gopls not available")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := ServerSpec{
		Language:   "go",
		Command:    goplsPath,
		Args:       []string{"serve"},
		ExtLangIDs: map[string]string{".go": "go"},
	}

	client := newClient(spec, dir, nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer client.close()

	if err := client.connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Hover over fmt.Println (line 5, col 5 = 0-indexed line 5, char 5)
	mainFile := filepath.Join(dir, "main.go")
	content, err := client.Hover(ctx, mainFile, 5, 5)
	if err != nil {
		t.Fatalf("hover failed: %v", err)
	}
	if content == "" {
		t.Error("expected hover content for fmt.Println, got empty")
	}
	t.Logf("hover result: %s", content)
}

func TestClientDefinitionIntegration(t *testing.T) {
	goplsPath := findGopls()
	if goplsPath == "" {
		t.Skip("gopls not available")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func greet() string {
	return "hello"
}

func main() {
	_ = greet()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := ServerSpec{
		Language:   "go",
		Command:    goplsPath,
		Args:       []string{"serve"},
		ExtLangIDs: map[string]string{".go": "go"},
	}

	client := newClient(spec, dir, nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer client.close()

	if err := client.connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Go to definition of greet() call (line 7, col 5 = 0-indexed)
	mainFile := filepath.Join(dir, "main.go")
	locs, err := client.Definition(ctx, mainFile, 7, 5)
	if err != nil {
		t.Fatalf("definition failed: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("expected at least one definition location")
	}

	// Definition should be in same file, line 2 (0-indexed)
	defLine := locs[0].Range.Start.Line
	if defLine != 2 {
		t.Errorf("expected definition at line 2, got %d", defLine)
	}
	t.Logf("definition at: %s:%d:%d", fileFromURI(locs[0].URI), defLine+1, locs[0].Range.Start.Character+1)
}

func TestClientDiagnosticsIntegration(t *testing.T) {
	goplsPath := findGopls()
	if goplsPath == "" {
		t.Skip("gopls not available")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// File with a deliberate error
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {
	x := undefinedFunc()
	_ = x
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := ServerSpec{
		Language:   "go",
		Command:    goplsPath,
		Args:       []string{"serve"},
		ExtLangIDs: map[string]string{".go": "go"},
	}

	client := newClient(spec, dir, nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer client.close()

	if err := client.connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	mainFile := filepath.Join(dir, "main.go")
	diags, err := client.Diagnostics(ctx, mainFile)
	if err != nil {
		t.Fatalf("diagnostics failed: %v", err)
	}
	if len(diags) == 0 {
		t.Error("expected diagnostics for file with error, got none")
	}
	for _, d := range diags {
		t.Logf("diagnostic: %s:%d:%d %s: %s",
			mainFile, d.Range.Start.Line+1, d.Range.Start.Character+1,
			severityStr(d.Severity), d.Message)
	}
}
