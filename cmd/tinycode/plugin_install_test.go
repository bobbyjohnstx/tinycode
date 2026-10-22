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
	if !strings.Contains(out, "tinycode plugin enable safety-net") {
		t.Fatalf("missing enable command hint: %q", out)
	}
	if !strings.Contains(out, config.GlobalConfigFile()) {
		t.Fatalf("hint should mention config file path, got %q", out)
	}
}

func TestPluginEnable_WritesGlobalConfig(t *testing.T) {
	t.Setenv("TINYCODE_CONFIG_DIR", t.TempDir())
	if err := enablePluginCLI("safety-net"); err != nil {
		t.Fatal(err)
	}
	path := config.GlobalConfigFile()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"safety-net"`) {
		t.Fatalf("config missing plugin: %s", raw)
	}
	// Idempotent
	if err := enablePluginCLI("safety-net"); err != nil {
		t.Fatal(err)
	}
}

func TestParsePluginInstallFlags(t *testing.T) {
	from, en := parsePluginInstallFlags([]string{"--enable"})
	if from != "" || !en {
		t.Fatalf("got from=%q enable=%v", from, en)
	}
	from, en = parsePluginInstallFlags([]string{"--from", "/tmp/bin", "--enable"})
	if from != "/tmp/bin" || !en {
		t.Fatalf("got from=%q enable=%v", from, en)
	}
}
