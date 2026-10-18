package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestRunSemgrep_LookPathMiss(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := runSemgrep(context.Background(), ".", defaultSemgrepConfig, "")
	if err == nil {
		t.Fatal("expected LookPath error")
	}
	if !strings.Contains(err.Error(), "pip install semgrep") || !strings.Contains(err.Error(), "brew install semgrep") {
		t.Fatalf("expected install hint, got %v", err)
	}
}

func TestScanTool_DefaultConfig(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "semgrep")
	body := "#!/bin/sh\necho \"$@\" > '" + argsPath + "'\nprintf '%s\\n' '{\"results\":[]}'\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, err := scanTool().Execute(context.Background(), []byte(`{"path":"src"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "No Semgrep findings") {
		t.Fatalf("got %q", got)
	}
	recorded, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(recorded), "--config="+defaultSemgrepConfig) {
		t.Fatalf("args = %q, want default config", recorded)
	}
}

func TestScanTool_OverrideConfig(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "semgrep")
	body := "#!/bin/sh\necho \"$@\" > '" + argsPath + "'\nprintf '%s\\n' '{\"results\":[]}'\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	_, err := scanTool().Execute(context.Background(), []byte(`{"path":"src","config":"custom"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(recorded), "--config=custom") {
		t.Fatalf("args = %q", recorded)
	}
}

func TestScanTool_FindingsExit1JSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "semgrep")
	jsonOut := `{"results":[{"check_id":"java.lang.security.audit.crypto.use-of-md5","path":"A.java","start":{"line":10,"col":1},"extra":{"message":"MD5 is weak","severity":"WARNING"}}]}`
	body := "#!/bin/sh\nprintf '%s\\n' '" + jsonOut + "'\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, err := scanTool().Execute(context.Background(), []byte(`{"path":"A.java"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("findings should not hard-error: %v", err)
	}
	if !strings.Contains(got, "MD5 is weak") || !strings.Contains(got, "findings: 1") {
		t.Fatalf("got %q", got)
	}
}

func TestScanTool_Timeout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "semgrep")
	// Busy-wait without relying on sleep (PATH is only this dir).
	if err := os.WriteFile(script, []byte("#!/bin/sh\nwhile true; do :; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	start := time.Now()
	_, err := runSemgrep(context.Background(), ".", defaultSemgrepConfig, "")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timed out, got %v", err)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("elapsed %v exceeds host 30s budget; want ≤25s cmd timeout", elapsed)
	}
	if elapsed < 20*time.Second {
		t.Fatalf("elapsed %v too short for 25s timeout (clock skew?)", elapsed)
	}
}

func TestFormatFindings_Truncates(t *testing.T) {
	var results []semgrepFinding
	for i := 0; i < 500; i++ {
		f := semgrepFinding{
			CheckID: "rule." + strings.Repeat("x", 80),
			Path:    "File.java",
		}
		f.Start.Line = i + 1
		f.Extra.Message = strings.Repeat("m", 200)
		f.Extra.Severity = "ERROR"
		results = append(results, f)
	}
	got := formatFindings(&semgrepResult{Results: results}, "p/java-security")
	if len(got) > maxScanOutputBytes+200 {
		t.Fatalf("output len %d exceeds soft cap", len(got))
	}
	if !strings.Contains(got, "omitted") {
		t.Fatalf("expected omission note in output")
	}
}
