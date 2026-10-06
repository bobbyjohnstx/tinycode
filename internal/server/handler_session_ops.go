package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/vcs"
)

func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	status := s.sessionManager.Status()
	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleSessionInit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	s.deps.Bus.Publish("session.initialized", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionSummarize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	if _, err := store.Get(id); err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	// Manual summarize/compact over HTTP is not wired to Processor.compact yet.
	// Do not publish a fake session.compacted success event.
	respondError(w, http.StatusNotImplemented, "session summarize/compact is not implemented")
}

func (s *Server) handleSessionCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Command string `json:"command"`
		Args    string `json:"args,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.deps.Bus.Publish("session.command", map[string]any{
		"sessionID": id,
		"command":   body.Command,
		"args":      body.Args,
	})
	respondJSON(w, http.StatusAccepted, map[string]any{
		"sessionID": id,
		"command":   body.Command,
	})
}

func (s *Server) handleSessionRevert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	// Stash synchronously so the 200 ACK means revert completed (or failed).
	if err := s.sessionManager.Revert(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.deps.Bus.Publish("session.reverted", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionUnrevert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	if err := s.sessionManager.Unrevert(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.deps.Bus.Publish("session.unreverted", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionRewindMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	var body struct {
		MessageID string `json:"messageID"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.MessageID == "" {
		respondError(w, http.StatusBadRequest, "messageID is required")
		return
	}

	ms := s.messageStore()
	deleted, err := ms.DeleteAfter(sessionID, body.MessageID)
	if err != nil {
		if errors.Is(err, session.ErrMessageNotFound) {
			respondError(w, http.StatusNotFound, "message not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"sessionID":       sessionID,
		"deletedCount":    deleted,
		"rewindToMessage": body.MessageID,
	})
}

func (s *Server) handleSessionChildren(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	children, err := store.Children(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if children == nil {
		children = []session.Info{}
	}

	respondJSON(w, http.StatusOK, children)
}

func (s *Server) handleSessionTodo(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	ms := s.messageStore()
	messages, err := ms.List(sessionID)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{"todos": []any{}})
		return
	}

	type todoItem struct {
		Text      string `json:"text"`
		MessageID string `json:"messageID"`
		Line      int    `json:"line"`
	}

	var todos []todoItem
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Type != session.PartText || part.Text == "" {
				continue
			}
			lines := strings.Split(part.Text, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				upper := strings.ToUpper(trimmed)
				if !strings.Contains(upper, "TODO") && !strings.Contains(upper, "FIXME") {
					continue
				}
				todos = append(todos, todoItem{
					Text:      trimmed,
					MessageID: msg.ID,
					Line:      i + 1,
				})
			}
		}
	}

	if todos == nil {
		todos = []todoItem{}
	}
	respondJSON(w, http.StatusOK, map[string]any{"todos": todos})
}

func (s *Server) handleSessionDiff(w http.ResponseWriter, r *http.Request) {
	dir := s.config.Directory
	if id := r.PathValue("id"); id != "" {
		store := s.sessionStore()
		if info, err := store.Get(id); err == nil && info.Directory != "" {
			dir = info.Directory
		}
	}

	// Summary stats come from numstat (same source as session.diff events).
	files, additions, deletions, numErr := vcs.GitDiffNumstat(dir)
	diff, diffErr := vcs.GitDiff(dir)
	if numErr != nil && diffErr != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"diff":    "",
			"files":   []any{},
			"summary": map[string]any{"additions": 0, "deletions": 0, "files": 0},
			"error":   diffErr.Error(),
		})
		return
	}
	if files == nil {
		files = []string{}
	}
	if diffErr != nil {
		diff = ""
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"diff":  diff,
		"files": files,
		"summary": map[string]any{
			"additions": additions,
			"deletions": deletions,
			"files":     len(files),
		},
	})
}

func parseDiffStats(diff string) (files []string, additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				file := strings.TrimPrefix(parts[3], "b/")
				files = append(files, file)
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	if files == nil {
		files = []string{}
	}
	return
}

func (s *Server) handleSessionPermissionReply(w http.ResponseWriter, r *http.Request) {
	permissionID := r.PathValue("permissionID")
	var body struct {
		Action   string `json:"action"`
		Reply    string `json:"reply"`
		Response string `json:"response"`
		Message  string `json:"message,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The SDK sends "reply" or "response" (once/always/reject); normalize to "action".
	action := body.Action
	if action == "" {
		action = body.Reply
	}
	if action == "" {
		action = body.Response
	}
	if action == "" {
		respondError(w, http.StatusBadRequest, "reply, response, or action is required")
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

	sessionID := r.PathValue("sessionID")
	s.publishPermissionReply(permissionID, sessionID, reply, body.Message)

	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": permissionID,
		"status":       "replied",
	})
}
