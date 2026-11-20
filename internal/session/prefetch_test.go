package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefetchFiles_SmallProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "util.go", "package main\n\nfunc helper() {}\n")

	text, paths := PrefetchFiles(dir, "**/*.{go}", 100*1024)

	if text == "" {
		t.Fatal("expected non-empty prefetch text")
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if !strings.Contains(text, "## Project files (prefetched)") {
		t.Error("expected section header in output")
	}
	if !strings.Contains(text, "main.go") {
		t.Error("expected main.go in output")
	}
	if !strings.Contains(text, "util.go") {
		t.Error("expected util.go in output")
	}
	if !strings.Contains(text, "```go") {
		t.Error("expected go code fence in output")
	}
	if !strings.Contains(text, "func main()") {
		t.Error("expected file content in output")
	}
	// Paths should be absolute.
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("expected absolute path, got %q", p)
		}
	}
}

func TestPrefetchFiles_ExceedsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	// Write a file just under the max single file size.
	big := strings.Repeat("x", 40*1024)
	writeFile(t, dir, "big1.go", big)
	writeFile(t, dir, "big2.go", big)
	writeFile(t, dir, "big3.go", big)

	// Total = 120KB, maxBytes = 100KB
	text, paths := PrefetchFiles(dir, "**/*.{go}", 100*1024)

	if text != "" {
		t.Error("expected empty prefetch text when exceeding max bytes")
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestPrefetchFiles_NoPrefetchField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")

	text, paths := PrefetchFiles(dir, "", 100*1024)

	if text != "" {
		t.Error("expected empty text for empty pattern")
	}
	if len(paths) != 0 {
		t.Error("expected no paths for empty pattern")
	}
}

func TestPrefetchFiles_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.go", "package main\n")

	// Write a binary file with invalid UTF-8.
	binPath := filepath.Join(dir, "binary.go")
	if err := os.WriteFile(binPath, []byte{0x00, 0xFF, 0xFE, 0x80}, 0644); err != nil {
		t.Fatal(err)
	}

	text, paths := PrefetchFiles(dir, "**/*.{go}", 100*1024)

	if len(paths) != 1 {
		t.Fatalf("expected 1 path (binary skipped), got %d", len(paths))
	}
	if !strings.Contains(text, "good.go") {
		t.Error("expected good.go in output")
	}
	if strings.Contains(text, "binary.go") {
		t.Error("expected binary.go to be skipped")
	}
}

func TestPrefetchFiles_SkipsLargeIndividualFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "small.go", "package main\n")
	// Write a file > 50KB.
	large := strings.Repeat("x", 51*1024)
	writeFile(t, dir, "large.go", large)

	text, paths := PrefetchFiles(dir, "**/*.{go}", 200*1024)

	if len(paths) != 1 {
		t.Fatalf("expected 1 path (large file skipped), got %d", len(paths))
	}
	if !strings.Contains(text, "small.go") {
		t.Error("expected small.go in output")
	}
	if strings.Contains(text, "large.go") {
		t.Error("expected large.go to be skipped")
	}
}

func TestPrefetchFiles_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "visible.go", "package main\n")
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	writeFile(t, dir, ".git/config.go", "package git\n")

	text, paths := PrefetchFiles(dir, "**/*.{go}", 100*1024)

	if len(paths) != 1 {
		t.Fatalf("expected 1 path (.git skipped), got %d", len(paths))
	}
	if strings.Contains(text, "config.go") {
		t.Error("expected .git/config.go to be skipped")
	}
}

func TestPrefetchFiles_DefaultMaxBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")

	// maxBytes=0 should use default.
	text, paths := PrefetchFiles(dir, "**/*.{go}", 0)

	if text == "" {
		t.Error("expected non-empty text with default max bytes")
	}
	if len(paths) != 1 {
		t.Errorf("expected 1 path, got %d", len(paths))
	}
}

func TestPrefetchFiles_MultipleExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "app.py", "print('hello')\n")
	writeFile(t, dir, "readme.txt", "just a readme\n")

	text, paths := PrefetchFiles(dir, "**/*.{go,py}", 100*1024)

	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if !strings.Contains(text, "main.go") {
		t.Error("expected main.go in output")
	}
	if !strings.Contains(text, "app.py") {
		t.Error("expected app.py in output")
	}
	if strings.Contains(text, "readme.txt") {
		t.Error("expected readme.txt to be excluded")
	}
}

func TestPrefetchFiles_SubdirectoryFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pkg", "util"), 0755)
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "pkg/util/helper.go", "package util\n")

	text, paths := PrefetchFiles(dir, "**/*.{go}", 100*1024)

	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if !strings.Contains(text, "pkg/util/helper.go") {
		t.Error("expected relative path pkg/util/helper.go in output")
	}
}

func TestParseExtensions_BraceExpansion(t *testing.T) {
	exts := parseExtensions("**/*.{go,py,js}")
	want := map[string]bool{".go": true, ".py": true, ".js": true}
	if len(exts) != len(want) {
		t.Fatalf("expected %d extensions, got %d", len(want), len(exts))
	}
	for k := range want {
		if !exts[k] {
			t.Errorf("expected extension %q", k)
		}
	}
}

func TestParseExtensions_SimpleGlob(t *testing.T) {
	exts := parseExtensions("**/*.go")
	if len(exts) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(exts))
	}
	if !exts[".go"] {
		t.Error("expected .go extension")
	}
}

func TestLangFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".rs", "rust"},
		{".xyz", ""},
	}
	for _, tt := range tests {
		if got := langFromExt(tt.ext); got != tt.want {
			t.Errorf("langFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
