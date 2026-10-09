package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/formatter"
)

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
			"errors":    0,
			"warnings":  0,
			"note":      "LSP manager not attached to this server instance",
		})
		return
	}
	if mgr.Disabled() {
		respondJSON(w, http.StatusOK, map[string]any{
			"enabled":   false,
			"languages": []string{},
			"errors":    0,
			"warnings":  0,
		})
		return
	}
	langs := mgr.AvailableLanguages()
	if langs == nil {
		langs = []string{}
	}
	errors, warnings := mgr.DiagnosticCounts()
	respondJSON(w, http.StatusOK, map[string]any{
		"enabled":   true,
		"languages": langs,
		"errors":    errors,
		"warnings":  warnings,
		"note":      "Language servers are lazily connected on first tool use",
	})
}

func (s *Server) handleFormatter(w http.ResponseWriter, _ *http.Request) {
	var cfg *config.FormatterConfig
	if s.deps.Config != nil {
		cfg = s.deps.Config.Formatter
	}
	respondJSON(w, http.StatusOK, formatter.Resolve(cfg).Status())
}
