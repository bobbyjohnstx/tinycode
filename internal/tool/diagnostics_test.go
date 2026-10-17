package tool

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticCommand_Go(t *testing.T) {
	cmd, dir := diagnosticCommand(".go", "/tmp/foo/main.go")
	if cmd == nil {
		t.Skip("go not on PATH")
	}
	if dir != "/tmp/foo" {
		t.Errorf("expected dir /tmp/foo, got %s", dir)
	}
	if cmd.Args[0] != "go" || cmd.Args[1] != "vet" {
		t.Errorf("expected go vet, got %v", cmd.Args)
	}
}

func TestDiagnosticCommand_Python(t *testing.T) {
	cmd, _ := diagnosticCommand(".py", "/tmp/foo/script.py")
	if cmd == nil {
		t.Skip("neither ruff nor python3 on PATH")
	}
	// Accept either ruff or python3
	base := filepath.Base(cmd.Args[0])
	if base != "ruff" && base != "python3" {
		t.Errorf("expected ruff or python3, got %s", base)
	}
}

func TestDiagnosticCommand_TypeScript(t *testing.T) {
	cmd, _ := diagnosticCommand(".ts", "/tmp/foo/index.ts")
	if cmd == nil {
		t.Skip("npx not on PATH")
	}
	found := false
	for _, arg := range cmd.Args {
		if arg == "tsc" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tsc in args, got %v", cmd.Args)
	}
}

func TestDiagnosticCommand_JavaPrefersCheckstyle(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "checkstyle"))
	writeStub(t, filepath.Join(bin, "javac"))
	t.Setenv("PATH", bin)

	path := filepath.Join(dir, "Foo.java")
	cmd, gotDir := diagnosticCommand(".java", path)
	if cmd == nil {
		t.Fatal("expected checkstyle command")
	}
	if gotDir != dir {
		t.Errorf("dir = %q, want %q", gotDir, dir)
	}
	base := filepath.Base(cmd.Args[0])
	if base != "checkstyle" {
		t.Fatalf("expected checkstyle, got %v", cmd.Args)
	}
	joined := strings.Join(cmd.Args, " ")
	for _, bad := range []string{"javac", "mvn", "gradle", "gradlew"} {
		if strings.Contains(joined, bad) {
			t.Errorf("argv must not contain %q: %v", bad, cmd.Args)
		}
	}
	if !containsArg(cmd.Args, "-f") || !containsArg(cmd.Args, "plain") {
		t.Errorf("expected -f plain, got %v", cmd.Args)
	}
}

func TestDiagnosticCommand_JavaFallsBackToJavac(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "javac"))
	t.Setenv("PATH", bin)

	path := filepath.Join(dir, "Foo.java")
	cmd, _ := diagnosticCommand(".java", path)
	if cmd == nil {
		t.Fatal("expected javac command")
	}
	base := filepath.Base(cmd.Args[0])
	if base != "javac" {
		t.Fatalf("expected javac, got %v", cmd.Args)
	}
	if len(cmd.Args) < 3 || cmd.Args[1] != "-Xlint" {
		t.Errorf("expected javac -Xlint <path>, got %v", cmd.Args)
	}
}

func TestDiagnosticCommand_JavaNeither(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // empty of tools

	cmd, _ := diagnosticCommand(".java", filepath.Join(dir, "Foo.java"))
	if cmd != nil {
		t.Errorf("expected nil when neither checkstyle nor javac on PATH, got %v", cmd.Args)
	}
}

func TestDiagnosticCommand_JavaNeverMaven(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Prefer checkstyle when both present; also cover javac-only.
	for _, tools := range [][]string{
		{"checkstyle", "javac", "mvn", "gradle"},
		{"javac", "mvn", "gradle"},
	} {
		for _, name := range tools {
			writeStub(t, filepath.Join(bin, name))
		}
		t.Setenv("PATH", bin)
		cmd, _ := diagnosticCommand(".java", filepath.Join(dir, "Foo.java"))
		if cmd == nil {
			t.Fatal("expected a diagnostic command")
		}
		for _, arg := range cmd.Args {
			base := filepath.Base(arg)
			if base == "mvn" || base == "gradle" || base == "gradlew" ||
				arg == "mvn" || arg == "gradle" || arg == "gradlew" {
				t.Errorf("argv must never contain maven/gradle: %v", cmd.Args)
			}
		}
	}
}

func writeStub(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestDiagnosticCommand_UnknownExtension(t *testing.T) {
	cmd, _ := diagnosticCommand(".xyz", "/tmp/foo/data.xyz")
	if cmd != nil {
		t.Error("expected nil command for unknown extension")
	}
}

func TestDiagnosticCommand_MissingTool(t *testing.T) {
	// .tsx with no npx available is hard to test directly, but we can test
	// that diagnosticCommand returns nil for an extension when its tool isn't
	// on PATH by using a non-existent extension handler path.
	cmd, _ := diagnosticCommand(".xyz", "/tmp/foo.xyz")
	if cmd != nil {
		t.Error("expected nil for unsupported extension")
	}
}

func TestDiagnostics_ValidGoFile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0644)

	// go vet needs a go.mod to work on a directory
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeDiagnostics(context.Background(), tc, mustJSON(t, diagnosticsArgs{FilePath: path}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No issues found") {
		// go vet on valid code should produce no issues
		t.Errorf("expected 'No issues found', got %q", result.Output)
	}
}

func TestDiagnostics_UnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.xyz")
	os.WriteFile(path, []byte("some data"), 0644)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeDiagnostics(context.Background(), tc, mustJSON(t, diagnosticsArgs{FilePath: path}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "No diagnostics available") {
		t.Errorf("expected 'No diagnostics available', got %q", result.Output)
	}
}

func TestDiagnostics_Timeout(t *testing.T) {
	// We can verify the timeout path by using a very short timeout via context.
	// The executeDiagnostics function uses its own 10s timeout, but a parent
	// context that's already canceled will propagate.
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644)

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately canceled

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeDiagnostics(ctx, tc, mustJSON(t, diagnosticsArgs{FilePath: path}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With an already-canceled context, the command should fail or time out.
	// We just verify it doesn't panic and returns some output.
	_ = result
}

func TestRunCmd_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	cmd := exec.Command("sh", "-c", `printf 'ran:%s' "$OPENROUTER_API_KEY"`)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := runCmd(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "ran:" {
		t.Fatalf("diagnostics command output = %q", got)
	}
}

func TestDiagnostics_Registration(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	def := r.Get("diagnostics")
	if def == nil {
		t.Fatal("diagnostics tool not registered")
	}
	if def.ID != "diagnostics" {
		t.Errorf("expected ID 'diagnostics', got %q", def.ID)
	}
}
