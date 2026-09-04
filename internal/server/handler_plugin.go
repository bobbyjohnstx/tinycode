package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
)

func (s *Server) pluginManager() *plugin.Manager {
	return s.deps.PluginManager
}

func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request) {
	mgr := s.pluginManager()
	if mgr == nil {
		respondJSON(w, http.StatusOK, []plugin.PluginInfo{})
		return
	}
	respondJSON(w, http.StatusOK, mgr.List())
}

func (s *Server) handlePluginLoad(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	mgr := s.pluginManager()
	if mgr == nil {
		respondError(w, http.StatusServiceUnavailable, "plugin system not initialized")
		return
	}

	info, err := mgr.Load(body.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handlePluginUnload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.ID == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	mgr := s.pluginManager()
	if mgr == nil {
		respondError(w, http.StatusServiceUnavailable, "plugin system not initialized")
		return
	}

	if err := mgr.Unload(body.ID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePluginRegistry(w http.ResponseWriter, r *http.Request) {
	mgr := s.pluginManager()
	if mgr == nil {
		respondJSON(w, http.StatusOK, []plugin.RegistryEntry{})
		return
	}
	respondJSON(w, http.StatusOK, mgr.Registry())
}
