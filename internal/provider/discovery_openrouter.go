package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DiscoverOpenRouter queries the OpenRouter API for available models.
// openRouterModelsURL is overridable in tests via SetOpenRouterModelsURLForTest.
var openRouterModelsURL = "https://openrouter.ai/api/v1/models"

// SetOpenRouterModelsURLForTest overrides the OpenRouter models endpoint URL.
// Returns the previous URL so callers can restore it.
func SetOpenRouterModelsURLForTest(url string) string {
	prev := openRouterModelsURL
	openRouterModelsURL = url
	return prev
}

func (d *Discovery) DiscoverOpenRouter(ctx context.Context, apiKey string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", openRouterModelsURL, nil)
	if err != nil {
		return fmt.Errorf("creating openrouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("openrouter discovery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openrouter returned status %d", resp.StatusCode)
	}

	var body openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("parsing openrouter response: %w", err)
	}

	models := make(map[string]*Model, len(body.Data))
	for _, entry := range body.Data {
		models[entry.ID] = buildOpenRouterModel(entry)
	}

	d.registry.Register(&Info{
		ID:      "openrouter",
		Name:    "OpenRouter",
		Source:  "custom",
		Env:     []string{"OPENROUTER_API_KEY"},
		Options: map[string]any{"apiKey": apiKey},
		Models:  models,
	})

	d.bus.Publish("provider.discovered", map[string]any{
		"providerID": "openrouter",
		"modelCount": len(models),
	})

	return nil
}

// buildOpenRouterModel constructs a Model from a single OpenRouter API entry.
func buildOpenRouterModel(entry openRouterModel) *Model {
	params := make(map[string]bool)
	for _, p := range entry.SupportedParameters {
		params[p] = true
	}

	contextLen := entry.ContextLength
	if entry.TopProvider != nil && entry.TopProvider.ContextLength > 0 {
		contextLen = entry.TopProvider.ContextLength
	}

	maxOutput := 0
	if entry.TopProvider != nil && entry.TopProvider.MaxCompletionTokens != nil {
		maxOutput = *entry.TopProvider.MaxCompletionTokens
	}
	if maxOutput == 0 {
		maxOutput = min(16384, contextLen/5)
	}

	var promptCost, completionCost float64
	if entry.Pricing != nil {
		if v, err := parseFloat(entry.Pricing.Prompt); err == nil {
			promptCost = v * 1_000_000
		}
		if v, err := parseFloat(entry.Pricing.Completion); err == nil {
			completionCost = v * 1_000_000
		}
	}

	inputMods := make(map[string]bool)
	if entry.Architecture != nil {
		for _, m := range entry.Architecture.InputModalities {
			inputMods[m] = true
		}
	}
	if len(inputMods) == 0 {
		inputMods["text"] = true
	}

	hasReasoning := false
	if entry.Reasoning != nil {
		hasReasoning = entry.Reasoning.Mandatory || entry.Reasoning.DefaultEnabled
	}

	family := ""
	if parts := strings.SplitN(entry.ID, "/", 2); len(parts) == 2 {
		family = parts[0]
	}

	return &Model{
		ID:         entry.ID,
		ProviderID: "openrouter",
		Name:       entry.Name,
		Family:     family,
		API: ModelAPI{
			ID:  entry.ID,
			URL: "https://openrouter.ai/api",
		},
		Status:  "active",
		Headers: make(map[string]string),
		Options: make(map[string]any),
		Cost:    ModelCost{Input: promptCost, Output: completionCost},
		Limit:   ModelLimit{Context: contextLen, Output: maxOutput},
		Capabilities: ModelCaps{
			Temperature: params["temperature"],
			Reasoning:   hasReasoning,
			Attachment:  inputMods["image"] || inputMods["file"],
			ToolCall:    params["tools"],
			Input: ModalityCaps{
				Text:  inputMods["text"],
				Audio: inputMods["audio"],
				Image: inputMods["image"],
				Video: inputMods["video"],
				PDF:   inputMods["file"],
			},
			Output: ModalityCaps{Text: true},
		},
	}
}

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	ContextLength       int                     `json:"context_length"`
	Architecture        *openRouterArchitecture `json:"architecture,omitempty"`
	Pricing             *openRouterPricing      `json:"pricing,omitempty"`
	TopProvider         *openRouterTopProvider  `json:"top_provider,omitempty"`
	SupportedParameters []string                `json:"supported_parameters,omitempty"`
	Reasoning           *openRouterReasoning    `json:"reasoning,omitempty"`
}

type openRouterArchitecture struct {
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
}

type openRouterPricing struct {
	Prompt     string `json:"prompt,omitempty"`
	Completion string `json:"completion,omitempty"`
}

type openRouterTopProvider struct {
	ContextLength       int  `json:"context_length,omitempty"`
	MaxCompletionTokens *int `json:"max_completion_tokens,omitempty"`
}

type openRouterReasoning struct {
	Mandatory      bool `json:"mandatory,omitempty"`
	DefaultEnabled bool `json:"default_enabled,omitempty"`
}

func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}
