package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMCPConfigPath_GlobalPrefersExistingThenJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_CONFIG_DIR", dir)

	got := ResolveMCPConfigPath(false, "")
	want := filepath.Join(dir, "tinycode.json")
	if got != want {
		t.Fatalf("empty dir: got %q, want %q", got, want)
	}

	jsonc := filepath.Join(dir, "tinycode.jsonc")
	if err := os.WriteFile(jsonc, []byte(`{"model":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got = ResolveMCPConfigPath(false, "")
	if got != jsonc {
		t.Fatalf("existing jsonc: got %q, want %q", got, jsonc)
	}
}

func TestResolveMCPConfigPath_ProjectPrefersInnermost(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(root, "tinycode.json")
	inner := filepath.Join(nested, ".tinycode", "tinycode.json")
	if err := os.MkdirAll(filepath.Dir(inner), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outer, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inner, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveMCPConfigPath(true, nested)
	if got != inner {
		t.Fatalf("got %q, want %q", got, inner)
	}
}

func TestResolveMCPConfigPath_ProjectDefaultDotTinycode(t *testing.T) {
	dir := t.TempDir()
	got := ResolveMCPConfigPath(true, dir)
	want := filepath.Join(dir, ".tinycode", "tinycode.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteMCPServer_CreatesAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_CONFIG_DIR", dir)
	t.Setenv("API_KEY", "secret-from-env")
	path := ResolveMCPConfigPath(false, "")

	cfg := MCPConfig{
		Command: "npx",
		Args:    []string{"-y", "@my/mcp-server"},
		Env:     map[string]string{"API_KEY": "{env:API_KEY}"},
	}
	if err := WriteMCPServer(path, "myserver", cfg); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"{env:API_KEY}"`)) {
		t.Fatalf("written file missing env placeholder: %s", raw)
	}

	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := loaded.MCP["myserver"]
	if !ok {
		t.Fatal("myserver missing after Load")
	}
	if got.Command != "npx" {
		t.Errorf("Command = %q, want npx", got.Command)
	}
	if len(got.Args) != 2 || got.Args[0] != "-y" {
		t.Errorf("Args = %#v", got.Args)
	}
	if got.Env["API_KEY"] != "secret-from-env" {
		t.Errorf("Env after Load = %#v", got.Env)
	}

	if err := WriteMCPServer(path, "remote", MCPConfig{
		URL:       "https://example.com/mcp",
		Transport: "streamable-http",
		Headers:   map[string]string{"Authorization": "Bearer tok"},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCP["myserver"]; !ok {
		t.Error("myserver lost after second write")
	}
	remote := loaded.MCP["remote"]
	if remote.URL != "https://example.com/mcp" || remote.Transport != "streamable-http" {
		t.Errorf("remote = %#v", remote)
	}
	if remote.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("headers = %#v", remote.Headers)
	}
}

func TestWriteMCPServer_PreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinycode.json")
	if err := os.WriteFile(path, []byte(`{"model":"ollama/qwen","share":"disabled"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteMCPServer(path, "fs", MCPConfig{
		Command: "npx",
		Args:    []string{"-y", "server"},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	if root["model"] != "ollama/qwen" {
		t.Errorf("model lost: %v", root["model"])
	}
}

func TestUpdateMCPServerHeaders_AuthAndLogout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinycode.json")
	if err := WriteMCPServer(path, "remote", MCPConfig{
		URL:       "https://example.com/mcp",
		Transport: "sse",
	}); err != nil {
		t.Fatal(err)
	}

	if err := UpdateMCPServerHeaders(path, "remote", nil, "Bearer {env:TOKEN}", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"Authorization": "Bearer {env:TOKEN}"`)) {
		t.Fatalf("auth placeholder missing in file: %s", raw)
	}

	t.Setenv("TOKEN", "live-token")
	info, err := LoadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.MCP["remote"].Headers["Authorization"]; got != "Bearer live-token" {
		t.Fatalf("auth after LoadFile = %q", got)
	}

	if err := UpdateMCPServerHeaders(path, "remote", nil, "", true); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`Authorization`)) {
		t.Fatalf("Authorization still present in file: %s", raw)
	}
}

func TestUpdateMCPServerHeaders_MissingServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tinycode.json")
	err := UpdateMCPServerHeaders(path, "missing", nil, "Bearer x", false)
	if err == nil {
		t.Fatal("expected error for missing server")
	}
}
