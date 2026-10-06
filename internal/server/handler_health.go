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
	mgr := s.deps.LSPManager
	if mgr == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"enabled":   false,
			"languages": []string{},
			"note":      "LSP manager not attached to this server instance",
		})
		return
	}
	if mgr.Disabled() {
		respondJSON(w, http.StatusOK, map[string]any{
			"enabled":   false,
			"languages": []string{},
		})
		return
	}
	langs := mgr.AvailableLanguages()
	if langs == nil {
		langs = []string{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"enabled":   true,
		"languages": langs,
		"note":      "Language servers are lazily connected on first tool use",
	})
}

func (s *Server) handleFormatter(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status": "unavailable",
	})
}
