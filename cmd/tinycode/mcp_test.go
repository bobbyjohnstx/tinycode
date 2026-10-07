package main

import (
	"reflect"
	"testing"
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
