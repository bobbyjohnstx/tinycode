package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

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

	// Normalize to TS-contract Reply type (allow→once, deny→reject).
	reply := action
	switch reply {
	case "allow":
		reply = "once"
	case "deny":
		reply = "reject"
	}
	switch reply {
	case "once", "always", "reject":
	default:
		respondError(w, http.StatusBadRequest, "unknown reply: must be once, always, reject, allow, or deny")
		return
	}

	s.publishPermissionReply(id, body.SessionID, reply, body.Message)

	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": id,
		"status":       "replied",
	})
}

// publishPermissionReply resolves a pending ask via PermService when possible
// (single permission.replied publish from RespondToAsk). Falls back to a direct
// bus publish for store-only replies with no matching pending ask.
func (s *Server) publishPermissionReply(requestID, sessionID, reply, message string) {
	if s.deps.PermService != nil {
		err := s.deps.PermService.RespondToAsk(permission.ReplyInput{
			RequestID: requestID,
			Reply:     permission.Reply(reply),
			Message:   message,
		})
		if err == nil {
			s.permissionStore.Remove(requestID)
			return
		}
	}

	s.permissionStore.Remove(requestID)

	evt := map[string]any{
		"requestID": requestID,
		"reply":     reply,
	}
	if sessionID != "" {
		evt["sessionID"] = sessionID
	}
	if message != "" {
		evt["message"] = message
	}
	s.deps.Bus.Publish("permission.replied", evt)
}
