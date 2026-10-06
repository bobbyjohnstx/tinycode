package server

import (
	"net/http"
	"os"
	"strings"
	"sync"
)

// credentialStore holds in-memory provider credentials.
type credentialStore struct {
	mu    sync.RWMutex
	creds map[string]map[string]string // providerID -> key/value pairs
}

func newCredentialStore() *credentialStore {
	return &credentialStore{
		creds: make(map[string]map[string]string),
	}
}

func (cs *credentialStore) Set(providerID string, creds map[string]string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.creds[providerID] = creds
}

func (cs *credentialStore) Get(providerID string) (map[string]string, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c, ok := cs.creds[providerID]
	return c, ok
}

func (cs *credentialStore) Delete(providerID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.creds, providerID)
}

func (s *Server) handleAuthPut(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")

	var body map[string]string
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.credentials.Set(providerID, body)

	key := body["apiKey"]
	if key == "" {
		key = body["api_key"]
	}

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

	s.credentials.Delete(providerID)

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
