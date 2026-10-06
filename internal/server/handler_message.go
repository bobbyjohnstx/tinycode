package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func (s *Server) handleMessageDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	messageID := r.PathValue("messageID")

	ms := s.messageStore()
	if err := ms.DeleteByID(messageID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ps := s.partStore()
	_ = ps.DeleteByMessage(messageID)

	s.deps.Bus.Publish("message.removed", map[string]any{
		"sessionID": sessionID,
		"messageID": messageID,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMessageGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	messageID := r.PathValue("messageID")

	ms := s.messageStore()

	messages, err := ms.List(sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, m := range messages {
		if m.ID != messageID {
			continue
		}

		// Parts live in the message blob (PartStore.Save is unused).
		var parts any
		if len(m.Parts) > 0 {
			parts = m.Parts
		} else {
			parts = []session.Part{}
		}

		createdMs := m.CreatedAt.UnixMilli()
		info := map[string]any{
			"id":        m.ID,
			"sessionID": m.SessionID,
			"role":      string(m.Role),
			"time":      map[string]any{"created": createdMs},
		}
		if m.Role == session.RoleAssistant {
			info["time"] = map[string]any{"created": createdMs, "completed": createdMs}
		}
		if m.Model != "" {
			info["modelID"] = m.Model
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"info":  info,
			"parts": parts,
		})
		return
	}

	respondError(w, http.StatusNotFound, "message not found")
}
