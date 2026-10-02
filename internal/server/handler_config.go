package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	dir := requestDirectory(r, ".")

	cfg, err := config.Load(dir)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if cfg.Model != "" {
		provID, modelID := provider.ParseModel(cfg.Model)
		if provID == "" || modelID == "" {
			if resolved := s.resolveDefaultModel(); resolved != nil {
				cfg.Model = resolved.ProviderID + "/" + resolved.ID
			}
		} else if _, err := s.deps.Registry.GetModel(provID, modelID); err != nil {
			if resolved := s.resolveDefaultModel(); resolved != nil {
				cfg.Model = resolved.ProviderID + "/" + resolved.ID
			}
		}
	}

	respondJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	configPath := config.GlobalConfigFile()

	existing := make(map[string]any)
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	allowedFields := map[string]bool{
		"model": true, "theme": true, "logLevel": true, "small_model": true,
		"agents": true, "scopedModels": true, "temperature": true, "topP": true,
		"maxTokens": true,
	}
	for k := range body {
		if !allowedFields[k] {
			respondError(w, http.StatusForbidden, fmt.Sprintf("field %q cannot be modified via API", k))
			return
		}
	}

	for k, v := range body {
		existing[k] = v
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to marshal config")
		return
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create config directory")
		return
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write config")
		return
	}

	respondJSON(w, http.StatusOK, existing)
}
