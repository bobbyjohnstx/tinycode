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

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	probeTimeout           = 2 * time.Second
	pollInterval           = 30 * time.Second
	dormantPollInitial     = 2 * time.Minute
	dormantPollMax         = 10 * time.Minute
	maxConsecutiveFailures = 3
	maxWarmupFailures      = 3
)

type Discovery struct {
	registry      *Registry
	bus           *bus.Bus
	client        *http.Client // short timeout for discovery probes
	ollamaClient  *http.Client // longer timeout for Show/Create/Delete
	cancel        context.CancelFunc
	autoProfile   *AutoProfileConfig
	detectGPU     func() (int64, error)
	gpuMemory     int64
	gpuOnce       sync.Once
	warmedMu      sync.Mutex
	warmedModels  map[string]bool
	warmingModels map[string]bool
	warmupFails   map[string]int
	dormantMu     sync.Mutex
	dormant       map[string]bool // providers removed after consecutive failures
	nextPoll      map[string]time.Time
	backoff       map[string]time.Duration
	now           func() time.Time
}

func NewDiscovery(registry *Registry, b *bus.Bus) *Discovery {
	return &Discovery{
		registry:      registry,
		bus:           b,
		detectGPU:     DetectGPUMemory,
		warmedModels:  make(map[string]bool),
		warmingModels: make(map[string]bool),
		warmupFails:   make(map[string]int),
		dormant:       make(map[string]bool),
		nextPoll:      make(map[string]time.Time),
		backoff:       make(map[string]time.Duration),
		now:           time.Now,
		client: &http.Client{
			Timeout: probeTimeout,
		},
		ollamaClient: &http.Client{
			Timeout: ollamaCreateTimeout,
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

	safego.Go(func() {
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
	})
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

// shouldPoll reports whether a provider should be probed this tick.
// Live providers are probed every tick. Dormant providers wait until their
// backoff elapses so a late-start daemon can still reconnect.
func (d *Discovery) shouldPoll(providerID string) bool {
	d.dormantMu.Lock()
	defer d.dormantMu.Unlock()
	if !d.dormant[providerID] {
		return true
	}
	next, ok := d.nextPoll[providerID]
	if !ok {
		return true
	}
	return !d.now().Before(next)
}

// scheduleDormantPoll sets the next probe for a dormant provider.
// The first wait is dormantPollInitial; later failures double it up to dormantPollMax.
func (d *Discovery) scheduleDormantPoll(providerID string) {
	d.dormantMu.Lock()
	defer d.dormantMu.Unlock()
	delay := d.backoff[providerID]
	if delay <= 0 {
		delay = dormantPollInitial
	} else {
		delay *= 2
		if delay > dormantPollMax {
			delay = dormantPollMax
		}
	}
	d.backoff[providerID] = delay
	d.nextPoll[providerID] = d.now().Add(delay)
}

// isDormant reports whether a provider was removed after consecutive failures.
func (d *Discovery) isDormant(providerID string) bool {
	d.dormantMu.Lock()
	defer d.dormantMu.Unlock()
	return d.dormant[providerID]
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
		d.scheduleDormantPoll(providerID)
		d.bus.Publish("provider.removed", map[string]any{
			"providerID":   providerID,
			"providerName": providerName,
			"reason":       "consecutive_failures",
			"failures":     count,
		})
		slog.Warn("provider removed after consecutive failures, polling for reconnect on backoff",
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
	delete(d.nextPoll, providerID)
	delete(d.backoff, providerID)
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
