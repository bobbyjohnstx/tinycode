package server

import (
	"net/http"

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
				defaults["provider"] = provID
				defaults["model"] = modelID
			}
		}
	}

	if len(defaults) == 0 && len(connected) > 0 {
		for _, p := range providers {
			if len(p.Models) == 0 {
				continue
			}
			for modelID := range p.Models {
				defaults["provider"] = p.ID
				defaults["model"] = modelID
				break
			}
			break
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"all":       providers,
		"connected": connected,
		"default":   defaults,
	})
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
