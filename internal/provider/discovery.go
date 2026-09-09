package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
)

const (
	probeTimeout            = 2 * time.Second
	pollInterval            = 30 * time.Second
	maxConsecutiveFailures  = 3
)

type Discovery struct {
	registry    *Registry
	bus         *bus.Bus
	client      *http.Client
	cancel      context.CancelFunc
	autoProfile *AutoProfileConfig
	detectGPU   func() (int64, error)
	gpuMemory   int64
	gpuOnce     sync.Once
	warmedMu    sync.Mutex
	warmedModels map[string]bool
	dormantMu   sync.Mutex
	dormant     map[string]bool // providers removed after consecutive failures
}

func NewDiscovery(registry *Registry, b *bus.Bus) *Discovery {
	return &Discovery{
		registry:     registry,
		bus:          b,
		detectGPU:    DetectGPUMemory,
		warmedModels: make(map[string]bool),
		dormant:      make(map[string]bool),
		client: &http.Client{
			Timeout: probeTimeout,
		},
	}
}

// SetAutoProfile configures GPU-aware auto-profiling for Ollama models.
func (d *Discovery) SetAutoProfile(cfg *AutoProfileConfig) {
	d.autoProfile = cfg
}

// Start begins background polling for local providers.
// The first poll runs synchronously so providers are available before the
// server starts accepting requests.
func (d *Discovery) Start(ctx context.Context, ollamaURL, vllmURL, lmStudioURL string) {
	ctx, d.cancel = context.WithCancel(ctx)

	d.poll(ctx, ollamaURL, vllmURL, lmStudioURL)

	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.poll(ctx, ollamaURL, vllmURL, lmStudioURL)
			}
		}
	}()
}

// Stop halts the discovery polling.
func (d *Discovery) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
}

func (d *Discovery) poll(ctx context.Context, ollamaURL, vllmURL, lmStudioURL string) {
	if ollamaURL != "" && d.shouldPoll("ollama") {
		d.discoverOllama(ctx, ollamaURL)
	}
	if vllmURL != "" && d.shouldPoll("vllm") {
		d.discoverVLLM(ctx, vllmURL)
	}
	if lmStudioURL != "" && d.shouldPoll("lm-studio") {
		d.discoverLMStudio(ctx, lmStudioURL)
	}
}

func (d *Discovery) shouldPoll(providerID string) bool {
	d.dormantMu.Lock()
	defer d.dormantMu.Unlock()
	return !d.dormant[providerID]
}

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

// handleDiscoveryFailure records a failure for a provider and removes it
// after maxConsecutiveFailures consecutive failures.
func (d *Discovery) handleDiscoveryFailure(providerID, providerName string, err error) {
	count := d.registry.RecordFailure(providerID)
	slog.Warn("provider discovery failed",
		"provider", providerID,
		"error", err,
		"consecutive_failures", count,
	)
	if count >= maxConsecutiveFailures {
		d.registry.Remove(providerID)
		d.dormantMu.Lock()
		d.dormant[providerID] = true
		d.dormantMu.Unlock()
		d.bus.Publish("provider.removed", map[string]any{
			"providerID":   providerID,
			"providerName": providerName,
			"reason":       "consecutive_failures",
			"failures":     count,
		})
		slog.Warn("provider removed after consecutive failures, polling suspended",
			"provider", providerID,
			"failures", count,
		)
	}
}

// handleDiscoverySuccess resets the failure counter and publishes a
// reconnected event if the provider was previously failing.
func (d *Discovery) handleDiscoverySuccess(providerID string) {
	d.dormantMu.Lock()
	wasDormant := d.dormant[providerID]
	delete(d.dormant, providerID)
	d.dormantMu.Unlock()

	prev := d.registry.ResetFailures(providerID)
	if prev > 0 || wasDormant {
		d.bus.Publish("provider.reconnected", map[string]any{
			"providerID":        providerID,
			"previous_failures": prev,
		})
		slog.Info("provider reconnected after failures",
			"provider", providerID,
			"previous_failures", prev,
		)
	}
}

// diffModels logs models added and removed compared to the existing
// registration for a provider.
func (d *Discovery) diffModels(providerID string, newModels map[string]*Model) {
	existing, err := d.registry.GetProvider(providerID)
	if err != nil {
		return // provider not yet registered, no diff needed
	}

	for id := range newModels {
		if _, ok := existing.Models[id]; !ok {
			slog.Info("model added", "provider", providerID, "model", id)
		}
	}
	for id := range existing.Models {
		if _, ok := newModels[id]; !ok {
			slog.Info("model removed", "provider", providerID, "model", id)
		}
	}
}

func (d *Discovery) discoverOllama(ctx context.Context, baseURL string) {
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

	// Separate base models from existing profiles
	existingProfiles := make(map[string]string) // base name -> profile name
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
		contextLen := 0
		family := ""
		if m.Details != nil {
			contextLen = m.Details.ContextLength
			family = m.Details.Family
		}
		if contextLen == 0 {
			contextLen = 8192
		}

		caps := ModelCaps{
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

		apiID := m.Name
		profileCtx := contextLen

		if d.autoProfileEnabled() && !d.isModelSkipped(m.Name) {
			numCtx := d.resolveNumCtx(ctx, baseURL, m.Name, contextLen)
			if numCtx >= minNumCtx {
				profName := ProfileName(m.Name, numCtx)
				if existing, ok := existingProfiles[m.Name]; ok && existing == profName {
					// Profile already exists with the right num_ctx
					apiID = profName
					profileCtx = numCtx
					delete(existingProfiles, m.Name)
				} else {
					// Create new profile (or replace stale one)
					if err := CreateProfile(ctx, d.client, baseURL, m.Name, profName, numCtx); err != nil {
						slog.Warn("failed to create ollama profile",
							"model", m.Name, "profile", profName, "error", err)
					} else {
						slog.Info("created ollama profile",
							"model", m.Name, "profile", profName, "num_ctx", numCtx)
						apiID = profName
						profileCtx = numCtx
						delete(existingProfiles, m.Name)
					}
				}
			}
		}

		models[m.Name] = &Model{
			ID:         m.Name,
			ProviderID: "ollama",
			Name:       m.Name,
			Family:     family,
			API: ModelAPI{
				ID:  apiID,
				URL: baseURL,
			},
			Status:  "active",
			Headers: make(map[string]string),
			Options: make(map[string]any),
			Cost:    ModelCost{},
			Limit:   ModelLimit{Context: profileCtx, Output: profileCtx / 2},
			Capabilities: caps,
		}
	}

	// Clean up stale profiles (profile exists but base model is gone)
	if d.autoProfileEnabled() {
		for baseName, profName := range existingProfiles {
			if _, exists := models[baseName]; !exists {
				if err := DeleteModel(ctx, d.client, baseURL, profName); err != nil {
					slog.Warn("failed to delete stale profile",
						"profile", profName, "error", err)
				} else {
					slog.Info("deleted stale ollama profile", "profile", profName)
				}
			}
		}
	}

	d.diffModels("ollama", models)
	d.handleDiscoverySuccess("ollama")

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

	// Trigger warmup probes for newly discovered models
	for _, m := range models {
		d.maybeWarmup(ctx, m)
	}
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

	if d.gpuMemory <= 0 {
		return advertisedCtx
	}

	// Query model details
	info, err := ShowModel(ctx, d.client, baseURL, modelName)
	if err != nil {
		slog.Warn("failed to query model info for auto-profiling",
			"model", modelName, "error", err)
		return advertisedCtx
	}

	numCtx := CalculateNumCtx(d.gpuMemory, *info, advertisedCtx)

	// Apply max cap from config
	if d.autoProfile != nil && d.autoProfile.MaxNumCtx != nil && numCtx > *d.autoProfile.MaxNumCtx {
		numCtx = *d.autoProfile.MaxNumCtx
	}

	return numCtx
}

// maybeWarmup triggers a background warmup probe for a model if it hasn't
// been warmed up yet. On probe failure, sets ToolCall capability to false.
func (d *Discovery) maybeWarmup(ctx context.Context, m *Model) {
	d.warmedMu.Lock()
	if d.warmedModels[m.ID] {
		d.warmedMu.Unlock()
		return
	}
	d.warmedModels[m.ID] = true
	d.warmedMu.Unlock()

	go func() {
		capable, err := WarmupProbe(ctx, d.client, m.API.URL, m.API.ID)
		if err != nil {
			slog.Warn("warmup probe failed", "model", m.ID, "error", err)
			capable = false
		}

		if !capable {
			d.registry.UpdateCapability(m.ProviderID, m.ID, "ToolCall", false)
			slog.Info("model does not support tool calls", "model", m.ID)
		}

		d.bus.Publish("provider.warmup.complete", map[string]any{
			"modelID":     m.ID,
			"toolCapable": capable,
		})
	}()
}

type vllmModelsResponse struct {
	Data []vllmModel `json:"data"`
}

type vllmModel struct {
	ID          string `json:"id"`
	MaxModelLen int    `json:"max_model_len,omitempty"`
}

func (d *Discovery) discoverVLLM(ctx context.Context, baseURL string) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.handleDiscoveryFailure("vllm", "vLLM", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.handleDiscoveryFailure("vllm", "vLLM", fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var body vllmModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		d.handleDiscoveryFailure("vllm", "vLLM", err)
		return
	}

	models := make(map[string]*Model, len(body.Data))
	for _, m := range body.Data {
		contextLen := m.MaxModelLen
		if contextLen == 0 {
			contextLen = 8192
		}

		models[m.ID] = &Model{
			ID:         m.ID,
			ProviderID: "vllm",
			Name:       m.ID,
			API: ModelAPI{
				ID:  m.ID,
				URL: baseURL,
			},
			Status:  "active",
			Headers: make(map[string]string),
			Options: make(map[string]any),
			Limit:   ModelLimit{Context: contextLen, Output: contextLen / 2},
			Capabilities: ModelCaps{
				Temperature: true,
				ToolCall:    true,
				Input:       ModalityCaps{Text: true},
				Output:      ModalityCaps{Text: true},
			},
		}
	}

	d.diffModels("vllm", models)
	d.handleDiscoverySuccess("vllm")

	d.registry.Register(&Info{
		ID:      "vllm",
		Name:    "vLLM",
		Source:  "custom",
		Env:     []string{},
		Options: map[string]any{},
		Models:  models,
	})

	d.bus.Publish("provider.discovered", map[string]any{
		"providerID": "vllm",
		"modelCount": len(models),
	})
}

func (d *Discovery) discoverLMStudio(ctx context.Context, baseURL string) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.handleDiscoveryFailure("lm-studio", "LM Studio", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.handleDiscoveryFailure("lm-studio", "LM Studio", fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var body vllmModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		d.handleDiscoveryFailure("lm-studio", "LM Studio", err)
		return
	}

	models := make(map[string]*Model, len(body.Data))
	for _, m := range body.Data {
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
			Limit:   ModelLimit{Context: 8192, Output: 4096},
			Capabilities: ModelCaps{
				Temperature: true,
				ToolCall:    true,
				Input:       ModalityCaps{Text: true},
				Output:      ModalityCaps{Text: true},
			},
		}
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

// DiscoverOpenRouter queries the OpenRouter API for available models.
func (d *Discovery) DiscoverOpenRouter(ctx context.Context, apiKey string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
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

		models[entry.ID] = &Model{
			ID:         entry.ID,
			ProviderID: "openrouter",
			Name:       entry.Name,
			Family:     family,
			API: ModelAPI{
				ID:  entry.ID,
				URL: "https://openrouter.ai/api/v1",
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

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID                  string                    `json:"id"`
	Name                string                    `json:"name"`
	ContextLength       int                       `json:"context_length"`
	Architecture        *openRouterArchitecture    `json:"architecture,omitempty"`
	Pricing             *openRouterPricing         `json:"pricing,omitempty"`
	TopProvider         *openRouterTopProvider     `json:"top_provider,omitempty"`
	SupportedParameters []string                  `json:"supported_parameters,omitempty"`
	Reasoning           *openRouterReasoning       `json:"reasoning,omitempty"`
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
