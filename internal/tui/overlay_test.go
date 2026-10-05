package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// --- Permission prompt tests ---

func TestPermissionPrompt_DefaultIsAllow(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "1", Permission: "shell"})

	// Press enter immediately — should default to Allow.
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	dm, ok := msg.(PermissionDismissedMsg)
	if !ok {
		t.Fatalf("expected PermissionDismissedMsg, got %T", msg)
	}
	if dm.Action != PermissionAllow {
		t.Errorf("expected Allow, got %v", dm.Action)
	}
	if p.IsVisible() {
		t.Error("expected prompt to be hidden after enter")
	}
}

func TestPermissionPrompt_NavigateAndSelect(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "2", Permission: "edit"})

	// Navigate right twice to reach Reject.
	p, _ = p.Update(keyMsg("right"))
	p, _ = p.Update(keyMsg("right"))

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	dm := msg.(PermissionDismissedMsg)
	if dm.Action != PermissionReject {
		t.Errorf("expected Reject, got %v", dm.Action)
	}
}

func TestPermissionPrompt_EscapeDoesNotReject(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "3", Permission: "edit"})

	p, cmd := p.Update(keyMsg("esc"))
	if cmd != nil {
		t.Fatal("expected no cmd from escape; Esc must not reject")
	}
	if !p.IsVisible() {
		t.Error("expected prompt to remain visible after escape")
	}

	// Allow flow still works after ignored Esc.
	p, cmd = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter after escape")
	}
	msg := cmd()
	dm := msg.(PermissionDismissedMsg)
	if dm.Action != PermissionAllow {
		t.Errorf("expected Allow after escape+enter, got %v", dm.Action)
	}
}

func TestPermissionPrompt_LeftBoundsCheck(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "4", Permission: "shell"})

	// Already at Allow (index 0); pressing left should stay at Allow.
	p, _ = p.Update(keyMsg("left"))
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	msg := cmd()
	dm := msg.(PermissionDismissedMsg)
	if dm.Action != PermissionAllow {
		t.Errorf("expected Allow after left at boundary, got %v", dm.Action)
	}
}

// --- Command palette tests ---

func TestPalette_FilterReducesItems(t *testing.T) {
	p := NewCommandPalette()
	p.Show([]PaletteItem{
		{Label: "build", Description: "default agent"},
		{Label: "plan", Description: "read-only planning"},
		{Label: "debug", Description: "debugging"},
	})

	if len(p.filtered) != 3 {
		t.Fatalf("expected 3 items initially, got %d", len(p.filtered))
	}

	// Simulate typing "bu" into the input.
	p.filtered = filterItems(p.items, "bu")
	if len(p.filtered) != 2 {
		t.Errorf("expected 2 items matching 'bu' (build, debug), got %d", len(p.filtered))
	}

	p.filtered = filterItems(p.items, "plan")
	if len(p.filtered) != 1 {
		t.Errorf("expected 1 item matching 'plan', got %d", len(p.filtered))
	}
	if p.filtered[0].Label != "plan" {
		t.Errorf("expected 'plan', got %q", p.filtered[0].Label)
	}
}

func TestPalette_FilterCaseInsensitive(t *testing.T) {
	items := []PaletteItem{
		{Label: "Build"},
		{Label: "Plan"},
	}

	filtered := filterItems(items, "BUILD")
	if len(filtered) != 1 {
		t.Errorf("expected 1 item for case-insensitive match, got %d", len(filtered))
	}
}

func TestPalette_EscapeDismisses(t *testing.T) {
	p := NewCommandPalette()
	p.Show([]PaletteItem{{Label: "test"}})

	p, cmd := p.Update(keyMsg("esc"))
	if p.IsVisible() {
		t.Error("expected palette hidden after escape")
	}
	if cmd == nil {
		t.Fatal("expected PaletteClosedMsg cmd")
	}
	msg := cmd()
	if _, ok := msg.(PaletteClosedMsg); !ok {
		t.Errorf("expected PaletteClosedMsg, got %T", msg)
	}
}

func TestPalette_EnterSelectsItem(t *testing.T) {
	p := NewCommandPalette()
	p.Show([]PaletteItem{
		{Label: "first", Value: "v1"},
		{Label: "second", Value: "v2"},
	})

	// Navigate down once, then select.
	p, _ = p.Update(keyMsg("down"))
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	sm, ok := msg.(PaletteSelectedMsg)
	if !ok {
		t.Fatalf("expected PaletteSelectedMsg, got %T", msg)
	}
	if sm.Item.Value != "v2" {
		t.Errorf("expected value 'v2', got %q", sm.Item.Value)
	}
}

func TestPalette_EmptyFilterShowsAll(t *testing.T) {
	items := []PaletteItem{
		{Label: "a"},
		{Label: "b"},
		{Label: "c"},
	}
	filtered := filterItems(items, "")
	if len(filtered) != 3 {
		t.Errorf("expected 3 items for empty query, got %d", len(filtered))
	}
}

// --- Model dialog tests ---

func TestModelDialog_NavWraps(t *testing.T) {
	d := NewModelDialog()
	d.Show([]ProviderInfo{
		{
			ID:   "ollama",
			Name: "Ollama",
			Models: []ModelInfo{
				{ID: "m1", Name: "model-1"},
				{ID: "m2", Name: "model-2"},
			},
		},
	})

	// Phase 1: provider list. Enter selects the provider.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Phase 2: model list. Navigate up to wrap to m2.
	d, _ = d.Update(keyMsg("k"))

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	sm, ok := msg.(ModelSelectedMsg)
	if !ok {
		t.Fatalf("expected ModelSelectedMsg, got %T", msg)
	}
	if sm.Selection.ModelID != "m2" {
		t.Errorf("expected m2 after wrapping, got %q", sm.Selection.ModelID)
	}
}

func TestModelDialog_EscapeHides(t *testing.T) {
	d := NewModelDialog()
	d.Show([]ProviderInfo{
		{ID: "p1", Name: "P1", Models: []ModelInfo{{ID: "m1", Name: "M1"}}},
	})

	d, _ = d.Update(keyMsg("esc"))
	if d.IsVisible() {
		t.Error("expected dialog hidden after escape")
	}
}

func TestModelDialog_EnterOnModel(t *testing.T) {
	d := NewModelDialog()
	d.Show([]ProviderInfo{
		{
			ID:   "openai",
			Name: "OpenAI",
			Models: []ModelInfo{
				{ID: "gpt-4", Name: "GPT-4"},
			},
		},
	})

	// Phase 1: select provider.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Phase 2: first model is pre-selected, press enter.
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter on model")
	}

	msg := cmd()
	sm := msg.(ModelSelectedMsg)
	if sm.Selection.ProviderID != "openai" || sm.Selection.ModelID != "gpt-4" {
		t.Errorf("expected openai/gpt-4, got %s/%s", sm.Selection.ProviderID, sm.Selection.ModelID)
	}
}

// --- Agent dialog tests ---

func TestAgentDialog_NavWraps(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "build", Description: "default"},
		{Name: "plan", Description: "read-only"},
		{Name: "debug", Description: "debugging"},
	})

	// At index 0 (build). Press up to wrap to debug.
	d, _ = d.Update(keyMsg("k"))
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	am := msg.(AgentSelectedMsg)
	if am.Agent != "debug" {
		t.Errorf("expected 'debug' after wrap, got %q", am.Agent)
	}
}

func TestAgentDialog_EscapeHides(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{{Name: "build"}})

	d, _ = d.Update(keyMsg("esc"))
	if d.IsVisible() {
		t.Error("expected dialog hidden after escape")
	}
}

func TestAgentDialog_EnterSelectsAgent(t *testing.T) {
	d := NewAgentDialog()
	d.Show([]api.AgentInfo{
		{Name: "build"},
		{Name: "plan"},
	})

	d, _ = d.Update(keyMsg("j"))
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	am := msg.(AgentSelectedMsg)
	if am.Agent != "plan" {
		t.Errorf("expected 'plan', got %q", am.Agent)
	}
}

// --- Model scoping tests ---

func TestModelDialog_ScopedFiltersModels(t *testing.T) {
	d := NewModelDialog()
	d.SetScopedModels([]string{"ollama/llama3", "openai/gpt-4"})
	d.Show([]ProviderInfo{
		{
			ID:   "ollama",
			Name: "Ollama",
			Models: []ModelInfo{
				{ID: "llama3", Name: "Llama 3"},
				{ID: "mistral", Name: "Mistral"},
				{ID: "phi3", Name: "Phi-3"},
			},
		},
		{
			ID:   "openai",
			Name: "OpenAI",
			Models: []ModelInfo{
				{ID: "gpt-4", Name: "GPT-4"},
				{ID: "gpt-3.5", Name: "GPT-3.5"},
			},
		},
	})

	// Enter to select Ollama provider.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// In normal mode with scoped models, only scoped models should show.
	prov := d.providers[d.selectedProv]
	models := d.filteredModels(prov)
	if len(models) != 1 {
		t.Fatalf("expected 1 scoped model for ollama, got %d", len(models))
	}
	if models[0].ID != "llama3" {
		t.Errorf("expected llama3, got %s", models[0].ID)
	}
}

func TestModelDialog_NoScopedShowsAll(t *testing.T) {
	d := NewModelDialog()
	// No scoped models set — should show all.
	d.Show([]ProviderInfo{
		{
			ID:   "ollama",
			Name: "Ollama",
			Models: []ModelInfo{
				{ID: "llama3", Name: "Llama 3"},
				{ID: "mistral", Name: "Mistral"},
			},
		},
	})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	prov := d.providers[d.selectedProv]
	models := d.filteredModels(prov)
	if len(models) != 2 {
		t.Fatalf("expected 2 models (no scoping), got %d", len(models))
	}
}

func TestModelDialog_ScopingModeShowsAll(t *testing.T) {
	d := NewModelDialog()
	d.SetScopedModels([]string{"ollama/llama3"})
	d.ShowScoping([]ProviderInfo{
		{
			ID:   "ollama",
			Name: "Ollama",
			Models: []ModelInfo{
				{ID: "llama3", Name: "Llama 3"},
				{ID: "mistral", Name: "Mistral"},
			},
		},
	})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	prov := d.providers[d.selectedProv]
	models := d.filteredModels(prov)
	if len(models) != 2 {
		t.Fatalf("expected 2 models in scoping mode, got %d", len(models))
	}
}

func TestModelDialog_SpaceTogglesScope(t *testing.T) {
	d := NewModelDialog()
	d.SetScopedModels([]string{})
	d.ShowScoping([]ProviderInfo{
		{
			ID:   "ollama",
			Name: "Ollama",
			Models: []ModelInfo{
				{ID: "llama3", Name: "Llama 3"},
			},
		},
	})

	// Enter to select provider.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Press space to scope the model.
	d, cmd := d.Update(keyMsg(" "))
	if cmd == nil {
		t.Fatal("expected ModelScopedMsg cmd from space")
	}

	msg := cmd()
	sm, ok := msg.(ModelScopedMsg)
	if !ok {
		t.Fatalf("expected ModelScopedMsg, got %T", msg)
	}
	if len(sm.ScopedModels) != 1 {
		t.Fatalf("expected 1 scoped model, got %d", len(sm.ScopedModels))
	}
	if sm.ScopedModels[0] != "ollama/llama3" {
		t.Errorf("expected ollama/llama3, got %s", sm.ScopedModels[0])
	}

	// Press space again to unscope.
	d, cmd = d.Update(keyMsg(" "))
	if cmd == nil {
		t.Fatal("expected ModelScopedMsg cmd from second space")
	}

	msg = cmd()
	sm = msg.(ModelScopedMsg)
	if len(sm.ScopedModels) != 0 {
		t.Errorf("expected 0 scoped models after unscope, got %d", len(sm.ScopedModels))
	}
}

func TestModelDialog_ScopeIndicatorInView(t *testing.T) {
	d := NewModelDialog()
	d.SetScopedModels([]string{"ollama/llama3"})
	d.Show([]ProviderInfo{
		{
			ID:   "ollama",
			Name: "Ollama",
			Models: []ModelInfo{
				{ID: "llama3", Name: "Llama 3"},
			},
		},
	})
	d.SetSize(80, 40)

	// Enter models phase.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := d.View()
	if !containsRune(view, '★') {
		t.Error("expected ★ indicator for scoped model in view")
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
