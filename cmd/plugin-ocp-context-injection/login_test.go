package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOcTarget(t *testing.T) {
	if err := validateOcTarget("https://api.example.com:6443", "sha256~abc"); err != nil {
		t.Fatal(err)
	}
	for _, server := range []string{"http://api.example.com", "https://api.example.com\n--token", "-https://x", "not a url"} {
		if err := validateOcTarget(server, "token"); err == nil {
			t.Fatalf("expected reject for server %q", server)
		}
	}
	if err := validateOcTarget("https://api.example.com", "tok\nen"); err == nil {
		t.Fatal("expected newline token to be rejected")
	}
}

func TestWriteKubeconfigRejectsNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	err := writeKubeconfig(path, "https://api.example.com\ninjected: true", "token", false)
	if err == nil {
		t.Fatal("expected newline in server to be rejected")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("kubeconfig was written after a rejected server")
	}
}

func TestOcLogin_MergeIncludesStderr(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	body := `#!/bin/sh
if [ "$1" = whoami ]; then
  printf 'alice\n'
  exit 0
fi
if [ "$1" = config ]; then
  printf 'merge refused\n' >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	dest := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(dest, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", dest)

	_, err := ocLogin(context.Background(), "https://api.example.com:6443", "SUPERSECRET", false)
	if err == nil || !strings.Contains(err.Error(), "merge refused") {
		t.Fatalf("expected stderr in merge error, got %v", err)
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestOcLogin_TokenNotInArgv(t *testing.T) {
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	script := filepath.Join(dir, "oc")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$ARGV_LOG"
if [ "$1" = whoami ]; then
  if [ "$OC_FAIL" = 1 ]; then
    printf 'rejected token SUPERSECRET\n'
    exit 1
  fi
  printf 'alice\n'
  exit 0
fi
if [ "$1" = config ]; then
  printf 'flattened\n'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ARGV_LOG", argvLog)
	dest := filepath.Join(dir, "kubeconfig")
	t.Setenv("KUBECONFIG", dest)

	const token = "SUPERSECRET"
	got, err := ocLogin(context.Background(), "https://api.example.com:6443", token, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, token) {
		t.Fatalf("result leaked token: %q", got)
	}
	if !strings.Contains(got, "alice") {
		t.Fatalf("result = %q", got)
	}
	logged, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), token) {
		t.Fatalf("argv leaked token: %s", logged)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), token) {
		t.Fatal("kubeconfig missing token")
	}
	if !strings.Contains(string(data), "insecure-skip-tls-verify: true") {
		t.Fatalf("kubeconfig missing insecure flag:\n%s", data)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("kubeconfig mode = %o", info.Mode().Perm())
	}

	t.Setenv("OC_FAIL", "1")
	os.Remove(dest)
	_, err = ocLogin(context.Background(), "https://api.example.com:6443", token, false)
	if err == nil {
		t.Fatal("expected login failure")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
}
