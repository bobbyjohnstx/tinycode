package server

import "net/http"

func (s *Server) handlePermissionList(w http.ResponseWriter, r *http.Request) {
	permissions := s.permissionStore.List()
	if permissions == nil {
		permissions = []PendingPermission{}
	}
	respondJSON(w, http.StatusOK, permissions)
}

func (s *Server) handlePermissionReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Action    string `json:"action"`
		SessionID string `json:"sessionID,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Action == "" {
		respondError(w, http.StatusBadRequest, "action is required")
		return
	}

	s.permissionStore.Remove(id)

	s.deps.Bus.Publish("permission.reply", map[string]any{
		"permissionID": id,
		"action":       body.Action,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": id,
		"status":       "replied",
	})
}
