package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

// RunSubagent executes a prompt in a child session and returns the assistant's
// text response. This is called by the task tool to implement /swarm and other
// subagent-spawning commands.
func (sm *SessionManager) RunSubagent(ctx context.Context, parentSessionID, prompt, agent, directory string) (string, error) {
	if directory == "" {
		directory = sm.dir
	}

	// Resolve the model from the parent session or defaults.
	model, err := sm.resolvePromptModel(parentSessionID, PromptInput{
		SessionID: parentSessionID,
		Agent:     agent,
	})
	if err != nil {
		return "", fmt.Errorf("cannot resolve model for subagent: %w", err)
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
		SessionID:    fmt.Sprintf("%s:sub", parentSessionID),
		Agent:        agent,
		Model:        model,
		SystemPrompt: systemPrompt,
		AgentPerms:   agentPerms,
		Directory:    directory,
	}, client, tools, sm.bus)

	slog.Info("subagent started", "parent", parentSessionID, "agent", agent, "model", model.ID)

	result := proc.Process(ctx, prompt)
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
