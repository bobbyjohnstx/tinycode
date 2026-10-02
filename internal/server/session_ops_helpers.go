package server

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// sessionErrorPayload builds a structured error object matching the SDK contract:
// { "name": "ErrorType", "data": { "message": "..." } }.
func sessionErrorPayload(name, message string) map[string]any {
	return map[string]any{
		"name": name,
		"data": map[string]any{
			"message": message,
		},
	}
}

// resolvePromptModel looks up the model from the registry. Returns (nil, error)
// if the model is not found or not specified.
func (sm *SessionManager) resolvePromptModel(sessionID string, input PromptInput) (*provider.Model, error) {
	if input.Model != nil {
		m, err := sm.registry.GetModel(input.Model.ProviderID, input.Model.ModelID)
		if err != nil {
			slog.Error("model not found", "provider", input.Model.ProviderID, "model", input.Model.ModelID, "error", err)
			sm.bus.Publish("session.error", map[string]any{
				"sessionID": sessionID,
				"error":     sessionErrorPayload("ProviderAuthError", fmt.Sprintf("model %s/%s not found: %v", input.Model.ProviderID, input.Model.ModelID, err)),
			})
			return nil, err
		}
		return m, nil
	}
	errMsg := "no model specified for prompt — configure a default model or select one when creating the session"
	slog.Error(errMsg, "sessionID", sessionID)
	sm.bus.Publish("session.error", map[string]any{
		"sessionID": sessionID,
		"error":     sessionErrorPayload("ProviderAuthError", errMsg),
	})
	return nil, fmt.Errorf("%s", errMsg)
}

// buildPromptSystemPrompt resolves the agent, wires instructions, and builds
// the system prompt for a prompt request.
func (sm *SessionManager) buildPromptSystemPrompt(input PromptInput, model *provider.Model) (agentPerms []string, systemPrompt string) {
	agentInfo := sm.agentRegistry.Get(input.Agent, model.SizeB())
	var agentPrompt string
	if agentInfo != nil {
		agentPrompt = agentInfo.Prompt
		agentPerms = extractAllowedPerms(agentInfo.Permission)
		slog.Info("agent loaded", "agent", input.Agent, "promptLen", len(agentPrompt), "compact", agentInfo.Compact)
	} else {
		slog.Warn("agent not found in registry", "agent", input.Agent)
	}

	var instructions string
	if sm.cfg != nil && len(sm.cfg.Instructions) > 0 {
		instructions = strings.Join(sm.cfg.Instructions, "\n\n")
	}

	systemPrompt = session.BuildSystemPrompt(session.SystemPromptInput{
		AgentPrompt:        agentPrompt,
		Instructions:       instructions,
		Directory:          sm.dir,
		ToolDefs:           sm.tools.ToolDefs(agentPerms),
		AppendSystemPrompt: sm.appendSystemPrompt,
	})

	slog.Info("system prompt built", "sessionID", input.SessionID, "agent", input.Agent, "promptLen", len(systemPrompt))
	return agentPerms, systemPrompt
}

// persistPromptResult saves new messages and token usage to the database.
func (sm *SessionManager) persistPromptResult(result *session.ProcessResult, existingMsgs []session.Message, ms *session.MessageStore, sessionID string) {
	if result == nil || len(result.Messages) <= len(existingMsgs) {
		return
	}

	sm.mu.Lock()
	active := sm.sessions[sessionID]
	var idMap map[string]string
	if active != nil {
		active.mu.Lock()
		if len(active.idMap) > 0 {
			idMap = make(map[string]string, len(active.idMap))
			for k, v := range active.idMap {
				idMap[k] = v
			}
		}
		active.mu.Unlock()
	}
	sm.mu.Unlock()

	newMsgs := result.Messages[len(existingMsgs):]
	for i := range newMsgs {
		msg := newMsgs[i]
		if idMap != nil {
			if bridgeID, ok := idMap[msg.ID]; ok {
				msg.ID = bridgeID
			}
		}
		if err := ms.Append(&msg); err != nil {
			slog.Error("failed to persist message", "error", err, "role", msg.Role, "sessionID", sessionID)
		}
	}

	store := session.NewStore(sm.db)
	_ = store.UpdateCost(sessionID, 0, session.TokenUsage{
		Input:  result.Usage.Input,
		Output: result.Usage.Output,
	})
}

// buildCompactionConfig creates a compaction config, applying any overrides from the app config.
func (sm *SessionManager) buildCompactionConfig() session.CompactionConfig {
	compactionCfg := session.DefaultCompactionConfig()
	if sm.cfg != nil && sm.cfg.Compaction != nil {
		if sm.cfg.Compaction.MaskObservations != nil {
			compactionCfg.MaskObservations = *sm.cfg.Compaction.MaskObservations
		}
		if sm.cfg.Compaction.PreserveRecentTokens != nil {
			compactionCfg.MaxPreserve = *sm.cfg.Compaction.PreserveRecentTokens
		}
		if sm.cfg.Compaction.MaxMessages != nil && *sm.cfg.Compaction.MaxMessages > 0 {
			compactionCfg.MaxMessages = *sm.cfg.Compaction.MaxMessages
		}
	}
	return compactionCfg
}

// autoTitle generates a session title from the user's first prompt.
func autoTitle(text string) string {
	title := strings.Join(strings.Fields(text), " ")
	// Strip leading slash commands
	if strings.HasPrefix(title, "/") {
		if idx := strings.Index(title, " "); idx > 0 {
			rest := strings.TrimSpace(title[idx:])
			if rest != "" {
				title = rest
			}
		}
	}
	if len(title) > 60 {
		cut := strings.LastIndex(title[:60], " ")
		if cut < 30 {
			cut = 60
		}
		title = title[:cut] + "..."
	}
	return title
}

// isDefaultTitle returns true if the title is an auto-generated default.
func isDefaultTitle(title string) bool {
	return title == "" ||
		title == "New Session" ||
		strings.HasPrefix(title, "New session - ") ||
		strings.HasPrefix(title, "Child session - ")
}
