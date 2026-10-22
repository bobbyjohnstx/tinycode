package server

import (
	"context"
	"net/http"
)

func (s *Server) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPService != nil {
		respondJSON(w, http.StatusOK, s.deps.MCPService.Status(r.Context()))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) handleMCPReconnect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.deps.MCPService == nil {
		respondError(w, http.StatusNotFound, "MCP service not available")
		return
	}
	// Detach from request cancel so Restart's async connect outlives this handler
	// (same pattern as StartPrompt).
	if err := s.deps.MCPService.Restart(context.WithoutCancel(r.Context()), name); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "reconnecting"})
}
