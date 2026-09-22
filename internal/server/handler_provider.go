package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

func (s *Server) handleProviderList(w http.ResponseWriter, r *http.Request) {
	reg := s.deps.Registry
	providers := reg.ListProviders()

	connected := make([]string, 0, len(providers))
	for _, p := range providers {
		if len(p.Models) > 0 {
			connected = append(connected, p.ID)
		}
	}

	defaults := map[string]string{}

	if s.config.DefaultModel != "" {
		provID, modelID := provider.ParseModel(s.config.DefaultModel)
		if provID != "" && modelID != "" {
			if _, err := reg.GetModel(provID, modelID); err == nil {
				defaults[provID] = modelID
			}
		}
	}

	if len(defaults) == 0 && len(connected) > 0 {
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
			defaults[candidates[0].providerID] = candidates[0].modelID
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"all":       providers,
		"connected": connected,
		"default":   defaults,
	})
}

func (s *Server) handleProviderAuth(w http.ResponseWriter, r *http.Request) {
	reg := s.deps.Registry
	providers := reg.ListProviders()

	localSources := map[string]bool{
		"custom": true,
	}
	localIDs := map[string]bool{
		"ollama":    true,
		"lm-studio": true,
		"vllm":      true,
	}

	result := make(map[string]any, len(providers))
	for _, p := range providers {
		if localSources[p.Source] && localIDs[p.ID] {
			result[p.ID] = []any{}
		} else if len(p.Env) > 0 {
			result[p.ID] = []map[string]any{
				{"type": "api", "label": "API Key"},
			}
		} else {
			result[p.ID] = []map[string]any{
				{"type": "api", "label": "API Key"},
			}
		}
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleProviderGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reg := s.deps.Registry

	info, err := reg.GetProvider(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "provider not found")
		return
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleModelList(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")
	reg := s.deps.Registry

	info, err := reg.GetProvider(providerID)
	if err != nil {
		respondError(w, http.StatusNotFound, "provider not found")
		return
	}

	respondJSON(w, http.StatusOK, info.Models)
}

func (s *Server) handleModelGet(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")
	modelID := r.PathValue("modelID")
	reg := s.deps.Registry

	model, err := reg.GetModel(providerID, modelID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, model)
}

func (s *Server) handleProviderBalance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reg := s.deps.Registry

	info, err := reg.GetProvider(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "provider not found")
		return
	}

	// Only OpenRouter supports balance queries.
	if !strings.EqualFold(info.Name, "openrouter") && !strings.EqualFold(id, "openrouter") {
		respondJSON(w, http.StatusOK, map[string]any{"remaining": nil, "provider": info.Name})
		return
	}

	apiKey, _ := info.Options["apiKey"].(string)
	if apiKey == "" {
		respondJSON(w, http.StatusOK, map[string]any{"remaining": nil, "provider": "OpenRouter"})
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://openrouter.ai/api/v1/auth/key", nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to fetch balance")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respondJSON(w, http.StatusOK, map[string]any{"remaining": nil, "provider": "OpenRouter"})
		return
	}

	var orResp struct {
		Data struct {
			Limit       *float64 `json:"limit"`
			Usage       float64  `json:"usage"`
			LimitRemain float64  `json:"limit_remaining"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		respondError(w, http.StatusBadGateway, "failed to parse balance response")
		return
	}

	// If no limit is set, remaining is not meaningful.
	if orResp.Data.Limit == nil {
		respondJSON(w, http.StatusOK, map[string]any{"remaining": nil, "provider": "OpenRouter"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"remaining": orResp.Data.LimitRemain,
		"provider":  "OpenRouter",
	})
}
