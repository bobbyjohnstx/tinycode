package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
)

func TestPluginInstall_ReleaseTagDevRequiresOverride(t *testing.T) {
	_, err := plugin.PluginReleaseTag("dev", "")
	if err == nil {
		t.Fatal("expected error for dev without override")
	}
	if !strings.Contains(err.Error(), "TINYCODE_PLUGIN_RELEASE") {
		t.Fatalf("expected actionable override hint, got %v", err)
	}
}

func TestPluginInstall_EnableHintPrinted(t *testing.T) {
	t.Setenv("TINYCODE_CONFIG_DIR", t.TempDir())

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printPluginEnableHint("safety-net")
	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if !strings.Contains(out, `Enable by adding "safety-net" to "plugins"`) {
		t.Fatalf("missing enable hint: %q", out)
	}
	if !strings.Contains(out, config.GlobalConfigFile()) {
		t.Fatalf("hint should mention config file path, got %q", out)
	}
}
