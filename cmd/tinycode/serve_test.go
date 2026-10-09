package main

import (
	"bytes"
	"encoding/base64"
	"log/slog"
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
