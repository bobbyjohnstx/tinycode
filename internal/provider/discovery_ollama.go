package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

type ollamaModel struct {
	Name         string              `json:"name"`
	Details      *ollamaModelDetails `json:"details,omitempty"`
	Capabilities []string            `json:"capabilities,omitempty"`
}

type ollamaModelDetails struct {
	ParameterSize string `json:"parameter_size,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	Family        string `json:"family,omitempty"`
}

func (d *Discovery) discoverOllama(ctx context.Context, baseURL string) {
	if !d.beginProbe("ollama") {
		return
	}
	defer d.endProbe("ollama")

	url := strings.TrimRight(baseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.handleDiscoveryFailure("ollama", "Ollama", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.handleDiscoveryFailure("ollama", "Ollama", fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		d.handleDiscoveryFailure("ollama", "Ollama", err)
		return
	}

	models := d.buildOllamaModels(ctx, baseURL, tags)

	d.diffModels("ollama", models)
	d.handleDiscoverySuccess("ollama")
	d.registerOllamaProvider(models)
}

// buildOllamaModels converts the Ollama tags response into a model map,
// handling profile creation and stale profile cleanup.
func (d *Discovery) buildOllamaModels(ctx context.Context, baseURL string, tags ollamaTagsResponse) map[string]*Model {
	existingProfiles := make(map[string]string)
	var baseModels []ollamaModel
	for _, m := range tags.Models {
		if IsProfile(m.Name) {
			existingProfiles[BaseModelName(m.Name)] = m.Name
		} else {
			baseModels = append(baseModels, m)
		}
	}

	models := make(map[string]*Model, len(baseModels))
	for _, m := range baseModels {
		apiID, profileCtx, caps, family := d.buildSingleOllamaModel(ctx, baseURL, m, existingProfiles)
		models[m.Name] = &Model{
			ID:         m.Name,
			ProviderID: "ollama",
			Name:       m.Name,
			Family:     family,
			API: ModelAPI{
				ID:  apiID,
				URL: baseURL,
			},
			Status:       "active",
			Headers:      make(map[string]string),
			Options:      make(map[string]any),
			Cost:         ModelCost{},
			Limit:        ModelLimit{Context: profileCtx, Output: profileCtx / 2},
			Capabilities: caps,
		}
	}

	// Clean up stale profiles (profile exists but base model is gone)
	if d.autoProfileEnabled() {
		for baseName, profName := range existingProfiles {
			if _, exists := models[baseName]; exists {
				continue
			}
			if err := DeleteModel(ctx, d.ollamaClient, baseURL, profName); err != nil {
				slog.Warn("failed to delete stale profile",
					"profile", profName, "error", err)
			} else {
				slog.Info("deleted stale ollama profile", "profile", profName)
			}
		}
	}

	return models
}

// buildSingleOllamaModel resolves capabilities, context length, and auto-profiling
// for a single Ollama model. It may mutate existingProfiles to track consumed profiles.
func (d *Discovery) buildSingleOllamaModel(ctx context.Context, baseURL string, m ollamaModel, existingProfiles map[string]string) (apiID string, profileCtx int, caps ModelCaps, family string) {
	// Raw tags context (0 when omitted); used so ShowModel can supply advertisedCtx.
	tagsCtx := 0
	if m.Details != nil {
		tagsCtx = m.Details.ContextLength
		family = m.Details.Family
	}
	contextLen := tagsCtx
	if contextLen == 0 {
		contextLen = 8192
	}

	caps = ModelCaps{
		Temperature: true,
		ToolCall:    true,
		Input:       ModalityCaps{Text: true},
		Output:      ModalityCaps{Text: true},
	}
	for _, c := range m.Capabilities {
		if c == "vision" {
			caps.Input.Image = true
			caps.Attachment = true
		}
	}

	apiID = m.Name
	profileCtx = contextLen

	if !d.autoProfileEnabled() || d.isModelSkipped(m.Name) {
		return
	}
	numCtx := d.resolveNumCtx(ctx, baseURL, m.Name, tagsCtx)
	if numCtx < minNumCtx {
		return
	}
	profName := ProfileName(m.Name, numCtx)
	if existing, ok := existingProfiles[m.Name]; ok && existing == profName {
		apiID = profName
		profileCtx = numCtx
		delete(existingProfiles, m.Name)
		return
	}
	// Delete superseded profile when num_ctx (and thus profile name) changed.
	if existing, ok := existingProfiles[m.Name]; ok && existing != profName {
		if err := DeleteModel(ctx, d.ollamaClient, baseURL, existing); err != nil {
			slog.Warn("failed to delete superseded ollama profile",
				"old_profile", existing, "new_profile", profName, "error", err)
		} else {
			slog.Info("deleted superseded ollama profile",
				"old_profile", existing, "new_profile", profName)
		}
		delete(existingProfiles, m.Name)
	}
	if err := CreateProfile(ctx, d.ollamaClient, baseURL, m.Name, profName, numCtx); err != nil {
		slog.Warn("failed to create ollama profile",
			"model", m.Name, "profile", profName, "error", err)
		return
	}
	slog.Info("created ollama profile",
		"model", m.Name, "profile", profName, "num_ctx", numCtx)
	apiID = profName
	profileCtx = numCtx
	delete(existingProfiles, m.Name)

	return
}

// registerOllamaProvider registers the discovered Ollama models with the
// registry. Tool-call probes wait until a model is selected.
func (d *Discovery) registerOllamaProvider(models map[string]*Model) {
	d.registry.Register(&Info{
		ID:      "ollama",
		Name:    "Ollama",
		Source:  "custom",
		Env:     []string{},
		Options: map[string]any{},
		Models:  models,
	})

	d.bus.Publish("provider.discovered", map[string]any{
		"providerID": "ollama",
		"modelCount": len(models),
	})
}

// Warmup probes tool-call support for a model the user has selected.
// Only Ollama models are probed, and each model is probed once.
func (d *Discovery) Warmup(ctx context.Context, m *Model) {
	if d == nil || m == nil || m.ProviderID != "ollama" {
		return
	}
	d.maybeWarmup(ctx, m)
}

// autoProfileEnabled returns true if auto-profiling is configured and not disabled.
func (d *Discovery) autoProfileEnabled() bool {
	if d.autoProfile == nil {
		return false
	}
	if d.autoProfile.Enabled != nil && !*d.autoProfile.Enabled {
		return false
	}
	return true
}

// isModelSkipped returns true if the model is marked as skip in the auto-profile config.
func (d *Discovery) isModelSkipped(modelName string) bool {
	if d.autoProfile == nil || d.autoProfile.Models == nil {
		return false
	}
	m, ok := d.autoProfile.Models[modelName]
	return ok && m.Skip
}

// resolveNumCtx determines the optimal num_ctx for a model, using config
// overrides, GPU detection, and the CalculateNumCtx algorithm.
// advertisedCtx is the tags context_length (0 when omitted).
func (d *Discovery) resolveNumCtx(ctx context.Context, baseURL, modelName string, advertisedCtx int) int {
	// Check per-model override
	if d.autoProfile != nil && d.autoProfile.Models != nil {
		if m, ok := d.autoProfile.Models[modelName]; ok && m.NumCtx != nil {
			return *m.NumCtx
		}
	}

	// Check default override
	if d.autoProfile != nil && d.autoProfile.DefaultNumCtx != nil {
		return *d.autoProfile.DefaultNumCtx
	}

	// Detect GPU memory (cached)
	d.gpuOnce.Do(func() {
		mem, err := d.detectGPU()
		if err != nil {
			slog.Warn("GPU memory detection failed, auto-profiling will use defaults", "error", err)
			return
		}
		d.gpuMemory = mem
		slog.Info("detected GPU memory", "bytes", mem, "gb", fmt.Sprintf("%.1f", float64(mem)/(1024*1024*1024)))
	})

	// Query model details — also supplies ContextLength when tags omit it.
	info, err := ShowModel(ctx, d.ollamaClient, baseURL, modelName)
	if err != nil {
		slog.Warn("failed to query model info for auto-profiling",
			"model", modelName, "error", err)
		if advertisedCtx <= 0 {
			return 8192
		}
		return advertisedCtx
	}
	if advertisedCtx <= 0 {
		if info.ContextLength > 0 {
			advertisedCtx = info.ContextLength
		} else {
			advertisedCtx = 8192
		}
	}

	if d.gpuMemory <= 0 {
		return advertisedCtx
	}

	numCtx := CalculateNumCtx(d.gpuMemory, *info, advertisedCtx)

	// Apply max cap from config
	if d.autoProfile != nil && d.autoProfile.MaxNumCtx != nil && numCtx > *d.autoProfile.MaxNumCtx {
		numCtx = *d.autoProfile.MaxNumCtx
	}

	return numCtx
}

// warmupClient returns an HTTP client whose timeout covers model load.
// The discovery client times out in 2s, which aborts a probe while Ollama
// is still loading weights.
func (d *Discovery) warmupClient() *http.Client {
	if d.client == nil || (d.client.Timeout > 0 && d.client.Timeout < warmupTimeout) {
		clone := http.Client{Timeout: warmupTimeout}
		if d.client != nil {
			clone = *d.client
			clone.Timeout = warmupTimeout
		}
		return &clone
	}
	return d.client
}

// maybeWarmup triggers a background warmup probe for a model if it hasn't
// been warmed up yet. Transient probe failures leave the model unmarked so
// the next Warmup call retries. ToolCall is only set false after a definitive
// non-capable response or after maxWarmupFailures consecutive errors.
func (d *Discovery) maybeWarmup(ctx context.Context, m *Model) {
	d.warmedMu.Lock()
	if d.warmedModels[m.ID] || d.warmingModels[m.ID] {
		d.warmedMu.Unlock()
		return
	}
	d.warmingModels[m.ID] = true
	d.warmedMu.Unlock()

	safego.Go(func() {
		defer func() {
			d.warmedMu.Lock()
			delete(d.warmingModels, m.ID)
			d.warmedMu.Unlock()
		}()

		capable, err := WarmupProbe(ctx, d.warmupClient(), m.API.URL, m.API.ID)
		if err != nil {
			slog.Warn("warmup probe failed", "model", m.ID, "error", err)
			d.warmedMu.Lock()
			d.warmupFails[m.ID]++
			fails := d.warmupFails[m.ID]
			d.warmedMu.Unlock()
			if fails < maxWarmupFailures {
				// Leave unmarked so the next Warmup retries.
				return
			}
			capable = false
		}

		d.warmedMu.Lock()
		d.warmedModels[m.ID] = true
		delete(d.warmupFails, m.ID)
		d.warmedMu.Unlock()

		if !capable {
			d.registry.UpdateCapability(m.ProviderID, m.ID, "ToolCall", false)
			slog.Info("model does not support tool calls", "model", m.ID)
		}

		d.bus.Publish("provider.warmup.complete", map[string]any{
			"modelID":     m.ID,
			"toolCapable": capable,
		})
	})
}
