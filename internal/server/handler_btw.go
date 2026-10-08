package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
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

	// Prefer small_model for side questions when configured.
	modelSource := "session"
	if cfg := s.sessionManager.cfg; cfg != nil && cfg.SmallModel != "" {
		pid, mid := parseModelRef(cfg.SmallModel)
		if sm, smErr := s.resolveSmallModel(pid, mid); smErr == nil {
			providerModel = sm
			modelSource = "small_model"
		}
		// Fall through to session model if small_model can't be resolved.
	}
	slog.Info("btw model routing", "question", question, "model", providerModel.ID, "provider", providerModel.ProviderID, "source", modelSource)

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

// parseModelRef splits a "provider/model" reference into provider and model IDs.
// If no slash is present, the entire string is returned as the model ID.
func parseModelRef(ref string) (providerID, modelID string) {
	if i := strings.Index(ref, "/"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// resolveSmallModel looks up a model by provider and model ID. When providerID
// is empty it searches all registered providers for a matching model.
func (s *Server) resolveSmallModel(providerID, modelID string) (*provider.Model, error) {
	if providerID != "" {
		return s.deps.Registry.GetModel(providerID, modelID)
	}
	for _, p := range s.deps.Registry.ListProviders() {
		if m, ok := p.Models[modelID]; ok {
			return m, nil
		}
	}
	return nil, fmt.Errorf("model %q not found in any provider", modelID)
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
