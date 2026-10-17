package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOllamaURL(t *testing.T) {
	const defaultURL = "http://127.0.0.1:11434"

	tests := []struct {
		name     string
		tinycode string
		ollama   string
		want     string
	}{
		{name: "default when unset", want: defaultURL},
		{name: "OLLAMA_HOST fallback", ollama: "http://ollama:11434", want: "http://ollama:11434"},
		{name: "TINYCODE_OLLAMA_HOST preferred", tinycode: "http://tiny:11434", want: "http://tiny:11434"},
		{
			name:     "both set prefers TINYCODE_OLLAMA_HOST",
			tinycode: "http://tiny:11434",
			ollama:   "http://ollama:11434",
			want:     "http://tiny:11434",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TINYCODE_OLLAMA_HOST", tt.tinycode)
			t.Setenv("OLLAMA_HOST", tt.ollama)
			if got := ollamaURL(); got != tt.want {
				t.Errorf("ollamaURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderPathUsable_Scenario(t *testing.T) {
	tests := []struct {
		name               string
		anyLocalConnected  bool
		hasCloudAPIKey     bool
		anyConfigConnected bool
		want               bool
	}{
		{
			name:              "cloud only with openrouter key",
			hasCloudAPIKey:    true,
			anyLocalConnected: false,
			want:              true,
		},
		{
			name:              "local only",
			anyLocalConnected: true,
			want:              true,
		},
		{
			name:               "config provider only",
			anyConfigConnected: true,
			want:               true,
		},
		{
			name:              "no local and no cloud key",
			anyLocalConnected: false,
			hasCloudAPIKey:    false,
			want:              false,
		},
		{
			name:               "all paths available",
			anyLocalConnected:  true,
			hasCloudAPIKey:     true,
			anyConfigConnected: true,
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerPathUsable(tt.anyLocalConnected, tt.hasCloudAPIKey, tt.anyConfigConnected)
			if got != tt.want {
				t.Errorf("providerPathUsable(%v, %v, %v) = %v, want %v",
					tt.anyLocalConnected, tt.hasCloudAPIKey, tt.anyConfigConnected, got, tt.want)
			}
		})
	}
}

func TestPrintProviderNextSteps_MentionsConnect(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printProviderNextSteps()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "/connect") {
		t.Errorf("expected /connect in next steps, got %q", out)
	}
	if !strings.Contains(out, "tinycode") {
		t.Errorf("expected tinycode launch hint in next steps, got %q", out)
	}
}

func stubPATHBinaries(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestCheckIDEs_DetectsStubsOnPATH(t *testing.T) {
	stubPATHBinaries(t, "code", "cursor", "nvim", "idea")
	cap := captureStdout(t)
	checkIDEs()
	out := cap.read()

	for _, want := range []string{
		"VS Code detected",
		"Cursor detected",
		"Neovim detected",
		"JetBrains detected",
		"docs/acp-integration.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
	if strings.Contains(out, "No IDE detected") {
		t.Errorf("did not expect standalone hint when IDEs are on PATH, got %q", out)
	}
	if strings.Contains(out, "install:") {
		t.Errorf("IDE check must not print install actions, got %q", out)
	}
}

func TestCheckIDEs_NoIDEOnPATH(t *testing.T) {
	stubPATHBinaries(t) // empty PATH dir — no IDE binaries
	cap := captureStdout(t)
	checkIDEs()
	out := cap.read()

	if !strings.Contains(out, "No IDE detected") {
		t.Errorf("expected standalone hint when no IDEs on PATH, got %q", out)
	}
	if !strings.Contains(out, "standalone") {
		t.Errorf("expected standalone wording, got %q", out)
	}
	for _, name := range []string{"VS Code detected", "Cursor detected", "Neovim detected", "JetBrains detected"} {
		if strings.Contains(out, name) {
			t.Errorf("did not expect %q when PATH has no IDEs, got %q", name, out)
		}
	}
}

func TestCheckIDEs_PartialDetection(t *testing.T) {
	stubPATHBinaries(t, "nvim")
	cap := captureStdout(t)
	checkIDEs()
	out := cap.read()

	if !strings.Contains(out, "Neovim detected") {
		t.Errorf("expected Neovim detected, got %q", out)
	}
	if !strings.Contains(out, "docs/acp-integration.md") {
		t.Errorf("expected ACP docs pointer, got %q", out)
	}
	if strings.Contains(out, "VS Code detected") {
		t.Errorf("did not expect VS Code when only nvim stubbed, got %q", out)
	}
	if strings.Contains(out, "No IDE detected") {
		t.Errorf("did not expect standalone hint when nvim is present, got %q", out)
	}
}

func TestCheckClipboard_LinuxMissingToolsWarns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("clipboard doctor check is linux-only")
	}
	stubPATHBinaries(t) // empty PATH — no clipboard helpers
	cap := captureStdout(t)
	checkClipboard()
	out := cap.read()

	if !strings.Contains(out, checkWarn) {
		t.Errorf("expected warning marker, got %q", out)
	}
	if !strings.Contains(out, "Clipboard:") {
		t.Errorf("expected Clipboard line, got %q", out)
	}
	if !strings.Contains(out, "error toast") {
		t.Errorf("expected error toast mention, got %q", out)
	}
	if strings.Contains(out, checkFail) {
		t.Errorf("clipboard-only missing must not fail doctor, got %q", out)
	}
	if strings.Contains(out, checkPass) {
		t.Errorf("did not expect pass when no helpers, got %q", out)
	}
}

func TestCheckClipboard_LinuxFindsWlCopy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("clipboard doctor check is linux-only")
	}
	stubPATHBinaries(t, "wl-copy")
	cap := captureStdout(t)
	checkClipboard()
	out := cap.read()

	if !strings.Contains(out, checkPass) {
		t.Errorf("expected pass when wl-copy present, got %q", out)
	}
	if !strings.Contains(out, "wl-copy found") {
		t.Errorf("expected wl-copy found, got %q", out)
	}
	if strings.Contains(out, checkWarn) {
		t.Errorf("did not expect warn when helper present, got %q", out)
	}
}

func TestCheckClipboard_NonLinuxSkips(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux skip path")
	}
	cap := captureStdout(t)
	checkClipboard()
	out := cap.read()
	if out != "" {
		t.Errorf("expected no clipboard output on %s, got %q", runtime.GOOS, out)
	}
}
