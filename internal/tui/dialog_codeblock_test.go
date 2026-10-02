package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCodeBlockDialogShowHide(t *testing.T) {
	d := NewCodeBlockDialog()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden initially")
	}

	blocks := []CodeBlock{{Language: "go", Code: "fmt.Println()", Preview: "fmt.Println()"}}
	d.Show(blocks, "full text", "/tmp")
	if !d.IsVisible() {
		t.Fatal("expected dialog to be visible after Show")
	}

	d.Hide()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden after Hide")
	}
}

func TestCodeBlockDialogNavigation(t *testing.T) {
	d := NewCodeBlockDialog()
	blocks := []CodeBlock{
		{Language: "go", Code: "go code", Preview: "go code"},
		{Language: "py", Code: "py code", Preview: "py code"},
	}
	d.Show(blocks, "full text", "/tmp")

	// Initial selection is 0 (Full response).
	if d.selected != 0 {
		t.Errorf("initial selected = %d, want 0", d.selected)
	}

	// Move down.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 1 {
		t.Errorf("after j, selected = %d, want 1", d.selected)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 2 {
		t.Errorf("after jj, selected = %d, want 2", d.selected)
	}

	// At bottom, can't go further.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 2 {
		t.Errorf("at bottom, selected = %d, want 2", d.selected)
	}

	// Move up.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.selected != 1 {
		t.Errorf("after k, selected = %d, want 1", d.selected)
	}
}

func TestCodeBlockDialogEsc(t *testing.T) {
	d := NewCodeBlockDialog()
	d.Show(nil, "text", "/tmp")

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if d.IsVisible() {
		t.Error("expected dialog to close on esc")
	}
}

func TestCodeBlockDialogEnterCopies(t *testing.T) {
	d := NewCodeBlockDialog()
	blocks := []CodeBlock{{Language: "go", Code: "go code", Preview: "go code"}}
	d.Show(blocks, "full text", "/tmp")

	// Select "Full response" (index 0) and press enter.
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.IsVisible() {
		t.Error("expected dialog to close on enter")
	}
	if cmd == nil {
		t.Error("expected clipboard command on enter")
	}
}

func TestCodeBlockDialogEnterCopiesBlock(t *testing.T) {
	d := NewCodeBlockDialog()
	blocks := []CodeBlock{{Language: "go", Code: "go code", Preview: "go code"}}
	d.Show(blocks, "full text", "/tmp")

	// Move to code block.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.IsVisible() {
		t.Error("expected dialog to close on enter")
	}
	if cmd == nil {
		t.Error("expected clipboard command on enter")
	}
}

func TestCodeBlockDialogWriteKey(t *testing.T) {
	d := NewCodeBlockDialog()
	blocks := []CodeBlock{{Language: "go", Code: "go code", Preview: "go code"}}
	d.Show(blocks, "full text", "/tmp")

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if d.IsVisible() {
		t.Error("expected dialog to close on w")
	}
	if cmd == nil {
		t.Error("expected write command on w")
	}
}

func TestCodeBlockDialogSelectedText(t *testing.T) {
	d := NewCodeBlockDialog()
	blocks := []CodeBlock{
		{Language: "go", Code: "go code", Preview: "go code"},
		{Language: "py", Code: "py code", Preview: "py code"},
	}
	d.Show(blocks, "full text", "/tmp")

	// Index 0 = full text.
	if got := d.selectedText(); got != "full text" {
		t.Errorf("selectedText() at 0 = %q, want %q", got, "full text")
	}

	// Index 1 = first block.
	d.selected = 1
	if got := d.selectedText(); got != "go code" {
		t.Errorf("selectedText() at 1 = %q, want %q", got, "go code")
	}

	// Index 2 = second block.
	d.selected = 2
	if got := d.selectedText(); got != "py code" {
		t.Errorf("selectedText() at 2 = %q, want %q", got, "py code")
	}
}

func TestNextAvailableFilename(t *testing.T) {
	dir := t.TempDir()

	// First call returns base name.
	name := nextAvailableFilename(dir, "code-block", ".txt")
	if name != "code-block.txt" {
		t.Errorf("first name = %q, want %q", name, "code-block.txt")
	}

	// Create the file, next call should return -2.
	os.WriteFile(filepath.Join(dir, "code-block.txt"), []byte("x"), 0o644)
	name = nextAvailableFilename(dir, "code-block", ".txt")
	if name != "code-block-2.txt" {
		t.Errorf("second name = %q, want %q", name, "code-block-2.txt")
	}
}

func TestCodeBlockDialogView(t *testing.T) {
	d := NewCodeBlockDialog()
	d.SetSize(100, 30)
	blocks := []CodeBlock{
		{Language: "go", Code: "fmt.Println()", Preview: "fmt.Println()"},
	}
	d.Show(blocks, "full text", "/tmp")

	view := d.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestCodeBlockDialogViewHidden(t *testing.T) {
	d := NewCodeBlockDialog()
	if d.View() != "" {
		t.Error("expected empty view when hidden")
	}
}
