package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestUnconfiguredToolsReturnErrors(t *testing.T) {
	p := newPlugin(options{})
	_, err := p.Tools[0].Execute(context.Background(), nil, plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestLintHardFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ansible-lint")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'boom\\n'\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	tool := lintTool()
	_, err := tool.Execute(context.Background(), []byte(`{"filePath":"play.yml"}`), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "ansible-lint error") {
		t.Fatalf("expected lint error, got %v", err)
	}
}

func TestLintViolationsStaySuccessful(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ansible-lint")
	body := "#!/bin/sh\nprintf '%s\\n' '[{\"check_name\":\"yaml\",\"severity\":\"high\",\"description\":\"bad\",\"location\":{\"path\":\"p.yml\",\"lines\":{\"begin\":3}}}]'\nexit 2\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, err := lintTool().Execute(context.Background(), []byte(`{"filePath":"play.yml"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "yaml") || !strings.Contains(got, "Violations found") {
		t.Fatalf("violations = %q", got)
	}
}
