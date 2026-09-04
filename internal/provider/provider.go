package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Info struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Source  string            `json:"source"`
	Env     []string          `json:"env"`
	Options map[string]any    `json:"options"`
	Models  map[string]*Model `json:"models"`
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]*Info
	disabled  map[string]bool
	enabled   map[string]bool
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]*Info),
		disabled:  make(map[string]bool),
		enabled:   make(map[string]bool),
	}
}

// SetFilters configures which providers are enabled/disabled.
func (r *Registry) SetFilters(enabled, disabled []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.enabled = make(map[string]bool, len(enabled))
	for _, id := range enabled {
		r.enabled[id] = true
	}

	r.disabled = make(map[string]bool, len(disabled))
	for _, id := range disabled {
		r.disabled[id] = true
	}
}

func (r *Registry) isFiltered(id string) bool {
	if len(r.enabled) > 0 && !r.enabled[id] {
		return true
	}
	return r.disabled[id]
}

// Register adds or updates a provider. Respects enabled/disabled filters.
func (r *Registry) Register(info *Info) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isFiltered(info.ID) {
		return
	}
	r.providers[info.ID] = info
}

// Remove removes a provider.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, id)
}

// GetProvider returns provider info by ID.
func (r *Registry) GetProvider(id string) (*Info, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", id)
	}
	return p, nil
}

// GetModel returns a model by provider and model ID.
func (r *Registry) GetModel(providerID, modelID string) (*Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerID)
	}

	m, ok := p.Models[modelID]
	if !ok {
		suggestion := r.suggestModel(providerID, modelID)
		if suggestion != "" {
			return nil, fmt.Errorf("model %q not found in provider %q, did you mean %q?", modelID, providerID, suggestion)
		}
		return nil, fmt.Errorf("model %q not found in provider %q", modelID, providerID)
	}

	return m, nil
}

// ListProviders returns all registered providers.
func (r *Registry) ListProviders() []*Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Info, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// ListModels returns all models across all providers.
func (r *Registry) ListModels() []*Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Model
	for _, p := range r.providers {
		for _, m := range p.Models {
			result = append(result, m)
		}
	}
	return result
}

// ParseModel splits "provider/model" into provider and model IDs.
func ParseModel(s string) (providerID, modelID string) {
	idx := strings.IndexByte(s, '/')
	if idx < 0 {
		return "", s
	}
	return s[:idx], s[idx+1:]
}

func (r *Registry) suggestModel(providerID, modelID string) string {
	p, ok := r.providers[providerID]
	if !ok {
		return ""
	}

	lower := strings.ToLower(modelID)
	var best string
	bestScore := 0

	for id := range p.Models {
		score := 0
		lowerID := strings.ToLower(id)
		if strings.Contains(lowerID, lower) || strings.Contains(lower, lowerID) {
			score = len(lowerID)
		}
		if score > bestScore {
			bestScore = score
			best = id
		}
	}
	return best
}

var sizePattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*[bB]`)

func parseModelSize(name string) *float64 {
	match := sizePattern.FindStringSubmatch(name)
	if match == nil {
		return nil
	}
	f, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return nil
	}
	return &f
}
