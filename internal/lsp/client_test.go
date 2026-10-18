package lsp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func TestClient_Alive_FalseAfterDoneClosed(t *testing.T) {
	c := newClient(ServerSpec{Language: "go", Command: "true"}, t.TempDir(), nil, time.Second)
	if !c.Alive() {
		t.Fatal("expected Alive() true before done closed")
	}
	c.doneOnce.Do(func() { close(c.done) })
	if c.Alive() {
		t.Fatal("expected Alive() false after done closed")
	}
}

func TestSyncFile_LRUEvictsDidCloseAndClearsDiags(t *testing.T) {
	script := writeFakeLSPServer(t)
	logPath := filepath.Join(t.TempDir(), "log.jsonl")
	root := t.TempDir()
	client := connectFakeClient(t, script, root, map[string]string{"FAKE_LSP_LOG": logPath})

	ctx := context.Background()
	files := make([]string, maxOpenDocs+1)
	for i := 0; i < maxOpenDocs+1; i++ {
		files[i] = writeGoFile(t, root, filepath.Join("f"+strconv.Itoa(i)+".go"), "package main\n")
	}

	for i := 0; i < maxOpenDocs; i++ {
		changed, err := client.syncFile(ctx, files[i])
		if err != nil {
			t.Fatalf("syncFile[%d]: %v", i, err)
		}
		if !changed {
			t.Fatalf("syncFile[%d]: expected changed=true on first open", i)
		}
	}

	firstURI := fileURI(files[0])
	client.diagMu.Lock()
	client.diagStore[firstURI] = []Diagnostic{{Message: "stale"}}
	client.diagWaiters[firstURI] = []chan struct{}{make(chan struct{}, 1)}
	client.diagMu.Unlock()

	changed, err := client.syncFile(ctx, files[maxOpenDocs])
	if err != nil {
		t.Fatalf("syncFile 65th: %v", err)
	}
	if !changed {
		t.Fatal("expected 65th open to change")
	}

	waitForFakeLogMethod(t, logPath, "textDocument/didClose", 2*time.Second)

	client.docMu.Lock()
	_, stillOpen := client.openDocs[firstURI]
	nOpen := len(client.openDocs)
	client.docMu.Unlock()
	if stillOpen {
		t.Fatal("LRU victim should leave openDocs")
	}
	if nOpen != maxOpenDocs {
		t.Fatalf("openDocs len = %d, want %d", nOpen, maxOpenDocs)
	}

	client.diagMu.Lock()
	_, hasDiag := client.diagStore[firstURI]
	_, hasWaiter := client.diagWaiters[firstURI]
	client.diagMu.Unlock()
	if hasDiag {
		t.Fatal("diagStore entry should be cleared on eviction")
	}
	if hasWaiter {
		t.Fatal("diagWaiters entry should be cleared on eviction")
	}

	// Re-sync victim → fresh didOpen version 1
	beforeOpens := countFakeLogMethod(t, logPath, "textDocument/didOpen")
	changed, err = client.syncFile(ctx, files[0])
	if err != nil {
		t.Fatalf("re-sync victim: %v", err)
	}
	if !changed {
		t.Fatal("re-sync of evicted URI should changed=true (fresh didOpen)")
	}
	client.docMu.Lock()
	doc := client.openDocs[firstURI]
	client.docMu.Unlock()
	if doc == nil || doc.version != 1 {
		t.Fatalf("re-opened doc version = %v, want 1", doc)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countFakeLogMethod(t, logPath, "textDocument/didOpen") > beforeOpens {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected additional didOpen after re-sync")
}

func TestDiagnostics_UnchangedReturnsCacheImmediately(t *testing.T) {
	script := writeFakeLSPServer(t)
	root := t.TempDir()
	// Delay publish so a wait would be visible
	client := connectFakeClient(t, script, root, map[string]string{
		"FAKE_LSP_DIAG_DELAY": "2",
	})
	file := writeGoFile(t, root, "main.go", "package main\n")
	ctx := context.Background()

	start := time.Now()
	diags, err := client.Diagnostics(ctx, file)
	if err != nil {
		t.Fatalf("first Diagnostics: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("expected diagnostics from first call")
	}
	if time.Since(start) < time.Second {
		t.Fatalf("first call returned too quickly (%v); diag delay not applied?", time.Since(start))
	}

	// Seed a distinct cache value and ensure second call returns it immediately.
	uri := fileURI(file)
	client.diagMu.Lock()
	client.diagStore[uri] = []Diagnostic{{Message: "cached"}}
	client.diagMu.Unlock()

	start = time.Now()
	diags, err = client.Diagnostics(ctx, file)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("second Diagnostics: %v", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("unchanged Diagnostics took %v, want immediate cache return", elapsed)
	}
	if len(diags) != 1 || diags[0].Message != "cached" {
		t.Fatalf("diags = %+v, want cached message", diags)
	}
}

func TestDiagnostics_ChangedWaitsForPublish(t *testing.T) {
	script := writeFakeLSPServer(t)
	root := t.TempDir()
	client := connectFakeClient(t, script, root, map[string]string{
		"FAKE_LSP_DIAG_DELAY": "0.3",
	})
	file := writeGoFile(t, root, "main.go", "package main\n")
	ctx := context.Background()

	if _, err := client.Diagnostics(ctx, file); err != nil {
		t.Fatalf("initial Diagnostics: %v", err)
	}

	if err := os.WriteFile(file, []byte("package main\nfunc f() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	diags, err := client.Diagnostics(ctx, file)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("changed Diagnostics: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("expected diagnostics after change")
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("changed path returned in %v; expected to wait for publish", elapsed)
	}
}

func TestRequestWithTimeout_UsesProvidedDuration(t *testing.T) {
	// Scaled: client timeout 200ms, ConnectTimeout 2s, init reply at ~400ms.
	// Proves initialize uses ConnectTimeout, not c.timeout (AC: 30s/120s/~45s).
	script := writeFakeLSPServer(t)
	spec := fakeLSPSpec(script)
	spec.ConnectTimeout = 2 * time.Second
	client := newClient(spec, t.TempDir(), map[string]string{
		"FAKE_LSP_INIT_DELAY":     "0.4",
		"FAKE_LSP_HANG_AFTER_INIT": "1",
	}, 200*time.Millisecond)
	defer client.close()

	ctx := context.Background()
	start := time.Now()
	if err := client.connect(ctx); err != nil {
		t.Fatalf("connect with ConnectTimeout should succeed after delayed init: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("expected to wait for delayed init, elapsed %v", elapsed)
	}

	// Later non-initialize RPC still uses client timeout (200ms), not ConnectTimeout.
	rpcStart := time.Now()
	_, err := client.request(ctx, "workspace/symbol", map[string]any{"query": "x"})
	if err == nil || !strings.Contains(err.Error(), "request timeout") {
		t.Fatalf("expected client-timeout on later RPC, got %v", err)
	}
	if elapsed := time.Since(rpcStart); elapsed > time.Second {
		t.Fatalf("later RPC should time out at client timeout (~200ms), took %v", elapsed)
	}
}

func TestRequestWithTimeout_ZeroFallsBackToClientTimeout(t *testing.T) {
	script := writeFakeLSPServer(t)
	spec := fakeLSPSpec(script)
	spec.ConnectTimeout = 0
	client := newClient(spec, t.TempDir(), map[string]string{
		"FAKE_LSP_INIT_DELAY": "0.3",
	}, 100*time.Millisecond)
	defer client.close()

	err := client.connect(context.Background())
	if err == nil {
		t.Fatal("expected initialize to time out at client timeout when ConnectTimeout=0")
	}
	if !strings.Contains(err.Error(), "request timeout") {
		t.Fatalf("expected request timeout, got %v", err)
	}
}

func TestConnect_InitializeTimeoutDoesNotDeadlineCommandContext(t *testing.T) {
	// Non-negotiable #799: ConnectTimeout must NOT be applied via context.WithTimeout
	// on CommandContext / the connect ctx. Timeout is select-only on initialize.
	script := writeFakeLSPServer(t)
	spec := fakeLSPSpec(script)
	spec.ConnectTimeout = 150 * time.Millisecond
	client := newClient(spec, t.TempDir(), map[string]string{
		"FAKE_LSP_INIT_HANG": "1",
	}, 30*time.Second)

	ctx := context.Background() // no deadline — must stay that way
	err := client.connect(ctx)
	if err == nil {
		_ = client.close()
		t.Fatal("expected initialize timeout")
	}
	if !strings.Contains(err.Error(), "request timeout") {
		t.Fatalf("expected request timeout (select path), got %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("parent connect ctx must not be cancelled by ConnectTimeout: %v", ctx.Err())
	}
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("parent connect ctx must not gain a deadline from ConnectTimeout")
	}
	// Error must not be context.DeadlineExceeded from a WithTimeout wrapper.
	if strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("initialize must not fail via CommandContext deadline: %v", err)
	}
}

func TestConnect_InitializeOmitsDidSaveTrue(t *testing.T) {
	script := writeFakeLSPServer(t)
	logPath := filepath.Join(t.TempDir(), "log.jsonl")
	_ = connectFakeClient(t, script, t.TempDir(), map[string]string{"FAKE_LSP_LOG": logPath})

	initMsg := waitForFakeLogMethod(t, logPath, "initialize", 2*time.Second)
	params, _ := initMsg["params"].(map[string]any)
	caps, _ := params["capabilities"].(map[string]any)
	td, _ := caps["textDocument"].(map[string]any)
	sync, ok := td["synchronization"].(map[string]any)
	if ok {
		if v, has := sync["didSave"]; has && v == true {
			t.Fatalf("initialize capabilities include didSave:true: %#v", sync)
		}
	}
	raw, _ := json.Marshal(params)
	if strings.Contains(string(raw), `"didSave":true`) || strings.Contains(string(raw), `"didSave": true`) {
		t.Fatalf("initialize params contain didSave true: %s", raw)
	}
}

func TestConnect_StderrBoundedOnFailure(t *testing.T) {
	// Script writes >64KiB to stderr then exits without speaking LSP.
	script := filepath.Join(t.TempDir(), "noisy.sh")
	body := "#!/bin/sh\npython3 -c 'import sys; sys.stderr.write(\"x\"*" + strconv.Itoa(maxStderrSize+4096) + "); sys.stderr.flush()'\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	client := newClient(ServerSpec{
		Language: "go",
		Command:  script,
	}, t.TempDir(), nil, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := client.connect(ctx)
	if err == nil {
		_ = client.close()
		t.Fatal("expected connect failure")
	}
	got := client.stderrSnippet()
	if len(got) > maxStderrSize {
		t.Fatalf("stderr len = %d, want <= %d", len(got), maxStderrSize)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty stderr capture")
	}
}

func TestCappedBuffer_RetainsLastBytes(t *testing.T) {
	buf := &cappedBuffer{max: 10}
	if _, err := buf.Write([]byte("abcdefghijXXXX")); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "efghijXXXX" {
		t.Fatalf("String() = %q, want %q", got, "efghijXXXX")
	}
}

func TestParseDocumentSymbols_Hierarchical(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"name": "Foo",
			"kind": 5,
			"range": {"start": {"line": 0, "character": 0}, "end": {"line": 10, "character": 0}},
			"selectionRange": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 3}},
			"children": [
				{
					"name": "bar",
					"kind": 6,
					"range": {"start": {"line": 2, "character": 0}, "end": {"line": 4, "character": 0}},
					"selectionRange": {"start": {"line": 2, "character": 4}, "end": {"line": 2, "character": 7}},
					"children": []
				}
			]
		}
	]`)
	nodes, err := parseDocumentSymbols(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
	}
	if nodes[0].Name != "Foo" || nodes[0].Kind != 5 || nodes[0].Line != 0 {
		t.Fatalf("root = %+v", nodes[0])
	}
	if len(nodes[0].Children) != 1 {
		t.Fatalf("children len = %d, want 1", len(nodes[0].Children))
	}
	child := nodes[0].Children[0]
	if child.Name != "bar" || child.Kind != 6 || child.Line != 2 {
		t.Fatalf("child = %+v", child)
	}
	if child.Flat || nodes[0].Flat {
		t.Fatal("hierarchical nodes should not be Flat")
	}
}

func TestParseDocumentSymbols_SymbolInformation(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"name": "Println",
			"kind": 12,
			"location": {
				"uri": "file:///fmt/print.go",
				"range": {"start": {"line": 273, "character": 5}, "end": {"line": 273, "character": 12}}
			},
			"containerName": "fmt"
		},
		{
			"name": "Writer",
			"kind": 11,
			"location": {
				"uri": "file:///io/io.go",
				"range": {"start": {"line": 99, "character": 5}, "end": {"line": 99, "character": 11}}
			}
		}
	]`)
	nodes, err := parseDocumentSymbols(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len = %d, want 2", len(nodes))
	}
	if !nodes[0].Flat || nodes[0].Name != "Println" || nodes[0].ContainerName != "fmt" || nodes[0].Line != 273 {
		t.Fatalf("nodes[0] = %+v", nodes[0])
	}
	if !nodes[1].Flat || nodes[1].Name != "Writer" || nodes[1].Line != 99 {
		t.Fatalf("nodes[1] = %+v", nodes[1])
	}
}

func TestClient_Rename_FakeServer(t *testing.T) {
	script := writeFakeLSPServer(t)
	root := t.TempDir()
	file := writeGoFile(t, root, "main.go", "package main\n")
	client := connectFakeClient(t, script, root, nil)

	edit, err := client.Rename(context.Background(), file, Position{Line: 0, Character: 0}, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if edit == nil || len(edit.Changes) < 2 {
		t.Fatalf("expected multi-file WorkspaceEdit, got %#v", edit)
	}
}

func TestClient_CodeActions_FakeServer(t *testing.T) {
	script := writeFakeLSPServer(t)
	root := t.TempDir()
	file := writeGoFile(t, root, "main.go", "package main\n")
	client := connectFakeClient(t, script, root, nil)

	actions, err := client.CodeActions(context.Background(), file, Range{
		Start: Position{Line: 0, Character: 0},
		End:   Position{Line: 0, Character: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) < 1 {
		t.Fatal("expected at least one code action")
	}
	if actions[0].Title == "" {
		t.Fatal("empty title")
	}
}

func TestWorkspaceEdit_JSONRoundTrip_ChangesAndDocumentChanges(t *testing.T) {
	rawChanges := `{
		"changes": {
			"file:///a.go": [
				{"range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 3}}, "newText": "Foo"}
			]
		}
	}`
	var editChanges WorkspaceEdit
	if err := json.Unmarshal([]byte(rawChanges), &editChanges); err != nil {
		t.Fatal(err)
	}
	if len(editChanges.Changes["file:///a.go"]) != 1 {
		t.Fatalf("changes len = %d", len(editChanges.Changes["file:///a.go"]))
	}
	if editChanges.Changes["file:///a.go"][0].NewText != "Foo" {
		t.Fatalf("newText = %q", editChanges.Changes["file:///a.go"][0].NewText)
	}
	round, err := json.Marshal(editChanges)
	if err != nil {
		t.Fatal(err)
	}
	var again WorkspaceEdit
	if err := json.Unmarshal(round, &again); err != nil {
		t.Fatal(err)
	}
	if again.Changes["file:///a.go"][0].NewText != "Foo" {
		t.Fatal("changes round-trip failed")
	}

	rawDoc := `{
		"documentChanges": [{
			"textDocument": {"uri": "file:///b.go", "version": 2},
			"edits": [{"range": {"start": {"line": 1, "character": 0}, "end": {"line": 1, "character": 1}}, "newText": "x"}]
		}]
	}`
	var editDoc WorkspaceEdit
	if err := json.Unmarshal([]byte(rawDoc), &editDoc); err != nil {
		t.Fatal(err)
	}
	if len(editDoc.DocumentChanges) != 1 {
		t.Fatalf("documentChanges len = %d", len(editDoc.DocumentChanges))
	}
	var tde TextDocumentEdit
	if err := json.Unmarshal(editDoc.DocumentChanges[0], &tde); err != nil {
		t.Fatal(err)
	}
	if tde.TextDocument.URI != "file:///b.go" || len(tde.Edits) != 1 || tde.Edits[0].NewText != "x" {
		t.Fatalf("TextDocumentEdit = %+v", tde)
	}
}

func TestConnect_InitializeAdvertisesCodeActionRenameApplyEdit(t *testing.T) {
	script := writeFakeLSPServer(t)
	logPath := filepath.Join(t.TempDir(), "log.jsonl")
	_ = connectFakeClient(t, script, t.TempDir(), map[string]string{"FAKE_LSP_LOG": logPath})

	initMsg := waitForFakeLogMethod(t, logPath, "initialize", 2*time.Second)
	params, _ := initMsg["params"].(map[string]any)
	caps, _ := params["capabilities"].(map[string]any)
	td, _ := caps["textDocument"].(map[string]any)
	ws, _ := caps["workspace"].(map[string]any)

	for _, key := range []string{"hover", "definition", "references", "documentSymbol", "codeAction", "rename"} {
		if _, ok := td[key]; !ok {
			t.Errorf("textDocument.%s missing", key)
		}
	}
	if _, ok := ws["symbol"]; !ok {
		t.Error("workspace.symbol missing")
	}
	if v, ok := ws["applyEdit"].(bool); !ok || !v {
		t.Fatalf("workspace.applyEdit = %#v, want true", ws["applyEdit"])
	}
	raw, _ := json.Marshal(params)
	if strings.Contains(string(raw), `"didSave":true`) || strings.Contains(string(raw), `"didSave": true`) {
		t.Fatalf("initialize params contain didSave true: %s", raw)
	}
}

func TestReadLoop_ApplyEditReturnsAppliedFalse(t *testing.T) {
	script := writeFakeLSPServer(t)
	logPath := filepath.Join(t.TempDir(), "log.jsonl")
	_ = connectFakeClient(t, script, t.TempDir(), map[string]string{
		"FAKE_LSP_LOG":            logPath,
		"FAKE_LSP_SEND_APPLY_EDIT": "1",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, msg := range readFakeLSPLog(t, logPath) {
			if msg["method"] != nil {
				continue
			}
			id, hasID := msg["id"]
			result, hasResult := msg["result"]
			if !hasID || !hasResult || id == nil {
				continue
			}
			resultMap, ok := result.(map[string]any)
			if !ok {
				continue
			}
			if applied, ok := resultMap["applied"].(bool); ok && !applied {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for applyEdit response {applied:false}")
}

func TestReadLoop_OtherServerRequestStillNull(t *testing.T) {
	script := writeFakeLSPServer(t)
	logPath := filepath.Join(t.TempDir(), "log.jsonl")
	_ = connectFakeClient(t, script, t.TempDir(), map[string]string{
		"FAKE_LSP_LOG":               logPath,
		"FAKE_LSP_SEND_OTHER_REQUEST": "1",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, msg := range readFakeLSPLog(t, logPath) {
			if msg["method"] != nil {
				continue
			}
			id, hasID := msg["id"]
			if !hasID || id == nil {
				continue
			}
			// JSON null unmarshals to nil interface
			if result, ok := msg["result"]; ok && result == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for null result on non-applyEdit server request")
}
