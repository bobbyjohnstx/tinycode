package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// lmStudioNativeModel is one entry from GET /api/v0/models.
// OpenAI-compat /v1/models omits context; the native API reports both the
// architectural max and the currently loaded window.
type lmStudioNativeModel struct {
	ID                  string   `json:"id"`
	Type                string   `json:"type"`
	State               string   `json:"state"`
	MaxContextLength    int      `json:"max_context_length"`
	LoadedContextLength int      `json:"loaded_context_length"`
	Capabilities        []string `json:"capabilities"`
}

type lmStudioNativeModelsResponse struct {
	Data []lmStudioNativeModel `json:"data"`
}

func lmStudioContextLen(m lmStudioNativeModel) int {
	// Prefer the served window when the model is loaded — max_context_length
	// is only the architectural ceiling (often much larger than what fits).
	if m.LoadedContextLength > 0 {
		return m.LoadedContextLength
	}
	if m.MaxContextLength > 0 {
		return m.MaxContextLength
	}
	return 8192
}

func lmStudioOutputLen(contextLen int) int {
	outputLen := 4096
	if outputLen >= contextLen {
		outputLen = contextLen / 2
		if outputLen < 1 {
			outputLen = contextLen
		}
	}
	return outputLen
}

func lmStudioToolCall(caps []string) bool {
	if len(caps) == 0 {
		return true // OpenAI-compat path / older LM Studio: assume tools OK
	}
	for _, c := range caps {
		if c == "tool_use" {
			return true
		}
	}
	return false
}

func (d *Discovery) discoverLMStudio(ctx context.Context, baseURL string) {
	if !d.beginProbe("lm-studio") {
		return
	}
	defer d.endProbe("lm-studio")

	base := strings.TrimRight(baseURL, "/")
	models, err := d.fetchLMStudioNativeModels(ctx, base)
	if err != nil {
		slog.Debug("lm-studio native models unavailable, falling back to /v1/models", "error", err)
		models, err = d.fetchLMStudioCompatModels(ctx, base)
	}
	if err != nil {
		d.handleDiscoveryFailure("lm-studio", "LM Studio", err)
		return
	}

	d.diffModels("lm-studio", models)
	d.handleDiscoverySuccess("lm-studio")

	d.registry.Register(&Info{
		ID:      "lm-studio",
		Name:    "LM Studio",
		Source:  "custom",
		Env:     []string{},
		Options: map[string]any{},
		Models:  models,
	})

	d.bus.Publish("provider.discovered", map[string]any{
		"providerID": "lm-studio",
		"modelCount": len(models),
	})
}

func (d *Discovery) fetchLMStudioNativeModels(ctx context.Context, baseURL string) (map[string]*Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v0/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body lmStudioNativeModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	models := make(map[string]*Model, len(body.Data))
	for _, m := range body.Data {
		if m.ID == "" || m.Type == "embeddings" || m.Type == "embedding" {
			continue
		}
		contextLen := lmStudioContextLen(m)
		models[m.ID] = &Model{
			ID:         m.ID,
			ProviderID: "lm-studio",
			Name:       m.ID,
			API: ModelAPI{
				ID:  m.ID,
				URL: baseURL,
			},
			Status:  "active",
			Headers: make(map[string]string),
			Options: make(map[string]any),
			Limit:   ModelLimit{Context: contextLen, Output: lmStudioOutputLen(contextLen)},
			Capabilities: ModelCaps{
				Temperature: true,
				ToolCall:    lmStudioToolCall(m.Capabilities),
				Input:       ModalityCaps{Text: true},
				Output:      ModalityCaps{Text: true},
			},
		}
	}
	return models, nil
}

func (d *Discovery) fetchLMStudioCompatModels(ctx context.Context, baseURL string) (map[string]*Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body vllmModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	models := make(map[string]*Model, len(body.Data))
	for _, m := range body.Data {
		if m.ID == "" {
			continue
		}
		contextLen := m.MaxModelLen
		if contextLen <= 0 {
			contextLen = 8192
		}
		models[m.ID] = &Model{
			ID:         m.ID,
			ProviderID: "lm-studio",
			Name:       m.ID,
			API: ModelAPI{
				ID:  m.ID,
				URL: baseURL,
			},
			Status:  "active",
			Headers: make(map[string]string),
			Options: make(map[string]any),
			Limit:   ModelLimit{Context: contextLen, Output: lmStudioOutputLen(contextLen)},
			Capabilities: ModelCaps{
				Temperature: true,
				ToolCall:    true,
				Input:       ModalityCaps{Text: true},
				Output:      ModalityCaps{Text: true},
			},
		}
	}
	return models, nil
}
