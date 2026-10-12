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
	probeTimeout = 2 * time.Second
	pollInterval = 30 * time.Second
	// healthyPollInterval is the gap between probes after a successful discovery.
	// A local daemon that is not running (LM Studio on 127.0.0.1:1234 when it
	// is not installed) is not a warning: failures log at info and, after
	// maxConsecutiveFailures, use dormant backoff instead of a probe every
	// pollInterval. That covers the noise in GitHub #39 / #28 without skipping
	// LM Studio on the startup scan.
	healthyPollInterval    = 15 * time.Minute
	dormantPollInitial     = 2 * time.Minute
	dormantPollMax         = 10 * time.Minute
	maxConsecutiveFailures = 3
	maxWarmupFailures      = 3
)

type Discovery struct {
	registry        *Registry
	bus             *bus.Bus
	client          *http.Client // short timeout for discovery probes
	ollamaClient    *http.Client // longer timeout for Show/Create/Delete
	cancel          context.CancelFunc
	autoProfile     *AutoProfileConfig
	detectGPU       func() (int64, error)
	gpuMemory       int64
	gpuOnce         sync.Once
	warmedMu        sync.Mutex
	warmedModels    map[string]bool
	warmingModels   map[string]bool
	warmupFails     map[string]int
	dormantMu       sync.Mutex
	dormant         map[string]bool // providers removed after consecutive failures
	nextPoll        map[string]time.Time
	backoff         map[string]time.Duration
	healthy         map[string]bool // last probe succeeded
	healthyNextPoll map[string]time.Time
	probing         map[string]bool // in-flight discover, shared by ticker and Rescan
	ollamaURL       string
	vllmURL         string
	lmStudioURL     string
	now             func() time.Time
}

func NewDiscovery(registry *Registry, b *bus.Bus) *Discovery {
	return &Discovery{
		registry:        registry,
		bus:             b,
		detectGPU:       DetectGPUMemory,
		warmedModels:    make(map[string]bool),
		warmingModels:   make(map[string]bool),
		warmupFails:     make(map[string]int),
		dormant:         make(map[string]bool),
		nextPoll:        make(map[string]time.Time),
		backoff:         make(map[string]time.Duration),
		healthy:         make(map[string]bool),
		healthyNextPoll: make(map[string]time.Time),
		probing:         make(map[string]bool),
		now:             time.Now,
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
	d.ollamaURL = ollamaURL
	d.vllmURL = vllmURL
	d.lmStudioURL = lmStudioURL

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

// Rescan force-probes the URLs passed to Start, ignoring the healthy and
// dormant schedules. Filtered providers are still skipped. An in-flight probe
// for the same provider is skipped.
func (d *Discovery) Rescan(ctx context.Context) {
	if d.ollamaURL != "" && !d.registry.isFiltered("ollama") {
		d.discoverOllama(ctx, d.ollamaURL)
	}
	if d.vllmURL != "" && !d.registry.isFiltered("vllm") {
		d.discoverVLLM(ctx, d.vllmURL)
	}
	if d.lmStudioURL != "" && !d.registry.isFiltered("lm-studio") {
		d.discoverLMStudio(ctx, d.lmStudioURL)
	}
}

// shouldPoll reports whether a provider should be probed this tick.
// Filtered providers are skipped. Unconfirmed providers are probed every tick.
// Healthy providers wait healthyPollInterval. Dormant providers wait until
// their backoff elapses.
func (d *Discovery) shouldPoll(providerID string) bool {
	if d.registry.isFiltered(providerID) {
		return false
	}
	d.dormantMu.Lock()
	defer d.dormantMu.Unlock()
	if d.dormant[providerID] {
		next, ok := d.nextPoll[providerID]
		if !ok {
			return true
		}
		return !d.now().Before(next)
	}
	if d.healthy[providerID] {
		next, ok := d.healthyNextPoll[providerID]
		if ok && d.now().Before(next) {
			return false
		}
	}
	return true
}

func (d *Discovery) beginProbe(providerID string) bool {
	d.dormantMu.Lock()
	defer d.dormantMu.Unlock()
	if d.probing[providerID] {
		return false
	}
	d.probing[providerID] = true
	return true
}

func (d *Discovery) endProbe(providerID string) {
	d.dormantMu.Lock()
	delete(d.probing, providerID)
	d.dormantMu.Unlock()
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

// handleDiscoveryFailure records a failure for a provider and removes it
// after maxConsecutiveFailures consecutive failures.
func (d *Discovery) handleDiscoveryFailure(providerID, providerName string, err error) {
	d.dormantMu.Lock()
	delete(d.healthy, providerID)
	delete(d.healthyNextPoll, providerID)
	d.dormantMu.Unlock()

	count := d.registry.RecordFailure(providerID)
	slog.Info("provider discovery failed",
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
	d.healthy[providerID] = true
	d.healthyNextPoll[providerID] = d.now().Add(healthyPollInterval)
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

type vllmModelsResponse struct {
	Data []vllmModel `json:"data"`
}

type vllmModel struct {
	ID          string `json:"id"`
	MaxModelLen int    `json:"max_model_len,omitempty"`
}

func (d *Discovery) discoverVLLM(ctx context.Context, baseURL string) {
	if !d.beginProbe("vllm") {
		return
	}
	defer d.endProbe("vllm")

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
