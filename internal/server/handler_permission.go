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
		Reply     string `json:"reply"`
		SessionID string `json:"sessionID,omitempty"`
		Message   string `json:"message,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// SDK sends "reply"; older clients send "action".
	action := body.Reply
	if action == "" {
		action = body.Action
	}
	if action == "" {
		respondError(w, http.StatusBadRequest, "reply or action is required")
		return
	}

	// Normalize to TS-contract Reply type
	reply := action
	switch reply {
	case "allow":
		reply = "once"
	case "deny":
		reply = "reject"
	}

	s.permissionStore.Remove(id)

	evt := map[string]any{
		"requestID": id,
		"reply":     reply,
	}
	if body.SessionID != "" {
		evt["sessionID"] = body.SessionID
	}
	if body.Message != "" {
		evt["message"] = body.Message
	}
	s.deps.Bus.Publish("permission.replied", evt)

	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": id,
		"status":       "replied",
	})
}
