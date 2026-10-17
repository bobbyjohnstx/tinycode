package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCopiedToClipboardMsg_FieldsPopulated(t *testing.T) {
	msg := CopiedToClipboardMsg{Chars: 42}
	if msg.Chars != 42 {
		t.Errorf("expected Chars=42, got %d", msg.Chars)
	}
	if msg.Err != nil {
		t.Errorf("expected nil Err, got %v", msg.Err)
	}
}

func TestCopiedToClipboardMsg_WithError(t *testing.T) {
	msg := CopiedToClipboardMsg{Err: errClipTest}
	if msg.Err == nil {
		t.Fatal("expected non-nil Err")
	}
	if msg.Chars != 0 {
		t.Errorf("expected Chars=0 on error, got %d", msg.Chars)
	}
}

func TestClipboardImageMsg_FieldsPopulated(t *testing.T) {
	msg := ClipboardImageMsg{
		Data:      "aGVsbG8=",
		MediaType: "image/png",
		Size:      5,
	}
	if msg.Data != "aGVsbG8=" {
		t.Errorf("expected Data=aGVsbG8=, got %q", msg.Data)
	}
	if msg.MediaType != "image/png" {
		t.Errorf("expected MediaType=image/png, got %q", msg.MediaType)
	}
	if msg.Size != 5 {
		t.Errorf("expected Size=5, got %d", msg.Size)
	}
}

func TestClipboardImageMsg_WithError(t *testing.T) {
	msg := ClipboardImageMsg{Err: errClipTest}
	if msg.Err == nil {
		t.Fatal("expected non-nil Err")
	}
	if msg.Data != "" {
		t.Errorf("expected empty Data on error, got %q", msg.Data)
	}
}

func TestCopyToClipboard_ReturnsCmd(t *testing.T) {
	cmd := copyToClipboard("hello world")
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd")
	}
	// Execute the command — on CI/headless it will fail, which is fine;
	// we verify the message type is correct either way.
	msg := cmd()
	result, ok := msg.(CopiedToClipboardMsg)
	if !ok {
		t.Fatalf("expected CopiedToClipboardMsg, got %T", msg)
	}
	// On macOS with pbcopy available, this succeeds.
	// On headless Linux, this fails but returns an error in the msg.
	if result.Err == nil {
		if result.Chars != 11 {
			t.Errorf("expected 11 chars for 'hello world', got %d", result.Chars)
		}
	}
}

func TestCopyToClipboard_CountsRunes(t *testing.T) {
	// "café" has 4 runes but 5 bytes
	cmd := copyToClipboard("café")
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd")
	}
	msg := cmd()
	result, ok := msg.(CopiedToClipboardMsg)
	if !ok {
		t.Fatalf("expected CopiedToClipboardMsg, got %T", msg)
	}
	if result.Err == nil {
		if result.Chars != 4 {
			t.Errorf("expected 4 runes for 'café', got %d", result.Chars)
		}
	}
}

func TestReadClipboardImage_ReturnsCmd(t *testing.T) {
	cmd := readClipboardImage()
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd")
	}
	msg := cmd()
	result, ok := msg.(ClipboardImageMsg)
	if !ok {
		t.Fatalf("expected ClipboardImageMsg, got %T", msg)
	}
	// On headless environments, reading an image will fail.
	// We just verify the msg type is correct.
	_ = result
}

func TestGetClipboardImage_CurrentPlatform(t *testing.T) {
	// On darwin, getClipboardImage calls getClipboardImageDarwin.
	// On linux, getClipboardImageLinux. Both may fail on CI
	// (no image in clipboard). We just verify no panic.
	_, err := getClipboardImage()
	switch runtime.GOOS {
	case "darwin", "linux":
		// Error is expected (no image in clipboard), but should not panic.
		if err == nil {
			t.Log("clipboard image read succeeded (unexpected but OK)")
		}
	default:
		if err == nil {
			t.Errorf("expected unsupported platform error on %s", runtime.GOOS)
		}
	}
}

func TestWriteClipboard_CurrentPlatform(t *testing.T) {
	err := writeClipboard("test-clipboard-write")
	switch runtime.GOOS {
	case "darwin":
		// pbcopy should be available on macOS.
		if err != nil {
			t.Logf("writeClipboard failed on darwin (headless?): %v", err)
		}
	case "linux":
		// May fail if no clipboard tool is installed.
		_ = err
	default:
		if err == nil {
			t.Errorf("expected unsupported platform error on %s", runtime.GOOS)
		}
	}
}

func stubClipboardBins(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestLinuxClipboardWriteTools_WaylandPrefersWlCopy(t *testing.T) {
	stubClipboardBins(t, "wl-copy", "xclip", "xsel")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	got := linuxClipboardWriteTools()
	if len(got) == 0 || got[0] != "wl-copy" {
		t.Fatalf("expected wl-copy first under Wayland, got %v", got)
	}
	want := []string{"wl-copy", "xclip", "xsel"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLinuxClipboardWriteTools_X11PrefersXclip(t *testing.T) {
	stubClipboardBins(t, "wl-copy", "xclip", "xsel")
	t.Setenv("WAYLAND_DISPLAY", "")

	got := linuxClipboardWriteTools()
	if len(got) == 0 || got[0] != "xclip" {
		t.Fatalf("expected xclip first without Wayland, got %v", got)
	}
	want := []string{"xclip", "xsel", "wl-copy"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestWriteClipboard_LinuxNoTool_WaylandError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only error path")
	}
	stubClipboardBins(t) // empty PATH
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	err := writeClipboard("hi")
	if err == nil {
		t.Fatal("expected error when no clipboard tool")
	}
	if !strings.Contains(err.Error(), "wl-clipboard") {
		t.Errorf("expected Wayland-aware error, got %v", err)
	}
}

// sentinel error for test construction
var errClipTest = func() error {
	return &clipTestErr{}
}()

type clipTestErr struct{}

func (e *clipTestErr) Error() string { return "test clipboard error" }
