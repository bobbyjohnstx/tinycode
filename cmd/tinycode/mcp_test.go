package main

import (
	"reflect"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestNormalizeMCPTransport(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"stdio", "stdio"},
		{"SSE", "sse"},
		{"http", "streamable-http"},
		{"streamable-http", "streamable-http"},
		{"weird", "weird"},
	}
	for _, tt := range tests {
		if got := normalizeMCPTransport(tt.in); got != tt.want {
			t.Errorf("normalizeMCPTransport(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseMCPStdioArgs(t *testing.T) {
	cmd, args, err := parseMCPStdioArgs([]string{"--", "npx", "-y", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "npx" || !reflect.DeepEqual(args, []string{"-y", "pkg"}) {
		t.Fatalf("got %q %#v", cmd, args)
	}

	cmd, args, err = parseMCPStdioArgs([]string{"npx", "-y", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "npx" || !reflect.DeepEqual(args, []string{"-y", "pkg"}) {
		t.Fatalf("got %q %#v", cmd, args)
	}

	if _, _, err := parseMCPStdioArgs(nil); err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestEnvFlag_Set(t *testing.T) {
	var e envFlag
	if err := e.Set("FOO=bar"); err != nil {
		t.Fatal(err)
	}
	if err := e.Set("BAZ"); err != nil {
		t.Fatal(err)
	}
	if e["FOO"] != "bar" {
		t.Errorf("FOO = %q", e["FOO"])
	}
	if e["BAZ"] != "{env:BAZ}" {
		t.Errorf("BAZ = %q", e["BAZ"])
	}
}

func TestHeaderFlag_Set(t *testing.T) {
	var h headerFlag
	if err := h.Set("Authorization: Bearer tok"); err != nil {
		t.Fatal(err)
	}
	if h["Authorization"] != "Bearer tok" {
		t.Errorf("got %q", h["Authorization"])
	}
	if err := h.Set("bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseMCPAuthArgs_FlagsAfterName(t *testing.T) {
	project, token, envVar, name, err := parseMCPAuthArgs([]string{"github", "--env", "GH_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if project || token != "" || envVar != "GH_TOKEN" || name != "github" {
		t.Fatalf("got project=%v token=%q env=%q name=%q", project, token, envVar, name)
	}

	_, token, _, name, err = parseMCPAuthArgs([]string{"--token", "abc", "svc"})
	if err != nil {
		t.Fatal(err)
	}
	if token != "abc" || name != "svc" {
		t.Fatalf("got token=%q name=%q", token, name)
	}

	if _, _, _, _, err := parseMCPAuthArgs([]string{"svc"}); err == nil {
		t.Fatal("expected error without --token/--env")
	}
}

func TestParseMCPAddArgs(t *testing.T) {
	opts, pos, err := parseMCPAddArgs([]string{
		"--project", "--transport", "stdio", "-e", "FOO=bar", "--header", "Authorization: Bearer x", "myserver", "--", "npx", "-y", "pkg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.project || opts.transport != "stdio" {
		t.Fatalf("opts = %+v", opts)
	}
	if opts.envs["FOO"] != "bar" {
		t.Fatalf("env FOO = %q", opts.envs["FOO"])
	}
	if opts.headers["Authorization"] != "Bearer x" {
		t.Fatalf("header = %q", opts.headers["Authorization"])
	}
	wantPos := []string{"myserver", "--", "npx", "-y", "pkg"}
	if !reflect.DeepEqual(pos, wantPos) {
		t.Fatalf("positionals = %#v, want %#v", pos, wantPos)
	}

	if _, _, err := parseMCPAddArgs([]string{"--transport"}); err == nil {
		t.Fatal("expected error for --transport without value")
	}
	if _, _, err := parseMCPAddArgs([]string{"--unknown"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if _, _, err := parseMCPAddArgs([]string{"--help"}); err == nil || err.Error() != "help" {
		t.Fatalf("expected help error, got %v", err)
	}
}

func TestParseMCPLogoutArgs(t *testing.T) {
	project, name, err := parseMCPLogoutArgs([]string{"--project", "github"})
	if err != nil {
		t.Fatal(err)
	}
	if !project || name != "github" {
		t.Fatalf("project=%v name=%q", project, name)
	}
	if _, _, err := parseMCPLogoutArgs([]string{}); err == nil {
		t.Fatal("expected error without name")
	}
	if _, _, err := parseMCPLogoutArgs([]string{"a", "b"}); err == nil {
		t.Fatal("expected error for two names")
	}
	if _, _, err := parseMCPLogoutArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestMCPTransportAndEndpointLabels(t *testing.T) {
	if got := mcpTransportLabel(config.MCPConfig{Transport: "http"}); got != "http" {
		t.Errorf("explicit transport = %q", got)
	}
	if got := mcpTransportLabel(config.MCPConfig{Command: "npx"}); got != "stdio" {
		t.Errorf("command implies stdio, got %q", got)
	}
	if got := mcpTransportLabel(config.MCPConfig{URL: "https://x"}); got != "sse" {
		t.Errorf("url implies sse, got %q", got)
	}
	if got := mcpTransportLabel(config.MCPConfig{}); got != "stdio" {
		t.Errorf("empty default = %q", got)
	}

	if got := mcpEndpointLabel(config.MCPConfig{URL: "https://x"}); got != "https://x" {
		t.Errorf("url endpoint = %q", got)
	}
	if got := mcpEndpointLabel(config.MCPConfig{}); got != "-" {
		t.Errorf("empty endpoint = %q", got)
	}
	if got := mcpEndpointLabel(config.MCPConfig{Command: "npx", Args: []string{"-y", "pkg"}}); got != "npx -y pkg" {
		t.Errorf("stdio endpoint = %q", got)
	}
}
