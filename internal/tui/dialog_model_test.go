package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelDialog_ShowFirstRun_TitleAndFooter(t *testing.T) {
	d := NewModelDialog()
	d.ShowFirstRun([]ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", ProviderID: "ollama", Name: "Qwen"}}},
	})
	d.SetSize(80, 24)

	view := d.View()
	if !strings.Contains(view, "Select a model to start") {
		t.Errorf("expected first-run title, got %q", view)
	}
	if strings.Contains(view, "Select Provider") {
		t.Error("normal Select Provider title should not appear on first-run")
	}
	if !strings.Contains(view, "esc skip · /connect anytime") {
		t.Errorf("expected first-run footer, got %q", view)
	}

	d.Hide()
	d.Show([]ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", ProviderID: "ollama", Name: "Qwen"}}},
	})
	view = d.View()
	if !strings.Contains(view, "Select Provider") {
		t.Error("expected normal title after Show")
	}
	if d.firstRunHint {
		t.Error("firstRunHint should be cleared by Show")
	}
}

func TestModelDialog_ViewProvidersEmpty_ShowsSetupGuidance(t *testing.T) {
	d := NewModelDialog()
	d.Show(nil)
	d.SetSize(80, 24)

	view := d.View()
	if !strings.Contains(view, "ollama serve") {
		t.Error("expected Ollama serve guidance in empty providers view")
	}
	if !strings.Contains(view, "ollama pull") {
		t.Error("expected Ollama pull guidance in empty providers view")
	}
	if !strings.Contains(view, "LM Studio") {
		t.Error("expected LM Studio guidance in empty providers view")
	}
	if !strings.Contains(view, "localhost:1234") {
		t.Error("expected LM Studio localhost:1234 in empty providers view")
	}
	if !strings.Contains(view, "TINYCODE_LMSTUDIO_HOST") {
		t.Error("expected TINYCODE_LMSTUDIO_HOST hint in empty providers view")
	}
	if !strings.Contains(view, "vLLM") {
		t.Error("expected vLLM guidance in empty providers view")
	}
	if !strings.Contains(view, "localhost:8000") {
		t.Error("expected vLLM localhost:8000 in empty providers view")
	}
	if !strings.Contains(view, "TINYCODE_VLLM_HOST") {
		t.Error("expected TINYCODE_VLLM_HOST hint in empty providers view")
	}
	if !strings.Contains(view, "OpenRouter") {
		t.Error("expected OpenRouter guidance in empty providers view")
	}
	if !strings.Contains(view, "rescan") {
		t.Error("expected rescan hint in empty providers view")
	}
	if !strings.Contains(view, "tinycode doctor") {
		t.Error("expected tinycode doctor hint in empty providers view")
	}
}

func TestModelDialog_FirstRun_ModelPhaseShowsEscBack(t *testing.T) {
	d := NewModelDialog()
	d.ShowFirstRun([]ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", Name: "Qwen"}}},
	})
	d.phase = phaseModels
	d.SetSize(80, 24)
	view := d.View()
	if strings.Contains(view, "esc skip") {
		t.Error("model phase must not show esc skip")
	}
	if !strings.Contains(view, "esc back") {
		t.Error("expected esc back on model phase")
	}
}

func TestModelDialog_EmptyProviders_RTriggersRescan(t *testing.T) {
	d := NewModelDialog()
	d.Show(nil)

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected ProvidersRefreshMsg cmd from r")
	}
	msg := cmd()
	if _, ok := msg.(ProvidersRefreshMsg); !ok {
		t.Fatalf("expected ProvidersRefreshMsg, got %T", msg)
	}
}

func TestModelDialog_EmptyModels_ShowsPullAndRescan(t *testing.T) {
	d := NewModelDialog()
	d.Show([]ProviderInfo{{ID: "ollama", Name: "Ollama", Models: nil}})
	d.phase = phaseModels
	d.SetSize(80, 24)

	view := d.View()
	if !strings.Contains(view, "No models") {
		t.Error("expected empty models copy")
	}
	if !strings.Contains(view, "ollama pull") {
		t.Error("expected ollama pull hint")
	}
	if !strings.Contains(view, "rescan") {
		t.Error("expected rescan hint")
	}
}

func TestModelDialog_RefreshProviders_UpdatesInPlace(t *testing.T) {
	d := NewModelDialog()
	d.Show(nil)
	d.RefreshProviders([]ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", Name: "Qwen"}}},
	})
	if len(d.providers) != 1 {
		t.Fatalf("providers = %d, want 1 after refresh", len(d.providers))
	}
}

func TestModelDialog_EmptyProviders_OEntersAPIKeyPhase(t *testing.T) {
	d := NewModelDialog()
	d.Show(nil)

	updated, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if updated.phase != phaseAPIKey {
		t.Fatalf("expected phaseAPIKey after 'o', got %v", updated.phase)
	}

	view := updated.View()
	if !strings.Contains(view, "OpenRouter API Key") {
		t.Error("expected API key entry view")
	}
}

func TestModelDialog_EscHidesFromProviders(t *testing.T) {
	d := NewModelDialog()
	d.Show([]ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", ProviderID: "ollama", Name: "Qwen"}}},
	})
	if !d.IsVisible() {
		t.Fatal("expected dialog visible after Show")
	}

	updated, _ := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.IsVisible() {
		t.Error("expected Escape to hide model dialog from providers phase")
	}
}

func TestModelDialog_APIKeyEnter_EmitsStoreMsg(t *testing.T) {
	d := NewModelDialog()
	d.Show(nil)
	d.phase = phaseAPIKey
	d.apiKey = "sk-test-key"

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd on Enter with API key")
	}
	msg := cmd()
	store, ok := msg.(StoreOpenRouterAuthMsg)
	if !ok {
		t.Fatalf("expected StoreOpenRouterAuthMsg, got %T", msg)
	}
	if store.APIKey != "sk-test-key" {
		t.Errorf("expected sk-test-key, got %q", store.APIKey)
	}
}

func TestEnsureScrollVisible_Table(t *testing.T) {
	tests := []struct {
		name                            string
		selected, scroll, maxVis, total int
		want                            int
	}{
		{"selected above scroll", 2, 5, 10, 20, 2},
		{"selected below visible", 15, 0, 10, 20, 6},
		{"already visible", 5, 3, 10, 20, 3},
		{"scroll past end", 19, 19, 10, 20, 10},
		{"negative scroll", 0, -5, 10, 20, 0},
		{"zero total", 0, 0, 10, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureScrollVisible(tt.selected, tt.scroll, tt.maxVis, tt.total)
			if got != tt.want {
				t.Errorf("ensureScrollVisible(%d,%d,%d,%d) = %d, want %d",
					tt.selected, tt.scroll, tt.maxVis, tt.total, got, tt.want)
			}
		})
	}
}

func TestWrapIndex_Table(t *testing.T) {
	tests := []struct {
		i, length, want int
	}{
		{0, 5, 0},
		{4, 5, 4},
		{5, 5, 0},
		{-1, 5, 4},
		{-5, 5, 0},
		{7, 3, 1},
		{0, 0, 0},
	}
	for _, tt := range tests {
		got := wrapIndex(tt.i, tt.length)
		if got != tt.want {
			t.Errorf("wrapIndex(%d, %d) = %d, want %d", tt.i, tt.length, got, tt.want)
		}
	}
}

func TestMaxVisibleProviders_Small(t *testing.T) {
	d := NewModelDialog()
	d.height = 10
	if got := d.maxVisibleProviders(); got != 3 {
		t.Errorf("expected 3 (clamped), got %d", got)
	}
}

func TestMaxVisibleProviders_Large(t *testing.T) {
	d := NewModelDialog()
	d.height = 30
	got := d.maxVisibleProviders()
	if got != 22 {
		t.Errorf("expected 22, got %d", got)
	}
}

func TestItoa_Table(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{100, "100"},
		{9999, "9999"},
	}
	for _, tt := range tests {
		got := itoa(tt.n)
		if got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
