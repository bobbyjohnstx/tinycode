package server

import "net/http"

const serverVersion = "0.1.0"

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"healthy": true,
		"version": serverVersion,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"version": serverVersion,
	})
}

func (s *Server) handleLSP(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status": "available",
		"note":   "LSP tools are registered via the tool registry; servers are lazily connected on first use",
	})
}

func (s *Server) handleFormatter(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status": "unavailable",
	})
}
