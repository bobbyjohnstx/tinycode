package tui

import (
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

func TestHandleProvidersLoadedMsg_SetsCurrentModel(t *testing.T) {
	app := NewApp("")

	providers := []ProviderInfo{
		{
			ID:   "test-provider",
			Name: "Test Provider",
			Models: []ModelInfo{
				{ID: "test-model", ProviderID: "test-provider", Name: "Test Model 7B"},
			},
		},
	}

	msg := ProvidersLoadedMsg{
		Providers:       providers,
		DefaultProvider: "test-provider",
		DefaultModel:    "test-model",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if result.state.CurrentModel.ProviderID != "test-provider" {
		t.Errorf("expected CurrentModel.ProviderID 'test-provider', got %q", result.state.CurrentModel.ProviderID)
	}
	if result.state.CurrentModel.ModelID != "test-model" {
		t.Errorf("expected CurrentModel.ModelID 'test-model', got %q", result.state.CurrentModel.ModelID)
	}
}

func TestHandleProvidersLoadedMsg_SkipsWhenModelAlreadySet(t *testing.T) {
	app := NewApp("")
	app.state.CurrentModel = ModelSelection{
		ProviderID: "existing-provider",
		ModelID:    "existing-model",
	}

	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{
			{ID: "new-provider", Name: "New", Models: []ModelInfo{
				{ID: "new-model", ProviderID: "new-provider", Name: "New Model"},
			}},
		},
		DefaultProvider: "new-provider",
		DefaultModel:    "new-model",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	// Should NOT override the existing model selection.
	if result.state.CurrentModel.ProviderID != "existing-provider" {
		t.Errorf("expected existing provider to be preserved, got %q", result.state.CurrentModel.ProviderID)
	}
	if result.state.CurrentModel.ModelID != "existing-model" {
		t.Errorf("expected existing model to be preserved, got %q", result.state.CurrentModel.ModelID)
	}
}

func TestHandleProvidersLoadedMsg_StoresProviderList(t *testing.T) {
	app := NewApp("")

	providers := []ProviderInfo{
		{ID: "ollama", Name: "Ollama", Models: []ModelInfo{
			{ID: "llama3.2", ProviderID: "ollama", Name: "Llama 3.2"},
		}},
		{ID: "lm-studio", Name: "LM Studio", Models: []ModelInfo{
			{ID: "phi-4", ProviderID: "lm-studio", Name: "Phi 4"},
		}},
	}

	msg := ProvidersLoadedMsg{
		Providers:       providers,
		DefaultProvider: "ollama",
		DefaultModel:    "llama3.2",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if len(result.state.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result.state.Providers))
	}
	if result.state.Providers[0].ID != "ollama" {
		t.Errorf("expected first provider 'ollama', got %q", result.state.Providers[0].ID)
	}
}

func TestHandleProvidersLoadedMsg_EmptyDefaultsLeavesModelUnset(t *testing.T) {
	app := NewApp("")

	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{
			{ID: "test", Name: "Test"},
		},
		DefaultProvider: "",
		DefaultModel:    "",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if result.state.CurrentModel.ModelID != "" {
		t.Errorf("expected empty model when defaults are empty, got %q", result.state.CurrentModel.ModelID)
	}
	if result.state.CurrentModel.ProviderID != "" {
		t.Errorf("expected empty provider when defaults are empty, got %q", result.state.CurrentModel.ProviderID)
	}
}

func TestHandleProvidersLoadedMsg_AutoOpensConnectWhenNoModel(t *testing.T) {
	app := NewApp("")
	msg := ProvidersLoadedMsg{
		Providers:       []ProviderInfo{{ID: "ollama", Name: "Ollama"}},
		DefaultProvider: "",
		DefaultModel:    "",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)

	if !result.modelDlg.IsVisible() {
		t.Fatal("expected connect dialog to auto-open when no model")
	}
	if result.focus != FocusDialog {
		t.Errorf("focus = %v, want FocusDialog", result.focus)
	}
	if !result.state.FirstRunConnectOffered {
		t.Error("expected FirstRunConnectOffered after auto-open")
	}
}

func TestHandleProvidersLoadedMsg_AutoOpenOnceOnly(t *testing.T) {
	app := NewApp("")
	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{{ID: "ollama", Name: "Ollama"}},
	}
	result, _ := app.handleProvidersLoadedMsg(msg)
	result.modelDlg.Hide()
	result.setFocus(FocusPrompt)

	result, _ = result.handleProvidersLoadedMsg(msg)
	if result.modelDlg.IsVisible() {
		t.Fatal("expected connect dialog not to re-open after first offer")
	}
}

func TestHandleProvidersLoadedMsg_NoAutoOpenWhenModelSet(t *testing.T) {
	app := NewApp("")
	msg := ProvidersLoadedMsg{
		Providers: []ProviderInfo{
			{ID: "ollama", Name: "Ollama", Models: []ModelInfo{{ID: "qwen", Name: "Qwen", ProviderID: "ollama"}}},
		},
		DefaultProvider: "ollama",
		DefaultModel:    "qwen",
	}

	result, _ := app.handleProvidersLoadedMsg(msg)
	if result.modelDlg.IsVisible() {
		t.Fatal("expected no auto-open when default model resolved")
	}
	if result.state.FirstRunConnectOffered {
		t.Error("FirstRunConnectOffered should stay false when model was set")
	}
}

func TestHandleProvidersLoadedMsg_AutoOpensWithEmptyProviders(t *testing.T) {
	app := NewApp("")
	result, _ := app.handleProvidersLoadedMsg(ProvidersLoadedMsg{})
	if !result.modelDlg.IsVisible() {
		t.Fatal("expected connect dialog even with empty provider list")
	}
}

func TestPaletteSelected_SkillExpandsIntoPrompt(t *testing.T) {
	app := NewApp("")
	app.state.Commands = []api.CommandInfo{
		{Name: "debug", Description: "Debug skill", Source: "skill"},
	}

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "debug", Value: "debug"},
	})
	if !handled {
		t.Fatal("expected palette selection handled")
	}
	val := result.prompt.Value()
	if val == "" || val == "debug" || val == "/debug" {
		t.Fatalf("expected expanded skill body in prompt, got %q", val)
	}
	if strings.HasPrefix(strings.TrimSpace(val), "---") {
		t.Error("prompt should not contain skill frontmatter")
	}
	if !strings.Contains(val, "Debug") && !strings.Contains(val, "root cause") {
		snippet := val
		if len(snippet) > 120 {
			snippet = snippet[:120]
		}
		t.Errorf("expected debug skill body content, got %q", snippet)
	}
}

func TestPaletteSelected_NonSkillNoPromptInject(t *testing.T) {
	app := NewApp("")
	app.state.Commands = []api.CommandInfo{
		{Name: "init", Description: "Init", Source: "builtin"},
	}
	app.prompt.SetValue("keep-me")

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "init", Value: "init"},
	})
	if !handled {
		t.Fatal("expected handled")
	}
	if result.prompt.Value() != "keep-me" {
		t.Errorf("non-skill palette select should not rewrite prompt, got %q", result.prompt.Value())
	}
}
