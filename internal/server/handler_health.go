package server

import "net/http"

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	healthy := true
	checks := map[string]string{}

	if err := s.deps.DB.PingContext(r.Context()); err != nil {
		healthy = false
		checks["database"] = "error: " + err.Error()
	} else {
		checks["database"] = "ok"
	}

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	respondJSON(w, status, map[string]any{
		"healthy": healthy,
		"version": s.config.Version,
		"checks":  checks,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"version": s.config.Version,
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
