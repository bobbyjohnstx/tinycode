package main

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogServeAuthToken_LogsTruncatedTokenOnly(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	token := "super-secret-token-value"
	logServeAuthToken(token, "http://127.0.0.1:4096/")

	out := buf.String()
	if strings.Contains(out, token) {
		t.Fatalf("log contains full token: %s", out)
	}
	authParam := base64.StdEncoding.EncodeToString([]byte("tinycode:" + token))
	if strings.Contains(out, authParam) || strings.Contains(out, "auth_token") {
		t.Fatalf("log contains auth URL: %s", out)
	}
	if !strings.Contains(out, "super-se...") {
		t.Fatalf("log missing truncated token: %s", out)
	}
	if !strings.Contains(out, "http://127.0.0.1:4096") {
		t.Fatalf("log missing base URL: %s", out)
	}
}

func TestLogServeAuthToken_EmptyToken(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	logServeAuthToken("", "http://127.0.0.1:4096")
	if buf.Len() != 0 {
		t.Fatalf("empty token should not log, got %s", buf.String())
	}
}

func TestPrintOpsConsoleURL_WritesFullURLToWriter(t *testing.T) {
	token := "super-secret-token-value"
	var buf bytes.Buffer
	printOpsConsoleURL(&buf, token, "http://127.0.0.1:4096/")

	authParam := base64.StdEncoding.EncodeToString([]byte("tinycode:" + token))
	want := "ops console: http://127.0.0.1:4096/?auth_token=" + authParam + "\n"
	if buf.String() != want {
		t.Fatalf("printed %q, want %q", buf.String(), want)
	}
}

func TestPrintOpsConsoleURL_EmptyToken(t *testing.T) {
	var buf bytes.Buffer
	printOpsConsoleURL(&buf, "", "http://127.0.0.1:4096")
	if buf.Len() != 0 {
		t.Fatalf("empty token should not print, got %q", buf.String())
	}
}

func TestResolveServeAuthToken(t *testing.T) {
	t.Run("NO_AUTH on localhost", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "1")
		t.Setenv("TINYCODE_FORCE_NO_AUTH", "")
		t.Setenv("TINYCODE_AUTH_TOKEN", "")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "")
		if got := resolveServeAuthToken("127.0.0.1"); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("AUTH_TOKEN preferred", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "")
		t.Setenv("TINYCODE_AUTH_TOKEN", "from-auth")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "legacy")
		if got := resolveServeAuthToken("127.0.0.1"); got != "from-auth" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("legacy password", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "")
		t.Setenv("TINYCODE_AUTH_TOKEN", "")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "legacy-pass")
		if got := resolveServeAuthToken("127.0.0.1"); got != "legacy-pass" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("generates when unset", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "")
		t.Setenv("TINYCODE_AUTH_TOKEN", "")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "")
		got := resolveServeAuthToken("127.0.0.1")
		if len(got) != 64 {
			t.Fatalf("expected generated 64-char token, got %q", got)
		}
	})
}

func TestResolveWebAuthToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	t.Run("NO_AUTH on localhost", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "1")
		t.Setenv("TINYCODE_AUTH_TOKEN", "")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "")
		if got := resolveWebAuthToken("localhost"); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("AUTH_TOKEN", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "")
		t.Setenv("TINYCODE_AUTH_TOKEN", "web-token")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "")
		if got := resolveWebAuthToken("127.0.0.1"); got != "web-token" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("persistent file fallback", func(t *testing.T) {
		t.Setenv("TINYCODE_NO_AUTH", "")
		t.Setenv("TINYCODE_AUTH_TOKEN", "")
		t.Setenv("TINYCODE_SERVER_PASSWORD", "")
		known := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		if err := os.WriteFile(filepath.Join(dir, "web_token"), []byte(known+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := resolveWebAuthToken("127.0.0.1"); got != known {
			t.Fatalf("got %q, want file token", got)
		}
	})
}
