package formatter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func enabledConfig(items map[string]config.FormatterItem) *config.FormatterConfig {
	on := true
	return &config.FormatterConfig{Enabled: &on, Items: items}
}

func TestResolve_DisabledByDefault(t *testing.T) {
	if got := Resolve(nil).Status(); len(got) != 0 {
		t.Fatalf("status = %#v, want empty", got)
	}
	off := false
	if got := Resolve(&config.FormatterConfig{Enabled: &off}).Status(); len(got) != 0 {
		t.Fatalf("status = %#v, want empty", got)
	}
}

func TestResolve_TrueEnablesGofmt(t *testing.T) {
	status := Resolve(enabledConfig(nil)).Status()
	if len(status) != 1 {
		t.Fatalf("status = %#v", status)
	}
	if status[0].Name != "gofmt" || !status[0].Enabled || status[0].Extensions[0] != ".go" {
		t.Fatalf("status = %#v", status)
	}
}

func TestFormat_GofmtRewritesGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc  f( ){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, changed, err := Resolve(enabledConfig(nil)).Format(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if name != "gofmt" || !changed {
		t.Fatalf("name=%q changed=%v", name, changed)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "func f()") {
		t.Fatalf("formatted source = %q", got)
	}
}

func TestFormat_GofmtLeavesInvalidGo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	raw := []byte("package main\nfunc (\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, err := Resolve(enabledConfig(nil)).Format(context.Background(), path)
	if err == nil {
		t.Fatal("expected gofmt to reject invalid Go")
	}
	if changed {
		t.Fatal("invalid Go should stay unchanged")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("file changed to %q", got)
	}
}

func TestFormat_DisabledGofmtSkipsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	raw := []byte("package main\nfunc  f( ){}\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	disabled := true
	r := Resolve(enabledConfig(map[string]config.FormatterItem{
		"gofmt": {Disabled: &disabled},
	}))
	status := r.Status()
	if len(status) != 1 || status[0].Name != "gofmt" || status[0].Enabled || status[0].Extensions[0] != ".go" {
		t.Fatalf("status = %#v, want disabled gofmt for .go", status)
	}
	name, changed, err := r.Format(context.Background(), path)
	if err != nil || changed || name != "" {
		t.Fatalf("name=%q changed=%v err=%v", name, changed, err)
	}
}

func TestFormat_CustomCommandDropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Resolve(enabledConfig(map[string]config.FormatterItem{
		"stamp": {
			Command:     []string{"sh", "-c", `printf '%s|%s' "$OPENROUTER_API_KEY" "$STAMP_TOKEN" > "$1"`, "sh"},
			Extensions:  []string{".txt"},
			Environment: map[string]string{"STAMP_TOKEN": "from-config"},
		},
	}))
	if _, _, err := r.Format(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "|from-config" {
		t.Fatalf("file = %q, want the configured token and not the parent API key", got)
	}
}

func TestFormat_CustomCommandRewritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Resolve(enabledConfig(map[string]config.FormatterItem{
		"stamp": {
			Command:    []string{"sh", "-c", "printf 'after\\n' > \"$1\"", "sh"},
			Extensions: []string{".txt"},
		},
	}))
	name, changed, err := r.Format(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if name != "stamp" || !changed {
		t.Fatalf("name=%q changed=%v", name, changed)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("file = %q", got)
	}
}
