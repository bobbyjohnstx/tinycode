package server

import (
	"errors"
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/plugin"
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
	respondJSON(w, http.StatusOK, mgr.Plugins())
}

func (s *Server) handlePluginLoad(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string         `json:"name"`
		Options map[string]any `json:"options"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
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

	if _, err := mgr.Load(body.Name, body.Options); err != nil {
		if errors.Is(err, plugin.ErrPluginNotFound) {
			respondError(w, http.StatusNotFound, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "loaded", "name": body.Name})
}

func (s *Server) handlePluginUnload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
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

	if err := mgr.UnloadPlugin(body.ID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePluginEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Event     string `json:"event"`
		SessionID string `json:"sessionID"`
		Data      any    `json:"data,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.deps.Bus.Publish("plugin.event", map[string]any{
		"event":     body.Event,
		"sessionID": body.SessionID,
		"data":      body.Data,
	})

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePluginRegistry(w http.ResponseWriter, r *http.Request) {
	entries := plugin.Registry()
	respondJSON(w, http.StatusOK, entries)
}
