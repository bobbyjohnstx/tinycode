package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

const (
	subagentMaxIterations = 5
	subagentTimeout       = 10 * time.Minute
)

var subagentSeq atomic.Int64

func nextSubagentLabel(agent string) string {
	n := subagentSeq.Add(1)
	return fmt.Sprintf("%s-%d", agent, n)
}

// RunSubagent executes a prompt in a child session and returns the assistant's
// text response. This is called by the task tool to implement /swarm and other
// subagent-spawning commands.
func (sm *SessionManager) RunSubagent(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (_ string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("subagent panic: %v", r)
			slog.Error("subagent panicked", "parent", parentSessionID, "agent", agent, "panic", r)
		}
	}()
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
			modelRef = &session.ModelRef{ProviderID: models[0].ProviderID, ModelID: models[0].ID}
		}
	}
	if modelRef == nil {
		return "", fmt.Errorf("no model available for subagent — configure a default model")
	}

	model, err := sm.registry.GetModel(modelRef.ProviderID, modelRef.ModelID)
	if err != nil {
		return "", fmt.Errorf("cannot resolve model %s/%s for subagent: %w", modelRef.ProviderID, modelRef.ModelID, err)
	}

	// Build system prompt for the subagent.
	input := PromptInput{
		SessionID: parentSessionID,
		Agent:     agent,
	}
	agentPerms, systemPrompt := sm.buildPromptSystemPrompt(input, model)

	client := sm.clientFactory(model)
	childTools := sm.tools
	if sm.toolSnapshot != nil {
		childTools = sm.toolSnapshot
	}
	childTools = childTools.WithDepth(parentDepth + 1)
	if autoApprove {
		childTools = childTools.WithAutoApprove()
	}

	// #230: Register MCP tools on the subagent's tool registry copy.
	// The toolSnapshot is captured before MCP tools are registered, so
	// subagents need to pick them up explicitly.
	if sm.mcpSvc != nil {
		mcpTools := sm.mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			childTools.Register(def)
		}
	}

	label := nextSubagentLabel(agent)
	subSessionID := fmt.Sprintf("%s:%s", parentSessionID, label)

	// Use agent Steps frontmatter if set, otherwise fall back to default.
	maxIter := subagentMaxIterations
	agentInfo := sm.agentRegistry.Get(agent, model.SizeB())
	if agentInfo != nil && agentInfo.Steps != nil && *agentInfo.Steps > 0 {
		maxIter = *agentInfo.Steps
	}

	// #233: Inherit parent LLM params from config.
	procCfg := session.ProcessorConfig{
		SessionID:     subSessionID,
		Agent:         agent,
		Model:         model,
		SystemPrompt:  systemPrompt,
		AgentPerms:    agentPerms,
		Perms:         sm.perms,
		Directory:     directory,
		MaxIterations: maxIter,
	}
	if sm.cfg != nil {
		procCfg.Temperature = sm.cfg.Temperature
		procCfg.TopP = sm.cfg.TopP
		procCfg.MaxTokens = sm.cfg.MaxTokens
	}
	proc := session.NewProcessor(procCfg, client, childTools, sm.bus)

	subCtx, cancel := context.WithTimeout(ctx, subagentTimeout)
	defer cancel()

	slog.Info("subagent started", "label", label, "parent", parentSessionID, "agent", agent, "model", model.ID)

	result := proc.Process(subCtx, prompt)
	if result == nil {
		return "", fmt.Errorf("subagent returned nil result")
	}
	// Iteration-limit errors are non-fatal: the subagent did useful work
	// (tool calls executed) but didn't produce a final summary within the
	// iteration budget. Extract whatever text it produced.
	if result.Error != nil && len(result.Messages) == 0 {
		return "", fmt.Errorf("subagent error: %w", result.Error)
	}
	if result.Error != nil {
		slog.Warn("subagent hit iteration limit, returning partial result", "label", label, "error", result.Error)
	}

	// #238: Track subagent token usage.
	if result.Usage.Input > 0 || result.Usage.Output > 0 {
		slog.Info("subagent tokens", "label", label, "agent", agent,
			"input", result.Usage.Input, "output", result.Usage.Output,
			"reasoning", result.Usage.Reasoning)
	}
	if sm.bus != nil {
		sm.bus.Publish("subagent.completed", map[string]any{
			"parentSessionID": parentSessionID,
			"label":           label,
			"agent":           agent,
			"inputTokens":     result.Usage.Input,
			"outputTokens":    result.Usage.Output,
			"reasoningTokens": result.Usage.Reasoning,
		})
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
	slog.Info("subagent completed", "label", label, "parent", parentSessionID, "agent", agent, "responseLen", len(response))
	return response, nil
}
