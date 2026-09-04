package server

import (
	"net/http"

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

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "updated",
	})
}
