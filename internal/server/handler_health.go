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
