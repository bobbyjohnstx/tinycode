package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/auth"
)

func (s *Server) handleAuthPut(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")

	var body map[string]string
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.credentials.Set(providerID, body); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to persist credentials")
		return
	}

	key := auth.KeyFromCreds(body)

	// Apply token to the provider registry so subsequent LLM client builds
	// (OpenRouter and OpenAI-compatible) pick up the stored key.
	if key != "" && s.deps.Registry != nil {
		if info, err := s.deps.Registry.GetProvider(providerID); err == nil {
			if info.Options == nil {
				info.Options = make(map[string]any)
			}
			info.Options["apiKey"] = key
		}
	}

	if strings.EqualFold(providerID, "openrouter") {
		if key != "" {
			_ = os.Setenv("OPENROUTER_API_KEY", key)
			if s.deps.Discovery != nil {
				if err := s.deps.Discovery.DiscoverOpenRouter(r.Context(), key); err != nil {
					respondError(w, http.StatusBadGateway, "openrouter discovery failed: "+err.Error())
					return
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"providerID": providerID,
		"status":     "stored",
	})
}

func (s *Server) handleAuthDelete(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")

	if err := s.credentials.Delete(providerID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete credentials")
		return
	}

	if s.deps.Registry != nil {
		if info, err := s.deps.Registry.GetProvider(providerID); err == nil && info.Options != nil {
			delete(info.Options, "apiKey")
			delete(info.Options, "api_key")
			delete(info.Options, "Authorization")
			delete(info.Options, "authorization")
		}
	}

	if strings.EqualFold(providerID, "openrouter") {
		_ = os.Unsetenv("OPENROUTER_API_KEY")
	}

	w.WriteHeader(http.StatusNoContent)
}
