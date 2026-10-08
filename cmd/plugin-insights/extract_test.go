package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func writeTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if strings.HasSuffix(name, "/") {
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
			hdr.Mode = 0o755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeDir {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGz_RejectsOversizedFile(t *testing.T) {
	orig := maxFileBytes
	maxFileBytes = 8
	t.Cleanup(func() { maxFileBytes = orig })

	archive := writeTarGz(t, map[string]string{
		"config/":        "",
		"config/big.txt": "0123456789abcdef",
	})
	dest := t.TempDir()
	_, err := extractTarGz(archive, dest)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "config", "big.txt")); !os.IsNotExist(statErr) {
		t.Fatal("truncated file was kept")
	}
}

func TestInsightsUse_ReplacesAndDisposesRoot(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"config/":                    "",
		"config/clusterversion.json": "{}",
	})
	p := newPlugin()
	tool := p.Tools[0]
	if tool.Name != "insights_use" {
		t.Fatalf("tool = %s", tool.Name)
	}
	args, _ := json.Marshal(map[string]string{"archive": archive})
	first, err := tool.Execute(t.Context(), args, plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := tool.Execute(t.Context(), args, plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	firstRoot := rootFromSummary(t, first)
	secondRoot := rootFromSummary(t, second)
	if _, err := os.Stat(firstRoot); !os.IsNotExist(err) {
		t.Fatalf("previous root %s still exists", firstRoot)
	}
	if _, err := os.Stat(secondRoot); err != nil {
		t.Fatalf("current root missing: %v", err)
	}
	if p.Hooks.Dispose == nil {
		t.Fatal("missing Dispose hook")
	}
	if err := p.Hooks.Dispose(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secondRoot); !os.IsNotExist(err) {
		t.Fatalf("dispose left %s", secondRoot)
	}
}

func rootFromSummary(t *testing.T, summary string) string {
	t.Helper()
	const prefix = "Extracted "
	if !strings.HasPrefix(summary, prefix) {
		t.Fatalf("summary = %q", summary)
	}
	idx := strings.Index(summary, " to ")
	if idx < 0 {
		t.Fatalf("summary = %q", summary)
	}
	line := summary[idx+4:]
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	return line
}
