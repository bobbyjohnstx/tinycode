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

// StoreOpenRouterAuthMsg requests storing an OpenRouter API key then reloading providers.
type StoreOpenRouterAuthMsg struct {
	APIKey string
}

type modelDialogPhase int

const (
	phaseProviders modelDialogPhase = iota
	phaseModels
	phaseAPIKey
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
	apiKey         string
	pendingModelID string
	scopedModels   map[string]bool
	scopingMode    bool
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
	d.apiKey = ""
	d.pendingModelID = ""
	d.scopingMode = false

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

// ShowScoping opens the dialog in scoping mode where all models are shown
// and the user can toggle models in/out of the scoped list.
func (d *ModelDialog) ShowScoping(providers []ProviderInfo) {
	d.providers = providers
	d.visible = true
	d.phase = phaseProviders
	d.selectedProv = 0
	d.selectedModel = 0
	d.scrollProv = 0
	d.scrollModel = 0
	d.filter = ""
	d.apiKey = ""
	d.pendingModelID = ""
	d.scopingMode = true
}

// SetScopedModels sets the current scoped models lookup.
func (d *ModelDialog) SetScopedModels(models []string) {
	d.scopedModels = make(map[string]bool, len(models))
	for _, m := range models {
		d.scopedModels[m] = true
	}
}

// ScopedModelsList returns the scoped models as a sorted slice.
func (d *ModelDialog) ScopedModelsList() []string {
	result := make([]string, 0, len(d.scopedModels))
	for k := range d.scopedModels {
		result = append(result, k)
	}
	return result
}

// isModelScoped checks if a provider/model combo is in the scoped set.
func (d *ModelDialog) isModelScoped(providerID, modelID string) bool {
	return d.scopedModels[providerID+"/"+modelID]
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
		switch d.phase {
		case phaseModels:
			d.phase = phaseProviders
			d.filter = ""
			return d, nil
		case phaseAPIKey:
			d.phase = phaseProviders
			d.apiKey = ""
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
	case phaseAPIKey:
		return d.updateAPIKey(keyMsg)
	}
	return d, nil
}

func (d ModelDialog) updateProviders(keyMsg tea.KeyMsg) (ModelDialog, tea.Cmd) {
	if len(d.providers) == 0 {
		if keyMsg.String() == "o" {
			d.phase = phaseAPIKey
			d.apiKey = ""
		}
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
		if d.selectedProv >= len(d.providers) {
			break
		}
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
	case " ":
		// Toggle scoped status for the selected model.
		if d.selectedModel < len(models) {
			m := models[d.selectedModel]
			key := prov.ID + "/" + m.ID
			if d.scopedModels == nil {
				d.scopedModels = make(map[string]bool)
			}
			if d.scopedModels[key] {
				delete(d.scopedModels, key)
			} else {
				d.scopedModels[key] = true
			}
			scoped := d.ScopedModelsList()
			return d, func() tea.Msg {
				return ModelScopedMsg{ScopedModels: scoped}
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

func (d ModelDialog) updateAPIKey(keyMsg tea.KeyMsg) (ModelDialog, tea.Cmd) {
	switch keyMsg.String() {
	case "enter":
		key := strings.TrimSpace(d.apiKey)
		if key == "" {
			return d, nil
		}
		d.phase = phaseProviders
		d.apiKey = ""
		return d, func() tea.Msg {
			return StoreOpenRouterAuthMsg{APIKey: key}
		}
	case "backspace":
		if len(d.apiKey) > 0 {
			d.apiKey = d.apiKey[:len(d.apiKey)-1]
		}
	default:
		r := keyMsg.String()
		for _, ch := range r {
			if ch >= ' ' && ch <= '~' {
				d.apiKey += string(ch)
			}
		}
	}
	return d, nil
}

func (d *ModelDialog) filteredModels(prov ProviderInfo) []ModelInfo {
	var candidates []ModelInfo

	// In scoping mode or when no models are scoped, show all models.
	// Otherwise, only show scoped models.
	if d.scopingMode || len(d.scopedModels) == 0 {
		candidates = prov.Models
	} else {
		for _, m := range prov.Models {
			if d.scopedModels[prov.ID+"/"+m.ID] {
				candidates = append(candidates, m)
			}
		}
	}

	if d.filter == "" {
		return candidates
	}
	lower := strings.ToLower(d.filter)
	var result []ModelInfo
	for _, m := range candidates {
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
	case phaseAPIKey:
		content = d.viewAPIKey()
	}

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

func (d ModelDialog) viewProviders() string {
	var sb strings.Builder
	if d.scopingMode {
		sb.WriteString("Scope Models (favorites)\n")
	} else {
		sb.WriteString("Select Provider\n")
	}

	if len(d.providers) == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  No providers found."))
		sb.WriteString("\n\n")
		sb.WriteString("  Start Ollama locally:\n")
		sb.WriteString(styleMetadata.Render("    ollama serve"))
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("    ollama pull <model>"))
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  then reopen /connect"))
		sb.WriteString("\n\n")
		sb.WriteString("  Or press ")
		sb.WriteString(styleSelected.Render("o"))
		sb.WriteString(" to enter an OpenRouter API key\n")
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  esc close"))
		return sb.String()
	}

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
		modelCount := len(d.filteredModels(p))
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

func (d ModelDialog) viewAPIKey() string {
	var sb strings.Builder
	sb.WriteString("OpenRouter API Key\n\n")
	sb.WriteString("  Enter your API key from openrouter.ai\n\n")
	masked := strings.Repeat("•", len(d.apiKey))
	sb.WriteString("  " + styleToolName.Render(masked))
	if d.apiKey == "" {
		sb.WriteString(styleMetadata.Render("▋"))
	}
	sb.WriteString("\n\n")
	sb.WriteString(styleMetadata.Render("  enter submit  esc cancel"))
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

		scopeIndicator := ""
		if d.isModelScoped(prov.ID, m.ID) {
			scopeIndicator = " ★"
		}

		if i == d.selectedModel {
			sb.WriteString(styleSelected.Render("▸ "+m.Name) + styleMetadata.Render(scopeIndicator))
		} else {
			sb.WriteString("  " + m.Name + styleMetadata.Render(scopeIndicator))
		}
	}

	if end < len(models) {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  ▼ more"))
	}

	sb.WriteString("\n\n")
	sb.WriteString(styleMetadata.Render("  esc back  space scope"))

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
