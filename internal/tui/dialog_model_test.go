package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
	if !strings.Contains(view, "OpenRouter") {
		t.Error("expected OpenRouter guidance in empty providers view")
	}
	if !strings.Contains(view, "/connect") {
		t.Error("expected /connect reopen hint in empty providers view")
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
