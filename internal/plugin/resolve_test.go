package plugin

import (
	"os"
	"path/filepath"
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
	_, err := ResolveBinary("log-sanitizer")
	if err == nil {
		// If log-sanitizer happens to be installed, this test is inconclusive.
		t.Skip("log-sanitizer plugin is installed on this system")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected 'not installed' in error for registry plugin, got: %v", err)
	}
}

func TestResolveBinary_FindsInPATH(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tinycode-plugin-test-path-resolve")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	// Prepend temp dir to PATH so LookPath finds it.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)

	result, err := ResolveBinary("test-path-resolve")
	if err != nil {
		t.Fatalf("expected ResolveBinary to find binary in PATH, got error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %q", result)
	}
	if filepath.Base(result) != "tinycode-plugin-test-path-resolve" {
		t.Errorf("expected basename tinycode-plugin-test-path-resolve, got %q", filepath.Base(result))
	}
}

func TestResolveBinary_DescriptiveErrorWhenNotFound(t *testing.T) {
	// Use a name that is not in the registry and not in PATH.
	_, err := ResolveBinary("zzz-definitely-not-a-plugin-xyz")
	if err == nil {
		t.Fatal("expected error when plugin not found anywhere")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected descriptive 'not found' error, got: %v", err)
	}
	// Should mention config dir, PATH, or registry.
	if !strings.Contains(err.Error(), "config dir") && !strings.Contains(err.Error(), "PATH") && !strings.Contains(err.Error(), "registry") {
		t.Errorf("expected error to mention search locations, got: %v", err)
	}
}

func TestIsExecutable_ExecutableFile(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "exec-test")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !isExecutable(binPath) {
		t.Error("expected executable file to return true")
	}
}

func TestIsExecutable_NonExecutableFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "noexec-test")
	if err := os.WriteFile(filePath, []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if isExecutable(filePath) {
		t.Error("expected non-executable file to return false")
	}
}

func TestIsExecutable_NonexistentFile(t *testing.T) {
	if isExecutable("/nonexistent/path/to/binary") {
		t.Error("expected nonexistent file to return false")
	}
}
