package server

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/skill"
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

// promptResult holds the outputs of buildPromptSystemPrompt.
type promptResult struct {
	AgentPerms    []string
	Ruleset       permission.Ruleset
	SystemPrompt  string
	PrefetchPaths []string // absolute paths of prefetched files to mark as read
}

// buildPromptSystemPrompt resolves the agent, wires instructions, and builds
// the system prompt for a prompt request. directory is the session working
// directory; when empty, sm.dir is used. When withNotes is true, model-written
// notes from prior sessions are loaded into the prompt.
func (sm *SessionManager) buildPromptSystemPrompt(input PromptInput, model *provider.Model, directory string, withNotes bool) promptResult {
	agentInfo := sm.agentRegistry.Get(input.Agent, model.SizeB())
	var agentPrompt string
	var ruleset permission.Ruleset
	var agentPerms []string
	tools := sm.tools
	if agentInfo != nil {
		agentPrompt = agentInfo.Prompt
		ruleset = agentInfo.Permission
		agentPerms = extractAllowedPerms(agentInfo.Permission)
		tools = tools.WithAgentRules(agentInfo.Permission)
		slog.Info("agent loaded", "agent", input.Agent, "promptLen", len(agentPrompt), "compact", agentInfo.Compact)
	} else {
		slog.Warn("agent not found in registry", "agent", input.Agent)
	}

	var instructions string
	if sm.cfg != nil && len(sm.cfg.Instructions) > 0 {
		instructions = strings.Join(sm.cfg.Instructions, "\n\n")
	}

	if directory == "" {
		directory = sm.dir
	}

	spi := session.SystemPromptInput{
		AgentPrompt:        agentPrompt,
		AgentName:          input.Agent,
		Instructions:       instructions,
		Directory:          directory,
		ConfigDir:          config.ConfigDir(),
		ToolDefs:           tools.ToolDefs(agentPerms),
		AppendSystemPrompt: sm.appendSystemPrompt,
	}

	if withNotes && sm.notesDir != "" {
		spi.NotesDir = sm.notesDir
		spi.NotesEnabled = true
	}

	systemPrompt := session.BuildSystemPrompt(spi)

	// Prefetch project files when the agent requests it.
	var prefetchPaths []string
	if agentInfo != nil && agentInfo.Prefetch != "" {
		maxBytes := agentInfo.PrefetchMaxBytes
		prefetchText, paths := session.PrefetchFiles(directory, agentInfo.Prefetch, maxBytes)
		if prefetchText != "" {
			systemPrompt += "\n\n---\n\n" + prefetchText
			prefetchPaths = paths
			slog.Info("prefetch injected", "agent", input.Agent, "files", len(paths))
		}
	}

	slog.Info("system prompt built", "sessionID", input.SessionID, "agent", input.Agent, "promptLen", len(systemPrompt))
	return promptResult{
		AgentPerms:    agentPerms,
		Ruleset:       ruleset,
		SystemPrompt:  systemPrompt,
		PrefetchPaths: prefetchPaths,
	}
}

// persistPromptResult saves new messages and token usage to the database.
// When compaction occurred, the full compacted history replaces the prior rows.
// ReplaceAll and Append failures are returned as errors; AddUsage failures are logged only.
func (sm *SessionManager) persistPromptResult(result *session.ProcessResult, existingMsgs []session.Message, ms *session.MessageStore, sessionID string) error {
	if result == nil {
		return nil
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

	applyIDMap := func(msg *session.Message) {
		if idMap == nil {
			return
		}
		if bridgeID, ok := idMap[msg.ID]; ok {
			msg.ID = bridgeID
		}
	}

	if result.Compacted {
		msgs := make([]session.Message, len(result.Messages))
		copy(msgs, result.Messages)
		for i := range msgs {
			applyIDMap(&msgs[i])
		}
		if err := ms.ReplaceAll(sessionID, msgs); err != nil {
			return fmt.Errorf("persist compacted messages: %w", err)
		}
	} else if len(result.Messages) > len(existingMsgs) {
		newMsgs := result.Messages[len(existingMsgs):]
		batch := make([]session.Message, len(newMsgs))
		for i := range newMsgs {
			batch[i] = newMsgs[i]
			applyIDMap(&batch[i])
		}
		if err := ms.AppendBatch(sessionID, batch); err != nil {
			return fmt.Errorf("persist messages: %w", err)
		}
	}

	store := session.NewStore(sm.db)
	if err := store.AddUsage(sessionID, 0, session.TokenUsage{
		Input:     result.Usage.Input,
		Output:    result.Usage.Output,
		Reasoning: result.Usage.Reasoning,
		Cache:     result.Usage.Cache,
	}); err != nil {
		slog.Error("failed to add session usage", "error", err, "sessionID", sessionID)
	}
	return nil
}

// resolveAgentLLMParams returns Temperature/TopP/MaxOutputTokens preferring
// agent frontmatter when set, otherwise global config defaults.
func (sm *SessionManager) resolveAgentLLMParams(agentInfo *agent.Info) (temp, topP *float64, maxTokens *int) {
	if sm.cfg != nil {
		temp = sm.cfg.Temperature
		topP = sm.cfg.TopP
	}
	if agentInfo != nil {
		if agentInfo.Temperature != nil {
			temp = agentInfo.Temperature
		}
		if agentInfo.TopP != nil {
			topP = agentInfo.TopP
		}
		if agentInfo.MaxOutputTokens != nil {
			maxTokens = agentInfo.MaxOutputTokens
		}
	}
	return temp, topP, maxTokens
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

const (
	maxAgentSkills    = 5
	maxAgentSkillSize = 16 * 1024 // 16 KiB combined
)

// loadAgentSkills resolves agent-scoped skill names to their content and
// returns a combined string to append to the system prompt. Missing skills
// produce a one-line note; they don't fail the task. At most 5 skills and
// 16 KiB total content are loaded.
func (sm *SessionManager) loadAgentSkills(names []string) string {
	if len(names) == 0 {
		return ""
	}

	var skillPaths []string
	if sm.cfg != nil && sm.cfg.Skills != nil {
		skillPaths = sm.cfg.Skills.Paths
	}
	discovered := skill.DiscoverWithPaths(config.ConfigDir(), sm.dir, skillPaths)
	byName := make(map[string]skill.Skill, len(discovered))
	for _, s := range discovered {
		byName[s.Name] = s
	}

	var parts []string
	var totalSize int
	loaded := 0
	for _, name := range names {
		if loaded >= maxAgentSkills {
			break
		}
		s, ok := byName[name]
		if !ok {
			parts = append(parts, fmt.Sprintf("[skill %q not found]", name))
			continue
		}
		body, err := skill.LoadContent(s)
		if err != nil {
			parts = append(parts, fmt.Sprintf("[skill %q: load error: %v]", name, err))
			continue
		}
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		if totalSize+len(body) > maxAgentSkillSize {
			parts = append(parts, fmt.Sprintf("[skill %q: skipped, would exceed 16 KiB cap]", name))
			break
		}
		parts = append(parts, fmt.Sprintf("<skill name=%q>\n%s\n</skill>", name, body))
		totalSize += len(body)
		loaded++
	}
	if len(parts) == 0 {
		return ""
	}
	return "[Agent Skills]\n" + strings.Join(parts, "\n\n")
}

// isDefaultTitle returns true if the title is an auto-generated default.
func isDefaultTitle(title string) bool {
	return title == "" ||
		title == "New Session" ||
		strings.HasPrefix(title, "New session - ") ||
		strings.HasPrefix(title, "Child session - ")
}
