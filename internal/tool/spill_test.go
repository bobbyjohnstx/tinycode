package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpillIfTruncated_NotTruncated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	tr := &TruncateResult{Content: "short output", Truncated: false, FullSize: 12}
	got := SpillIfTruncated("sess1", "short output", tr)
	if got != "short output" {
		t.Errorf("expected unchanged content, got: %s", got)
	}

	// No file should be created.
	entries, _ := os.ReadDir(filepath.Join(dir, spillSubdir))
	if len(entries) > 0 {
		t.Errorf("expected no spill files, found %d", len(entries))
	}
}

func TestSpillIfTruncated_Truncated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	fullOutput := strings.Repeat("x\n", 3000) // well over limits
	preview := "first 30 lines..."
	tr := &TruncateResult{Content: preview, Truncated: true, FullSize: len(fullOutput)}

	got := SpillIfTruncated("sess1", fullOutput, tr)

	if !strings.HasPrefix(got, preview) {
		t.Errorf("expected result to start with preview, got prefix: %s", got[:50])
	}
	if !strings.Contains(got, "[Full output saved to") {
		t.Fatalf("expected spill path hint, got: %s", got)
	}
	if !strings.Contains(got, "Use `read` to see more.]") {
		t.Fatalf("expected read hint, got: %s", got)
	}

	// Extract the path and verify the file content.
	start := strings.Index(got, "saved to ") + len("saved to ")
	end := strings.Index(got[start:], ".")
	path := got[start : start+end+4] // include ".txt"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read spill file: %v", err)
	}
	if string(data) != fullOutput {
		t.Errorf("spill file content mismatch: got %d bytes, want %d", len(data), len(fullOutput))
	}
}

func TestSpillCleanup_OldFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	spillPath := filepath.Join(dir, spillSubdir)
	if err := os.MkdirAll(spillPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create an old file (8 days ago).
	oldFile := filepath.Join(spillPath, "old-session-123.txt")
	if err := os.WriteFile(oldFile, []byte("old data"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Trigger a spill to run cleanup.
	tr := &TruncateResult{Content: "preview", Truncated: true, FullSize: 1000}
	SpillIfTruncated("sess2", "full content here", tr)

	// Old file should be gone.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("expected old spill file to be removed, but it still exists")
	}
}

func TestSpillCleanup_MaxFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	spillPath := filepath.Join(dir, spillSubdir)
	if err := os.MkdirAll(spillPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create spillMaxFiles files so the next spill pushes us over.
	for i := 0; i < spillMaxFiles; i++ {
		name := filepath.Join(spillPath, strings.Replace(
			time.Now().Add(time.Duration(i)*time.Millisecond).Format("20060102-150405.000"), ".", "-", 1)+".txt")
		if err := os.WriteFile(name, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Stagger mod times so cleanup ordering is deterministic.
		mt := time.Now().Add(-time.Duration(spillMaxFiles-i) * time.Minute)
		os.Chtimes(name, mt, mt)
	}

	// Trigger one more spill.
	tr := &TruncateResult{Content: "preview", Truncated: true, FullSize: 100}
	SpillIfTruncated("sess3", "new content", tr)

	entries, err := os.ReadDir(spillPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > spillMaxFiles {
		t.Errorf("expected at most %d files, got %d", spillMaxFiles, len(entries))
	}
}

func TestSpillCleanup_ErrorsDoNotFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	// Just verify the function completes without panic when dir doesn't exist.
	cleanupSpillDir(filepath.Join(dir, "nonexistent"))
	// No assertion needed — we're verifying it doesn't panic or propagate errors.
}
