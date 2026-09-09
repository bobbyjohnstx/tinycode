package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModelDialogOpenMsg signals the model dialog should open.
type ModelDialogOpenMsg struct{}

// ModelSelectedMsg signals a model was selected.
type ModelSelectedMsg struct {
	Selection ModelSelection
}

type modelDialogPhase int

const (
	phaseProviders modelDialogPhase = iota
	phaseModels
)

// ModelDialog displays a two-step provider/model selection dialog.
type ModelDialog struct {
	providers      []ProviderInfo
	phase          modelDialogPhase
	selectedProv   int
	selectedModel  int
	scrollProv     int
	scrollModel    int
	filter         string
	pendingModelID string
	visible        bool
	width          int
	height         int
}

// NewModelDialog creates a ModelDialog.
func NewModelDialog() ModelDialog {
	return ModelDialog{}
}

// Show opens the dialog with the given providers.
func (d *ModelDialog) Show(providers []ProviderInfo, current ...ModelSelection) {
	d.providers = providers
	d.visible = true
	d.phase = phaseProviders
	d.selectedProv = 0
	d.selectedModel = 0
	d.scrollProv = 0
	d.scrollModel = 0
	d.filter = ""
	d.pendingModelID = ""

	if len(current) > 0 && current[0].ProviderID != "" {
		for i, p := range providers {
			if p.ID == current[0].ProviderID {
				d.selectedProv = i
				d.pendingModelID = current[0].ModelID
				break
			}
		}
	}
}

// Hide closes the dialog.
func (d *ModelDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d ModelDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *ModelDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *ModelDialog) maxVisibleProviders() int {
	mv := d.height - 8
	if mv < 3 {
		mv = 3
	}
	return mv
}

func (d *ModelDialog) maxVisibleModels() int {
	return 8
}

func ensureScrollVisible(selected, scroll, maxVis, total int) int {
	if selected < scroll {
		scroll = selected
	}
	if selected >= scroll+maxVis {
		scroll = selected - maxVis + 1
	}
	if scroll > total-maxVis {
		scroll = total - maxVis
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// Update handles key events for the model dialog.
func (d ModelDialog) Update(msg tea.Msg) (ModelDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	if keyMsg.String() == "esc" {
		if d.phase == phaseModels {
			d.phase = phaseProviders
			d.filter = ""
			return d, nil
		}
		d.visible = false
		return d, nil
	}

	switch d.phase {
	case phaseProviders:
		return d.updateProviders(keyMsg)
	case phaseModels:
		return d.updateModels(keyMsg)
	}
	return d, nil
}

func (d ModelDialog) updateProviders(keyMsg tea.KeyMsg) (ModelDialog, tea.Cmd) {
	if len(d.providers) == 0 {
		return d, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		d.selectedProv = wrapIndex(d.selectedProv-1, len(d.providers))
		d.scrollProv = ensureScrollVisible(d.selectedProv, d.scrollProv, d.maxVisibleProviders(), len(d.providers))
	case "down", "j":
		d.selectedProv = wrapIndex(d.selectedProv+1, len(d.providers))
		d.scrollProv = ensureScrollVisible(d.selectedProv, d.scrollProv, d.maxVisibleProviders(), len(d.providers))
	case "enter":
		if d.selectedProv < len(d.providers) {
			d.phase = phaseModels
			d.selectedModel = 0
			d.scrollModel = 0
			d.filter = ""
			if d.pendingModelID != "" {
				for i, m := range d.providers[d.selectedProv].Models {
					if m.ID == d.pendingModelID {
						d.selectedModel = i
						d.scrollModel = ensureScrollVisible(i, 0, d.maxVisibleModels(), len(d.providers[d.selectedProv].Models))
						break
					}
				}
				d.pendingModelID = ""
			}
		}
	}
	return d, nil
}

func (d ModelDialog) updateModels(keyMsg tea.KeyMsg) (ModelDialog, tea.Cmd) {
	prov := d.providers[d.selectedProv]
	models := d.filteredModels(prov)

	if len(models) == 0 {
		switch keyMsg.String() {
		case "backspace":
			if len(d.filter) > 0 {
				d.filter = d.filter[:len(d.filter)-1]
				d.selectedModel = 0
				d.scrollModel = 0
			}
		}
		return d, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		d.selectedModel = wrapIndex(d.selectedModel-1, len(models))
		d.scrollModel = ensureScrollVisible(d.selectedModel, d.scrollModel, d.maxVisibleModels(), len(models))
	case "down", "j":
		d.selectedModel = wrapIndex(d.selectedModel+1, len(models))
		d.scrollModel = ensureScrollVisible(d.selectedModel, d.scrollModel, d.maxVisibleModels(), len(models))
	case "enter":
		if d.selectedModel < len(models) {
			m := models[d.selectedModel]
			d.visible = false
			return d, func() tea.Msg {
				return ModelSelectedMsg{
					Selection: ModelSelection{
						ProviderID: prov.ID,
						ModelID:    m.ID,
					},
				}
			}
		}
	case "backspace":
		if len(d.filter) > 0 {
			d.filter = d.filter[:len(d.filter)-1]
			d.selectedModel = 0
			d.scrollModel = 0
		}
	default:
		r := keyMsg.String()
		added := false
		for _, ch := range r {
			if ch >= ' ' && ch <= '~' {
				d.filter += string(ch)
				added = true
			}
		}
		if added {
			d.selectedModel = 0
			d.scrollModel = 0
		}
	}
	return d, nil
}

func (d *ModelDialog) filteredModels(prov ProviderInfo) []ModelInfo {
	if d.filter == "" {
		return prov.Models
	}
	lower := strings.ToLower(d.filter)
	var result []ModelInfo
	for _, m := range prov.Models {
		if strings.Contains(strings.ToLower(m.Name), lower) {
			result = append(result, m)
		}
	}
	return result
}

// View renders the model dialog.
func (d ModelDialog) View() string {
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

	var content string
	switch d.phase {
	case phaseProviders:
		content = d.viewProviders()
	case phaseModels:
		content = d.viewModels()
	}

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

func (d ModelDialog) viewProviders() string {
	var sb strings.Builder
	sb.WriteString("Select Provider\n")

	mv := d.maxVisibleProviders()
	end := d.scrollProv + mv
	if end > len(d.providers) {
		end = len(d.providers)
	}

	if d.scrollProv > 0 {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  ▲ more"))
	}

	for i := d.scrollProv; i < end; i++ {
		sb.WriteString("\n")
		p := d.providers[i]
		label := p.Name
		modelCount := len(p.Models)
		suffix := styleMetadata.Render(" (" + itoa(modelCount) + " models)")

		if i == d.selectedProv {
			sb.WriteString(styleSelected.Render("▸ "+label) + suffix)
		} else {
			sb.WriteString("  " + label + suffix)
		}
	}

	if end < len(d.providers) {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  ▼ more"))
	}

	return sb.String()
}

func (d ModelDialog) viewModels() string {
	prov := d.providers[d.selectedProv]
	models := d.filteredModels(prov)

	var sb strings.Builder
	sb.WriteString(styleToolName.Render(prov.Name))
	sb.WriteString("\n")

	if d.filter != "" {
		sb.WriteString("\n")
		sb.WriteString("  " + styleMetadata.Render("Search:") + " " + styleToolName.Render(d.filter))
	} else if len(prov.Models) > 10 {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  Type to search"))
	}

	mv := d.maxVisibleModels()
	end := d.scrollModel + mv
	if end > len(models) {
		end = len(models)
	}

	if d.scrollModel > 0 {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  ▲ more"))
	}

	for i := d.scrollModel; i < end; i++ {
		sb.WriteString("\n")
		m := models[i]

		if i == d.selectedModel {
			sb.WriteString(styleSelected.Render("▸ " + m.Name))
		} else {
			sb.WriteString("  " + m.Name)
		}
	}

	if end < len(models) {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  ▼ more"))
	}

	sb.WriteString("\n\n")
	sb.WriteString(styleMetadata.Render("  esc back"))

	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// wrapIndex wraps an index within [0, length).
func wrapIndex(i, length int) int {
	if length == 0 {
		return 0
	}
	return ((i % length) + length) % length
}
