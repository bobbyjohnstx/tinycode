package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode-go/internal/config"
)

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("directory")
	if dir == "" {
		dir = "."
	}

	cfg, err := config.Load(dir)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	configPath := config.GlobalConfigFile()

	existing := make(map[string]any)
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &existing)
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

	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write config")
		return
	}

	respondJSON(w, http.StatusOK, existing)
}
