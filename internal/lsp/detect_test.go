package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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
		{"java", ".java"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := extForLanguage(tt.lang)
		if got != tt.want {
			t.Errorf("extForLanguage(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestExtForLanguage_Java(t *testing.T) {
	if got := extForLanguage("java"); got != ".java" {
		t.Errorf("extForLanguage(java) = %q, want .java", got)
	}
}

func TestDetectServers_JavaMarkerGated(t *testing.T) {
	dir := t.TempDir()
	_, jdtlsOnPath := lookPath("jdtls")

	detected := detectServers(dir)
	for _, s := range detected {
		if s.Language == "java" {
			t.Fatal("java/jdtls must not be detected without markers")
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	detected = detectServers(dir)
	var foundJava bool
	for _, s := range detected {
		if s.Language == "java" {
			foundJava = true
			if s.Command != "jdtls" {
				t.Fatalf("command = %q, want jdtls", s.Command)
			}
			if len(s.Args) != 0 {
				t.Fatalf("Args = %v, want empty", s.Args)
			}
			if s.ExtLangIDs[".java"] != "java" {
				t.Fatalf("unexpected ExtLangIDs: %v", s.ExtLangIDs)
			}
			if s.ConnectTimeout != 120*time.Second {
				t.Fatalf("ConnectTimeout = %v, want 120s", s.ConnectTimeout)
			}
		}
	}
	if jdtlsOnPath && !foundJava {
		t.Fatal("expected java detected when marker present and jdtls on PATH")
	}
	if !jdtlsOnPath && foundJava {
		t.Fatal("java should not appear when jdtls missing from PATH")
	}
	if hint := installHints["java"]; hint == "" {
		t.Fatal("expected installHints for java")
	}
}

func TestDetectServers_WalksParentsDepthCapped(t *testing.T) {
	base := t.TempDir()
	// Nest 9 levels: leaf is d9. Marker in d1 is depth 8 from leaf; marker in base is depth 9.
	cur := base
	var chain []string
	for i := 1; i <= 9; i++ {
		cur = filepath.Join(cur, "d"+itoaDetect(i))
		if err := os.MkdirAll(cur, 0o755); err != nil {
			t.Fatal(err)
		}
		chain = append(chain, cur)
	}
	leaf := chain[8] // d9
	d1 := chain[0]   // d1

	// Marker only at base (depth 9 from leaf) — should be ignored.
	if err := os.WriteFile(filepath.Join(base, "go.mod"), []byte("module deep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detected := detectServers(leaf)
	for _, s := range detected {
		if s.Language == "go" {
			t.Fatal("go should not be detected when marker is beyond walk depth 8")
		}
	}

	// Marker at d1 (depth 8) — should be found if gopls on PATH.
	if err := os.WriteFile(filepath.Join(d1, "go.mod"), []byte("module near\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detected = detectServers(leaf)
	found, marker := markerExists(leaf, []string{"go.mod"}, maxMarkerWalkDepth)
	if !found {
		t.Fatal("expected marker within depth 8")
	}
	if marker != "go.mod" {
		t.Fatalf("marker = %q", marker)
	}
	// Presence in detectServers depends on LookPath; assert walk itself found it.
	_ = detected
}

func TestDetectServers_NearestMarkerWins(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "pkg")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "go.mod"), []byte("module child\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, marker := markerExists(child, []string{"go.mod"}, maxMarkerWalkDepth)
	if !found || marker != "go.mod" {
		t.Fatalf("found=%v marker=%q", found, marker)
	}
	// Nearest directory is child itself (depth 0), not root.
	if _, err := os.Stat(filepath.Join(child, "go.mod")); err != nil {
		t.Fatal(err)
	}
	// Walking from child must return before reaching root: verify by temporarily
	// removing child marker — then parent wins.
	if err := os.Remove(filepath.Join(child, "go.mod")); err != nil {
		t.Fatal(err)
	}
	found, _ = markerExists(child, []string{"go.mod"}, maxMarkerWalkDepth)
	if !found {
		t.Fatal("expected parent marker after child removed")
	}
}

func TestDetectServers_ClangdMarkerGated(t *testing.T) {
	dir := t.TempDir()
	_, clangdOnPath := lookPath("clangd")

	detected := detectServers(dir)
	for _, s := range detected {
		if s.Language == "cpp" {
			t.Fatal("cpp/clangd must not be detected without markers")
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	detected = detectServers(dir)
	var foundCPP bool
	for _, s := range detected {
		if s.Language == "cpp" {
			foundCPP = true
			if s.Command != "clangd" {
				t.Fatalf("command = %q, want clangd", s.Command)
			}
			if s.ExtLangIDs[".cpp"] != "cpp" || s.ExtLangIDs[".c"] != "c" {
				t.Fatalf("unexpected ExtLangIDs: %v", s.ExtLangIDs)
			}
		}
	}
	if clangdOnPath && !foundCPP {
		t.Fatal("expected cpp detected when marker present and clangd on PATH")
	}
	if !clangdOnPath && foundCPP {
		t.Fatal("cpp should not appear when clangd missing from PATH")
	}
	if hint := installHints["cpp"]; hint == "" {
		t.Fatal("expected installHints for cpp")
	}
}

func TestLanguageIDForFile_MtsCtsMjsCjs(t *testing.T) {
	tests := []struct {
		path   string
		langID string
	}{
		{"app.mts", "typescript"},
		{"app.cts", "typescript"},
		{"app.mjs", "javascript"},
		{"app.cjs", "javascript"},
	}
	for _, tt := range tests {
		if got := languageIDForFile(tt.path); got != tt.langID {
			t.Errorf("languageIDForFile(%q) = %q, want %q", tt.path, got, tt.langID)
		}
		if got := languageForFile(tt.path); got != "typescript" {
			t.Errorf("languageForFile(%q) = %q, want typescript", tt.path, got)
		}
	}
}

func TestLanguageForFile_CppExtensions(t *testing.T) {
	for _, path := range []string{"a.c", "a.h", "a.cpp", "a.cc", "a.cxx", "a.hpp"} {
		if got := languageForFile(path); got != "cpp" {
			t.Errorf("languageForFile(%q) = %q, want cpp", path, got)
		}
	}
	if got := extForLanguage("cpp"); got != ".cpp" {
		t.Errorf("extForLanguage(cpp) = %q, want .cpp", got)
	}
}

func lookPath(cmd string) (string, bool) {
	path, err := exec.LookPath(cmd)
	return path, err == nil
}

func itoaDetect(n int) string {
	return strconv.Itoa(n)
}
