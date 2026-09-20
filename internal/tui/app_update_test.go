package tui

import "testing"

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
