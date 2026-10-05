package server

import (
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// ResolveDefaultModelRef returns a ModelRef for the best available model,
// checking the configured default first, then falling back to the smallest
// chat-capable model prioritizing local providers.
func ResolveDefaultModelRef(reg *provider.Registry, defaultModel string) *session.ModelRef {
	return resolveDefaultModelRef(reg, defaultModel)
}

func resolveDefaultModelRef(reg *provider.Registry, defaultModel string) *session.ModelRef {
	// 1. Check configured default model (e.g. "ollama/llama3.2").
	if defaultModel != "" {
		providerID, modelID := provider.ParseModel(defaultModel)
		if providerID != "" && modelID != "" {
			if _, err := reg.GetModel(providerID, modelID); err == nil {
				return &session.ModelRef{ProviderID: providerID, ModelID: modelID}
			}
		}
	}

	// 2. Pick the smallest chat-capable model from local providers first.
	providers := reg.ListProviders()
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })

	localProviders := map[string]bool{"ollama": true, "lm-studio": true, "vllm": true}
	type candidate struct {
		providerID string
		modelID    string
		sizeB      float64
		local      bool
	}
	var candidates []candidate

	for _, p := range providers {
		for id, m := range p.Models {
			if strings.Contains(strings.ToLower(id), "embed") {
				continue
			}
			size := 999.0
			if s := m.SizeB(); s != nil {
				size = *s
			}
			candidates = append(candidates, candidate{p.ID, id, size, localProviders[p.ID]})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].local != candidates[j].local {
			return candidates[i].local
		}
		if candidates[i].sizeB != candidates[j].sizeB {
			return candidates[i].sizeB < candidates[j].sizeB
		}
		if candidates[i].providerID != candidates[j].providerID {
			return candidates[i].providerID < candidates[j].providerID
		}
		return candidates[i].modelID < candidates[j].modelID
	})
	if len(candidates) > 0 {
		return &session.ModelRef{ProviderID: candidates[0].providerID, ModelID: candidates[0].modelID}
	}

	return nil
}
