package server

import (
	"net/http"
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

	respondJSON(w, http.StatusOK, map[string]any{
		"providerID": providerID,
		"status":     "stored",
	})
}

func (s *Server) handleAuthDelete(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")

	s.credentials.Delete(providerID)

	w.WriteHeader(http.StatusNoContent)
}
