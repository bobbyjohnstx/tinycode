package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ThemeSelectedMsg struct {
	ThemeID string
}

// ThemePreviewMsg fires on every cursor move for live preview.
type ThemePreviewMsg struct {
	ThemeID string
}

// ThemeRevertMsg fires when dialog is dismissed without confirming.
type ThemeRevertMsg struct {
	ThemeID string
}

type ThemeDialog struct {
	themes   []ColorTheme
	selected int
	scroll   int
	filter   string
	visible  bool
	width    int
	height   int
	current  string
	initial  string
}

func NewThemeDialog() ThemeDialog {
	return ThemeDialog{}
}

func (d *ThemeDialog) Show(themes []ColorTheme, currentID string) {
	d.themes = themes
	d.current = currentID
	d.initial = currentID
	d.selected = 0
	d.scroll = 0
	d.filter = ""
	d.visible = true
	for i, t := range themes {
		if t.ID == currentID {
			d.selected = i
			break
		}
	}
}

func (d *ThemeDialog) IsVisible() bool {
	return d.visible
}

func (d *ThemeDialog) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d ThemeDialog) maxVisible() int {
	return 12
}

func (d ThemeDialog) filtered() []ColorTheme {
	if d.filter == "" {
		return d.themes
	}
	q := strings.ToLower(d.filter)
	var result []ColorTheme
	for _, t := range d.themes {
		if strings.Contains(strings.ToLower(t.Name), q) || strings.Contains(strings.ToLower(t.ID), q) {
			result = append(result, t)
		}
	}
	return result
}

func (d ThemeDialog) Update(msg tea.KeyMsg) (ThemeDialog, tea.Cmd) {
	items := d.filtered()

	switch msg.String() {
	case "up":
		if d.selected > 0 {
			d.selected--
		} else {
			d.selected = max(0, len(items)-1)
		}
		if d.selected < d.scroll {
			d.scroll = d.selected
		}
		return d, d.previewCmd(items)

	case "down":
		if d.selected < len(items)-1 {
			d.selected++
		} else {
			d.selected = 0
		}
		if d.selected >= d.scroll+d.maxVisible() {
			d.scroll = d.selected - d.maxVisible() + 1
		}
		return d, d.previewCmd(items)

	case "enter":
		if d.selected >= 0 && d.selected < len(items) {
			id := items[d.selected].ID
			d.visible = false
			d.current = id
			return d, func() tea.Msg { return ThemeSelectedMsg{ThemeID: id} }
		}

	case "esc":
		d.visible = false
		initial := d.initial
		return d, func() tea.Msg { return ThemeRevertMsg{ThemeID: initial} }

	case "backspace":
		if len(d.filter) > 0 {
			d.filter = d.filter[:len(d.filter)-1]
			d.selected = 0
			d.scroll = 0
			return d, d.previewCmd(d.filtered())
		}

	default:
		r := msg.String()
		for _, ch := range r {
			if ch >= ' ' && ch <= '~' {
				d.filter += string(ch)
			}
		}
		d.selected = 0
		d.scroll = 0
		return d, d.previewCmd(d.filtered())
	}

	return d, nil
}

func (d ThemeDialog) previewCmd(items []ColorTheme) tea.Cmd {
	if d.selected >= 0 && d.selected < len(items) {
		id := items[d.selected].ID
		return func() tea.Msg { return ThemePreviewMsg{ThemeID: id} }
	}
	return nil
}

func (d ThemeDialog) View() string {
	if !d.visible {
		return ""
	}

	items := d.filtered()
	maxVis := d.maxVisible()

	dialogW := 50
	if dialogW > d.width-4 {
		dialogW = d.width - 4
	}

	title := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}).
		Render("Select Theme")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	if d.filter != "" {
		filterLine := "  " + styleMetadata.Render("Search:") + " " + styleToolName.Render(d.filter)
		lines = append(lines, filterLine)
		lines = append(lines, "")
	}

	end := d.scroll + maxVis
	if end > len(items) {
		end = len(items)
	}
	visible := items[d.scroll:end]

	highlightBg := lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}

	for i, t := range visible {
		idx := d.scroll + i
		name := t.Name
		if len(name) > dialogW-6 {
			name = name[:dialogW-9] + "..."
		}

		indicator := "  "
		if t.ID == d.current {
			indicator = "● "
		}

		row := indicator + name
		if idx == d.selected {
			row = lipgloss.NewStyle().
				Background(highlightBg).
				Bold(true).
				Width(dialogW - 4).
				Render(row)
		} else {
			row = lipgloss.NewStyle().
				Width(dialogW - 4).
				Render(row)
		}
		lines = append(lines, row)
	}

	if len(items) > maxVis {
		scrollInfo := styleMetadata.Render(
			strings.Repeat(" ", dialogW-20) +
				"↑↓ scroll · type to filter")
		lines = append(lines, "")
		lines = append(lines, scrollInfo)
	}

	content := strings.Join(lines, "\n")
	dialog := styleDialogBorder.Width(dialogW).Render(content)

	padTop := (d.height - strings.Count(dialog, "\n") - 1) / 3
	if padTop < 1 {
		padTop = 1
	}
	padLeft := (d.width - dialogW - 4) / 2
	if padLeft < 0 {
		padLeft = 0
	}

	return strings.Repeat("\n", padTop) +
		lipgloss.NewStyle().PaddingLeft(padLeft).Render(dialog)
}
