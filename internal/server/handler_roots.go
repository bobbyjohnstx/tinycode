package server

import (
	"net/http"
)

func (s *Server) handleRootsAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Path == "" {
		respondError(w, http.StatusBadRequest, "path is required")
		return
	}

	if s.deps.ToolRegistry == nil {
		respondError(w, http.StatusInternalServerError, "tool registry unavailable")
		return
	}

	s.deps.ToolRegistry.AddWritableRoot(body.Path)
	respondJSON(w, http.StatusOK, map[string]any{
		"roots": s.deps.ToolRegistry.WritableRootsList(),
	})
}

func (s *Server) handleRootsList(w http.ResponseWriter, _ *http.Request) {
	var roots []string
	if s.deps.ToolRegistry != nil {
		roots = s.deps.ToolRegistry.WritableRootsList()
	}
	if roots == nil {
		roots = []string{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"roots": roots,
	})
}
