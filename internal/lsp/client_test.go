package lsp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConnect_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	out := filepath.Join(t.TempDir(), "env.txt")
	script := `printf '%s|%s' "$OPENROUTER_API_KEY" "$LSP_TOKEN" > '` + strings.ReplaceAll(out, `'`, `'\''`) + `'`
	client := newClient(ServerSpec{
		Command: "sh",
		Args:    []string{"-c", script},
	}, t.TempDir(), map[string]string{"LSP_TOKEN": "from-config"}, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.connect(ctx); err == nil {
		t.Fatal("expected initialize to fail for a shell that is not a language server")
	}

	deadline := time.Now().Add(2 * time.Second)
	var got []byte
	var err error
	for time.Now().Before(deadline) {
		got, err = os.ReadFile(out)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "|from-config" {
		t.Fatalf("child env = %q, want the configured token and not the parent API key", got)
	}
}

func TestFileURI_EncodesSpacesAndHash(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(dir, "my file.go")
	hashed := filepath.Join(dir, "a#b.go")

	uri := fileURI(spaced)
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("fileURI(%q) = %q, want file:// prefix", spaced, uri)
	}
	if strings.Contains(uri, " ") {
		t.Errorf("fileURI must percent-encode spaces, got %q", uri)
	}
	if !strings.Contains(uri, "%20") {
		t.Errorf("fileURI(%q) = %q, want %%20 for space", spaced, uri)
	}

	uriHash := fileURI(hashed)
	if strings.Contains(uriHash, "#") {
		t.Errorf("fileURI must percent-encode #, got %q", uriHash)
	}
	if !strings.Contains(uriHash, "%23") {
		t.Errorf("fileURI(%q) = %q, want %%23 for #", hashed, uriHash)
	}

	roundTrip := fileFromURI(uri)
	if filepath.Clean(roundTrip) != filepath.Clean(spaced) {
		t.Errorf("fileFromURI(fileURI(%q)) = %q, want %q", spaced, roundTrip, spaced)
	}
}

func TestRemoveDiagWaiter_CleansOnAllPaths(t *testing.T) {
	c := newClient(ServerSpec{Language: "go", Command: "true"}, t.TempDir(), nil, time.Second)
	uri := "file:///tmp/test.go"
	ch := make(chan struct{}, 1)

	c.diagMu.Lock()
	c.diagWaiters[uri] = append(c.diagWaiters[uri], ch)
	c.diagMu.Unlock()

	c.removeDiagWaiter(uri, ch)

	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	if _, ok := c.diagWaiters[uri]; ok {
		t.Errorf("waiter should be removed, got %v", c.diagWaiters[uri])
	}
}

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

func TestParseHoverContents_MarkupContent(t *testing.T) {
	raw := json.RawMessage(`{"kind":"markdown","value":"func Println(a ...any)"}`)
	got := parseHoverContents(raw)
	if got != "func Println(a ...any)" {
		t.Errorf("parseHoverContents(MarkupContent) = %q, want %q", got, "func Println(a ...any)")
	}
}

func TestParseHoverContents_PlainString(t *testing.T) {
	raw := json.RawMessage(`"this is a plain hover string"`)
	got := parseHoverContents(raw)
	if got != "this is a plain hover string" {
		t.Errorf("parseHoverContents(string) = %q, want %q", got, "this is a plain hover string")
	}
}

func TestParseHoverContents_ArrayWithLanguageBlocks(t *testing.T) {
	raw := json.RawMessage(`[{"language":"go","value":"func Foo()"},{"language":"","value":"Documentation for Foo"},"plain text"]`)
	got := parseHoverContents(raw)
	if !strings.Contains(got, "```go\nfunc Foo()\n```") {
		t.Errorf("expected go code block, got %q", got)
	}
	if !strings.Contains(got, "Documentation for Foo") {
		t.Errorf("expected documentation text, got %q", got)
	}
	if !strings.Contains(got, "plain text") {
		t.Errorf("expected plain text item, got %q", got)
	}
}

func TestParseHoverContents_ArrayOfPlainStrings(t *testing.T) {
	raw := json.RawMessage(`["first line","second line"]`)
	got := parseHoverContents(raw)
	if !strings.Contains(got, "first line") || !strings.Contains(got, "second line") {
		t.Errorf("parseHoverContents(string array) = %q, want both lines", got)
	}
}

func TestParseHoverContents_FallbackRawJSON(t *testing.T) {
	// A number is not a valid hover content type; should fall through to raw.
	raw := json.RawMessage(`42`)
	got := parseHoverContents(raw)
	if got != "42" {
		t.Errorf("parseHoverContents(raw) = %q, want %q", got, "42")
	}
}

func TestParseLocations_NullReturnsNil(t *testing.T) {
	locs, err := parseLocations(json.RawMessage("null"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locs != nil {
		t.Errorf("expected nil, got %v", locs)
	}
}

func TestParseLocations_EmptyReturnsNil(t *testing.T) {
	locs, err := parseLocations(json.RawMessage(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locs != nil {
		t.Errorf("expected nil, got %v", locs)
	}
}

func TestParseLocations_SingleLocation(t *testing.T) {
	raw := json.RawMessage(`{"uri":"file:///src/main.go","range":{"start":{"line":10,"character":5},"end":{"line":10,"character":15}}}`)
	locs, err := parseLocations(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if locs[0].URI != "file:///src/main.go" {
		t.Errorf("URI = %q, want %q", locs[0].URI, "file:///src/main.go")
	}
	if locs[0].Range.Start.Line != 10 {
		t.Errorf("start line = %d, want 10", locs[0].Range.Start.Line)
	}
}

func TestParseLocations_ArrayOfLocations(t *testing.T) {
	raw := json.RawMessage(`[
		{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":5}}},
		{"uri":"file:///b.go","range":{"start":{"line":20,"character":3},"end":{"line":20,"character":10}}}
	]`)
	locs, err := parseLocations(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(locs))
	}
	if locs[0].URI != "file:///a.go" {
		t.Errorf("locs[0].URI = %q, want file:///a.go", locs[0].URI)
	}
	if locs[1].Range.Start.Line != 20 {
		t.Errorf("locs[1].Range.Start.Line = %d, want 20", locs[1].Range.Start.Line)
	}
}

func TestFileFromURI_PercentEncodedPath(t *testing.T) {
	uri := "file:///path/with%20space/file%23hash.go"
	got := fileFromURI(uri)
	if !strings.Contains(got, "with space") {
		t.Errorf("fileFromURI(%q) = %q, expected decoded space", uri, got)
	}
	if !strings.Contains(got, "file#hash.go") {
		t.Errorf("fileFromURI(%q) = %q, expected decoded hash", uri, got)
	}
}

func TestFileFromURI_InvalidScheme(t *testing.T) {
	// Non-file URI should strip file:// prefix or return as-is.
	uri := "http://example.com/file.go"
	got := fileFromURI(uri)
	if got == "" {
		t.Error("fileFromURI should return non-empty for non-file URI")
	}
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
