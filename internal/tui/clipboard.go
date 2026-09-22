package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// CopiedToClipboardMsg is emitted after text is copied to the clipboard.
type CopiedToClipboardMsg struct {
	Chars int
	Err   error
}

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		err := writeClipboard(text)
		if err != nil {
			return CopiedToClipboardMsg{Err: err}
		}
		return CopiedToClipboardMsg{Chars: len([]rune(text))}
	}
}

// ClipboardImageMsg is emitted after an image is read from the clipboard.
type ClipboardImageMsg struct {
	Data      string // base64-encoded image data
	MediaType string // e.g. "image/png"
	Size      int    // raw byte count
	Err       error
}

// readClipboardImage reads image data from the system clipboard.
func readClipboardImage() tea.Cmd {
	return func() tea.Msg {
		data, err := getClipboardImage()
		if err != nil {
			return ClipboardImageMsg{Err: err}
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		return ClipboardImageMsg{
			Data:      encoded,
			MediaType: "image/png",
			Size:      len(data),
		}
	}
}

// getClipboardImage retrieves PNG image data from the system clipboard.
func getClipboardImage() ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		return getClipboardImageDarwin()
	case "linux":
		return getClipboardImageLinux()
	default:
		return nil, fmt.Errorf("clipboard image not supported on %s", runtime.GOOS)
	}
}

func getClipboardImageDarwin() ([]byte, error) {
	// Check if clipboard contains image data.
	infoCmd := exec.Command("osascript", "-e", `clipboard info`)
	out, err := infoCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("checking clipboard: %w", err)
	}
	if !strings.Contains(string(out), "PNGf") {
		return nil, fmt.Errorf("no image in clipboard")
	}

	// Write clipboard image to temp file via AppleScript.
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, "tinycode-clipboard.png")
	script := fmt.Sprintf(`
set theImage to the clipboard as «class PNGf»
set filePath to POSIX file %q
set fileRef to open for access filePath with write permission
set eof of fileRef to 0
write theImage to fileRef
close access fileRef
`, tmpPath)
	writeCmd := exec.Command("osascript", "-e", script)
	if err := writeCmd.Run(); err != nil {
		return nil, fmt.Errorf("reading clipboard image: %w", err)
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading temp image: %w", err)
	}
	return data, nil
}

func getClipboardImageLinux() ([]byte, error) {
	// Try xclip first, then xsel.
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o")
		data, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("no image in clipboard")
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("no image in clipboard")
		}
		return data, nil
	}
	if _, err := exec.LookPath("wl-paste"); err == nil {
		cmd := exec.Command("wl-paste", "--type", "image/png")
		data, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("no image in clipboard")
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("no image in clipboard")
		}
		return data, nil
	}
	return nil, fmt.Errorf("no clipboard tool found (install xclip or wl-paste)")
}

func writeClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-copy)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
