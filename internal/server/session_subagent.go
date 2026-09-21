package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

const (
	subagentMaxIterations = 5
	subagentTimeout       = 2 * time.Minute
)

// RunSubagent executes a prompt in a child session and returns the assistant's
// text response. This is called by the task tool to implement /swarm and other
// subagent-spawning commands.
func (sm *SessionManager) RunSubagent(ctx context.Context, parentSessionID, prompt, agent, directory string) (string, error) {
	if directory == "" {
		directory = sm.dir
	}

	// Resolve the model: look up the parent session's model from DB,
	// then fall back to the first connected provider's default.
	var modelRef *session.ModelRef
	if parentSessionID != "" {
		store := session.NewStore(sm.db)
		if info, err := store.Get(parentSessionID); err == nil && info.Model != nil {
			modelRef = info.Model
		}
	}
	if modelRef == nil {
		models := sm.registry.ListModels()
		if len(models) > 0 {
			modelRef = &session.ModelRef{ProviderID: models[0].ProviderID, ID: models[0].ID}
		}
	}
	if modelRef == nil {
		return "", fmt.Errorf("no model available for subagent — configure a default model")
	}

	model, err := sm.registry.GetModel(modelRef.ProviderID, modelRef.ID)
	if err != nil {
		return "", fmt.Errorf("cannot resolve model %s/%s for subagent: %w", modelRef.ProviderID, modelRef.ID, err)
	}

	// Build system prompt for the subagent.
	input := PromptInput{
		SessionID: parentSessionID,
		Agent:     agent,
	}
	agentPerms, systemPrompt := sm.buildPromptSystemPrompt(input, model)

	client := sm.clientFactory(model)
	tools := sm.tools
	if sm.toolSnapshot != nil {
		tools = sm.toolSnapshot
	}

	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:     fmt.Sprintf("%s:sub", parentSessionID),
		Agent:         agent,
		Model:         model,
		SystemPrompt:  systemPrompt,
		AgentPerms:    agentPerms,
		Directory:     directory,
		MaxIterations: subagentMaxIterations,
	}, client, tools, sm.bus)

	subCtx, cancel := context.WithTimeout(ctx, subagentTimeout)
	defer cancel()

	slog.Info("subagent started", "parent", parentSessionID, "agent", agent, "model", model.ID, "maxIter", subagentMaxIterations, "timeout", subagentTimeout)

	result := proc.Process(subCtx, prompt)
	if result == nil {
		return "", fmt.Errorf("subagent returned nil result")
	}
	if result.Error != nil {
		return "", fmt.Errorf("subagent error: %w", result.Error)
	}

	// Extract assistant text from result messages.
	var texts []string
	for _, msg := range result.Messages {
		if msg.Role == "assistant" {
			for _, part := range msg.Parts {
				if part.Type == "text" && part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
		}
	}

	response := strings.Join(texts, "\n\n")
	slog.Info("subagent completed", "parent", parentSessionID, "agent", agent, "responseLen", len(response))
	return response, nil
}
