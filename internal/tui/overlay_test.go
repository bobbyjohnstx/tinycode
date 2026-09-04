package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

// --- Permission prompt tests ---

func TestPermissionPrompt_DefaultIsAllow(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "1", Tool: "shell"})

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
	p.Show(PermissionRequest{ID: "2", Tool: "write"})

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

func TestPermissionPrompt_EscapeRejects(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "3", Tool: "edit"})

	p, cmd := p.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("expected cmd from escape")
	}

	msg := cmd()
	dm := msg.(PermissionDismissedMsg)
	if dm.Action != PermissionReject {
		t.Errorf("expected Reject on escape, got %v", dm.Action)
	}
	if p.IsVisible() {
		t.Error("expected prompt to be hidden after escape")
	}
}

func TestPermissionPrompt_LeftBoundsCheck(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "4", Tool: "shell"})

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

	// Items: [header(0), m1(1), m2(2)].
	// Initial selection starts at 0 (header).
	// Press down to get to first model.
	d, _ = d.Update(keyMsg("j"))
	// Should be at m1 (index 1).

	// Press up — should wrap to m2 (index 2) since header is skipped.
	d, _ = d.Update(keyMsg("k"))
	// Should be at m2 (index 2).

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

	// Navigate to the model (skip header).
	d, _ = d.Update(keyMsg("j"))
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

