package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CodeBlockWrittenMsg is emitted after a code block is written to a file.
type CodeBlockWrittenMsg struct {
	Path string
	Err  error
}

// CodeBlockDialog displays a picker for code blocks within a response.
type CodeBlockDialog struct {
	blocks   []CodeBlock
	fullText string
	cwd      string
	selected int
	visible  bool
	width    int
	height   int
}

// NewCodeBlockDialog creates a CodeBlockDialog.
func NewCodeBlockDialog() CodeBlockDialog {
	return CodeBlockDialog{}
}

// Show opens the dialog with the given code blocks and full response text.
func (d *CodeBlockDialog) Show(blocks []CodeBlock, fullText, cwd string) {
	d.blocks = blocks
	d.fullText = fullText
	d.cwd = cwd
	d.selected = 0
	d.visible = true
}

// Hide closes the dialog.
func (d *CodeBlockDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d CodeBlockDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *CodeBlockDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key events for the code block picker.
func (d CodeBlockDialog) Update(msg tea.Msg) (CodeBlockDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	itemCount := len(d.blocks) + 1 // +1 for "Full response"
	switch keyMsg.String() {
	case "up", "k":
		if d.selected > 0 {
			d.selected--
		}
	case "down", "j":
		if d.selected < itemCount-1 {
			d.selected++
		}
	case "enter":
		text := d.selectedText()
		d.visible = false
		return d, copyToClipboard(text)
	case "w":
		text := d.selectedText()
		cwd := d.cwd
		d.visible = false
		return d, writeCodeBlockToFile(text, cwd)
	case "esc", "q":
		d.visible = false
	}

	return d, nil
}

// selectedText returns the text for the currently selected item.
func (d CodeBlockDialog) selectedText() string {
	if d.selected == 0 {
		return d.fullText
	}
	idx := d.selected - 1
	if idx < len(d.blocks) {
		return d.blocks[idx].Code
	}
	return d.fullText
}

// View renders the code block picker dialog.
func (d CodeBlockDialog) View() string {
	if !d.visible {
		return ""
	}

	dialogWidth := d.width / 2
	if dialogWidth < 40 {
		dialogWidth = 40
	}
	if dialogWidth > 80 {
		dialogWidth = 80
	}

	var sb strings.Builder
	sb.WriteString("Code Blocks  ")
	sb.WriteString(styleMetadata.Render("enter=copy  w=file  esc=close"))
	sb.WriteString("\n\n")

	// "Full response" option at top.
	if d.selected == 0 {
		sb.WriteString(styleSelected.Render("▸ Full response"))
	} else {
		sb.WriteString("  Full response")
	}

	contentWidth := dialogWidth - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	for i, block := range d.blocks {
		sb.WriteString("\n\n")

		label := "code"
		if block.Language != "" {
			label = block.Language
		}

		preview := block.Preview
		maxPreview := contentWidth - len(label) - 5
		if maxPreview < 10 {
			maxPreview = 10
		}
		if len(preview) > maxPreview {
			preview = preview[:maxPreview-3] + "..."
		}

		line := fmt.Sprintf("[%s] %s", label, styleMetadata.Render(preview))

		if d.selected == i+1 {
			sb.WriteString(styleSelected.Render("▸ " + line))
		} else {
			sb.WriteString("  " + line)
		}
	}

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

// writeCodeBlockToFile writes text to a file in the working directory.
func writeCodeBlockToFile(text, cwd string) tea.Cmd {
	return func() tea.Msg {
		name := nextAvailableFilename(cwd, "code-block", ".txt")
		path := filepath.Join(cwd, name)
		err := os.WriteFile(path, []byte(text+"\n"), 0o644)
		if err != nil {
			return CodeBlockWrittenMsg{Err: err}
		}
		return CodeBlockWrittenMsg{Path: path}
	}
}

// nextAvailableFilename returns a filename like "code-block.txt",
// "code-block-2.txt", etc. that doesn't already exist.
func nextAvailableFilename(dir, base, ext string) string {
	name := base + ext
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	for i := 2; i < 1000; i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
	return fmt.Sprintf("%s-%d%s", base, 999, ext)
}
