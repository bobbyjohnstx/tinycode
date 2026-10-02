package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

const (
	btwMaxContextMessages = 10
	btwTimeout            = 30 * time.Second
)

func (s *Server) handleSessionBtw(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	var body struct {
		Question string       `json:"question"`
		Model    *promptModel `json:"model,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	question := strings.TrimSpace(body.Question)
	if question == "" {
		respondError(w, http.StatusBadRequest, "question is required")
		return
	}

	// Resolve model from request body or session store.
	model := body.Model
	store := s.sessionStore()
	info, err := store.Get(sessionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}
	if model == nil && info.Model != nil {
		model = &promptModel{
			ProviderID: info.Model.ProviderID,
			ModelID:    info.Model.ModelID,
		}
	}
	if model == nil {
		respondError(w, http.StatusBadRequest, "no model available for side question")
		return
	}

	providerModel, err := s.deps.Registry.GetModel(model.ProviderID, model.ModelID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "model not found")
		return
	}

	// Load recent conversation messages for context.
	ms := s.messageStore()
	messages, err := ms.List(sessionID)
	if err != nil {
		messages = nil
	}

	contextSummary := buildBtwContext(messages)

	// Build one-shot LLM request.
	systemMsg := "You are answering a brief side question from the user. " +
		"They are in the middle of a coding task and have a quick meta-question. " +
		"Answer concisely and directly. Do not use tools."

	var llmMessages []llm.Message
	llmMessages = append(llmMessages, llm.Message{Role: "system", Content: systemMsg})

	if contextSummary != "" {
		llmMessages = append(llmMessages, llm.Message{
			Role:    "user",
			Content: "Here is the recent conversation context:\n\n" + contextSummary,
		})
		llmMessages = append(llmMessages, llm.Message{
			Role:    "assistant",
			Content: "I've noted the conversation context. What is your side question?",
		})
	}

	llmMessages = append(llmMessages, llm.Message{Role: "user", Content: question})

	req := llm.Request{
		Model:    providerModel.ID,
		Messages: llmMessages,
	}

	client := s.sessionManager.clientFactory(providerModel)

	ctx, cancel := context.WithTimeout(r.Context(), btwTimeout)
	defer cancel()

	ch, err := client.Stream(ctx, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "LLM request failed: "+err.Error())
		return
	}

	var parts []string
	for event := range ch {
		switch event.Type {
		case llm.EventTextDelta:
			parts = append(parts, event.Text)
		case llm.EventError:
			respondError(w, http.StatusInternalServerError, "LLM stream error: "+event.Error.Error())
			return
		}
	}

	answer := strings.Join(parts, "")

	respondJSON(w, http.StatusOK, map[string]string{
		"answer": answer,
	})
}

// buildBtwContext extracts a concise summary from the last N messages,
// including only text parts to keep the context small for side questions.
func buildBtwContext(messages []session.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// Take the last N messages.
	start := 0
	if len(messages) > btwMaxContextMessages {
		start = len(messages) - btwMaxContextMessages
	}
	recent := messages[start:]

	var sb strings.Builder
	for _, msg := range recent {
		if msg.Role == session.RoleTool {
			continue
		}
		var text string
		for _, part := range msg.Parts {
			if part.Type == session.PartText && part.Text != "" {
				text = part.Text
				break
			}
		}
		if text == "" {
			continue
		}

		// Truncate individual messages to keep context small.
		if len(text) > 500 {
			text = text[:500] + "..."
		}

		role := string(msg.Role)
		sb.WriteString("[" + role + "]: " + text + "\n\n")
	}

	return sb.String()
}
