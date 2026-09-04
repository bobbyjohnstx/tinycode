package server

import (
	"context"
	"net/http"
	"strconv"

	id2 "github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

func (s *Server) sessionStore() *session.Store {
	return session.NewStore(s.deps.DB)
}

func (s *Server) messageStore() *session.MessageStore {
	return session.NewMessageStore(s.sessionStore())
}

func (s *Server) partStore() *session.PartStore {
	return session.NewPartStore(s.deps.DB)
}

func (s *Server) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParentID string            `json:"parentID,omitempty"`
		Title    string            `json:"title,omitempty"`
		Agent    string            `json:"agent,omitempty"`
		Model    *session.ModelRef `json:"model,omitempty"`
	}

	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Assign a default agent if none was provided.
	if body.Agent == "" {
		if s.config.DefaultAgent != "" {
			body.Agent = s.config.DefaultAgent
		} else {
			body.Agent = "build"
		}
	}

	// Assign a default model if none was provided.
	if body.Model == nil {
		body.Model = s.resolveDefaultModel()
	}

	dir := r.URL.Query().Get("directory")
	if dir == "" {
		dir = s.config.Directory
	}

	projectID := project.IDFromDirectory(dir)

	store := s.sessionStore()
	info, err := store.Create(session.CreateInput{
		ProjectID: projectID,
		Directory: dir,
		ParentID:  body.ParentID,
		Title:     body.Title,
		Agent:     body.Agent,
		Model:     body.Model,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.deps.Bus.Publish("session.created", map[string]any{
		"sessionID": info.ID,
		"info":      info,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("directory")
	if dir == "" {
		dir = s.config.Directory
	}

	projectID := project.IDFromDirectory(dir)

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	store := s.sessionStore()
	sessions, err := store.List(projectID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []session.Info{}
	}

	respondJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleSessionUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Title string `json:"title,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	store := s.sessionStore()

	if body.Title != "" {
		if err := store.UpdateTitle(id, body.Title); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	if err := store.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.deps.Bus.Publish("session.deleted", map[string]any{
		"sessionID": id,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Content == "" {
		respondError(w, http.StatusBadRequest, "content is required")
		return
	}

	s.deps.Bus.Publish("session.prompt", map[string]any{
		"sessionID": id,
		"content":   body.Content,
	})

	respondJSON(w, http.StatusAccepted, map[string]any{
		"sessionID": id,
		"status":    "processing",
	})
}

func (s *Server) handleSessionPromptAsync(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	var body struct {
		MessageID string       `json:"messageID,omitempty"`
		Model     *promptModel `json:"model,omitempty"`
		Agent     string       `json:"agent,omitempty"`
		Parts     []promptPart `json:"parts"`
		Variant   string       `json:"variant,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.sessionManager.StartPrompt(context.Background(), PromptInput{
		SessionID: sessionID,
		Model:     body.Model,
		Agent:     body.Agent,
		Parts:     body.Parts,
		MessageID: body.MessageID,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionAbort(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.sessionManager.Abort(id)

	s.deps.Bus.Publish("session.abort", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"sessionID": id,
		"status":    "aborted",
	})
}

func (s *Server) handleSessionFork(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		MessageID string `json:"messageID,omitempty"`
		Title     string `json:"title,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	store := s.sessionStore()
	parent, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	forked, err := store.Create(session.CreateInput{
		ProjectID: parent.ProjectID,
		Directory: parent.Directory,
		ParentID:  id,
		Title:     body.Title,
		Agent:     parent.Agent,
		Model:     parent.Model,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Copy messages from parent to forked session.
	ms := s.messageStore()
	parentMsgs, err := ms.List(id)
	if err == nil {
		for i := range parentMsgs {
			msg := parentMsgs[i]
			msg.SessionID = forked.ID
			newID, idErr := id2.Ascending("message")
			if idErr != nil {
				continue
			}
			msg.ID = newID
			_ = ms.Append(&msg)
		}
	}

	respondJSON(w, http.StatusCreated, forked)
}

// resolveDefaultModel returns a ModelRef using the configured default model or,
// failing that, the first model from the first available provider.
func (s *Server) resolveDefaultModel() *session.ModelRef {
	// 1. Check configured default model (e.g. "ollama/llama3.2").
	if s.config.DefaultModel != "" {
		providerID, modelID := provider.ParseModel(s.config.DefaultModel)
		if providerID != "" && modelID != "" {
			return &session.ModelRef{ProviderID: providerID, ID: modelID}
		}
		// If no slash separator, modelID is the whole string; skip — we need both.
	}

	// 2. Pick the first model from the first available provider.
	providers := s.deps.Registry.ListProviders()
	for _, p := range providers {
		for _, m := range p.Models {
			return &session.ModelRef{ProviderID: p.ID, ID: m.ID}
		}
	}

	return nil
}

func (s *Server) handleMessageList(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	ms := s.messageStore()
	ps := s.partStore()

	messages, err := ms.List(sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		storedParts, _ := ps.ListByMessage(m.ID)

		var parts any
		if len(storedParts) > 0 {
			parts = storedParts
		} else if len(m.Parts) > 0 {
			parts = m.Parts
		} else {
			parts = []session.StoredPart{}
		}

		result = append(result, map[string]any{
			"info":  m,
			"parts": parts,
		})
	}

	respondJSON(w, http.StatusOK, result)
}
