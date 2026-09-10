package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

// PromptInput describes a user prompt to be processed by a session.
type PromptInput struct {
	SessionID string
	Model     *promptModel
	Agent     string
	Parts     []promptPart
	MessageID string
}

type promptModel struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type promptPart struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Text    string `json:"text,omitempty"`
}

func (sm *SessionManager) subscribeCommands() {
	sub := sm.bus.Subscribe("session.command")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			command, _ := props["command"].(string)
			args, _ := props["args"].(string)
			sm.handleCommand(sessionID, command, args)
		}
	}()
}

func (sm *SessionManager) handleCommand(sessionID, command, args string) {
	switch command {
	case "abort":
		sm.Abort(sessionID)
	default:
		content := "/" + command
		if args != "" {
			content += " " + args
		}
		sm.bus.Publish("session.prompt", map[string]any{
			"sessionID": sessionID,
			"content":   content,
		})
	}
}

func (sm *SessionManager) subscribePrompts() {
	sub := sm.bus.Subscribe("session.prompt")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			content, _ := props["content"].(string)
			if sessionID == "" || content == "" {
				continue
			}
			store := session.NewStore(sm.db)
			info, err := store.Get(sessionID)
			if err != nil || info.Model == nil {
				continue
			}
			sm.StartPrompt(sm.ctx, PromptInput{
				SessionID: sessionID,
				Model: &promptModel{
					ProviderID: info.Model.ProviderID,
					ModelID:    info.Model.ID,
				},
				Agent: info.Agent,
				Parts: []promptPart{{Type: "text", Text: content}},
			})
		}
	}()
}

func (sm *SessionManager) Abort(sessionID string) {
	sm.mu.Lock()
	active, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return
	}
	cancel := active.cancel
	proc := active.processor
	sm.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if proc != nil {
		proc.Abort()
	}
}

func (sm *SessionManager) StartPrompt(ctx context.Context, input PromptInput) {
	sm.mu.Lock()
	var oldDone chan struct{}
	if active, ok := sm.sessions[input.SessionID]; ok {
		if active.cancel != nil {
			active.cancel()
		}
		oldDone = active.done
	}
	done := make(chan struct{})
	pctx, cancel := context.WithCancel(ctx)
	sm.sessions[input.SessionID] = &activeSession{
		cancel: cancel,
		done:   done,
		idMap:  make(map[string]string),
	}
	sm.mu.Unlock()

	if oldDone != nil {
		<-oldDone
	}

	go sm.processPrompt(pctx, input, done)
}

func (sm *SessionManager) processPrompt(ctx context.Context, input PromptInput, done chan struct{}) {
	defer close(done)
	sessionID := input.SessionID
	defer func() {
		sm.mu.Lock()
		delete(sm.sessions, sessionID)
		sm.mu.Unlock()

		tool.ClearFileMutexes()

		sm.bus.Publish("session.status", map[string]any{
			"sessionID": sessionID,
			"status":    map[string]any{"alert": false, "working": false},
		})
	}()

	sm.bus.Publish("session.status", map[string]any{
		"sessionID": sessionID,
		"status":    map[string]any{"alert": false, "working": true},
	})

	// Extract user text from parts
	var userText string
	for _, p := range input.Parts {
		if p.Type == "text" {
			if p.Text != "" {
				userText += p.Text
			} else if p.Content != "" {
				userText += p.Content
			}
		}
	}
	if userText == "" {
		return
	}

	// Resolve model
	var model *provider.Model
	if input.Model != nil {
		m, err := sm.registry.GetModel(input.Model.ProviderID, input.Model.ModelID)
		if err != nil {
			slog.Error("model not found", "provider", input.Model.ProviderID, "model", input.Model.ModelID, "error", err)
			sm.bus.Publish("session.error", map[string]any{
				"sessionID": sessionID,
				"error":     fmt.Sprintf("model %s/%s not found: %v", input.Model.ProviderID, input.Model.ModelID, err),
			})
			return
		}
		model = m
	}
	if model == nil {
		errMsg := "no model specified for prompt — configure a default model or select one when creating the session"
		slog.Error(errMsg, "sessionID", sessionID)
		sm.bus.Publish("session.error", map[string]any{
			"sessionID": sessionID,
			"error":     errMsg,
		})
		return
	}

	// Look up the agent
	agentInfo := sm.agentRegistry.Get(input.Agent, model.SizeB())
	var agentPrompt string
	var agentPerms []string
	if agentInfo != nil {
		agentPrompt = agentInfo.Prompt
		agentPerms = extractAllowedPerms(agentInfo.Permission)
		slog.Info("agent loaded", "agent", input.Agent, "promptLen", len(agentPrompt), "compact", agentInfo.Compact)
	} else {
		slog.Warn("agent not found in registry", "agent", input.Agent)
	}

	// Wire user instructions from config
	var instructions string
	if sm.cfg != nil && len(sm.cfg.Instructions) > 0 {
		instructions = strings.Join(sm.cfg.Instructions, "\n\n")
	}

	// Build system prompt
	systemPrompt := session.BuildSystemPrompt(session.SystemPromptInput{
		AgentPrompt:  agentPrompt,
		Instructions: instructions,
		Directory:    sm.dir,
		ToolDefs:     sm.tools.ToolDefs(agentPerms),
	})

	slog.Info("system prompt built", "sessionID", sessionID, "agent", input.Agent, "promptLen", len(systemPrompt))

	// Sync MCP tools before processing
	if sm.mcpSvc != nil {
		mcpTools := sm.mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			sm.tools.Register(def)
		}
	}

	// Load existing messages for this session
	ms := session.NewMessageStore(session.NewStore(sm.db))
	existingMsgs, err := ms.List(sessionID)
	if err != nil {
		slog.Warn("failed to load existing messages, starting with empty history", "sessionID", sessionID, "error", err)
		existingMsgs = nil
	}

	// Wire SubagentDepth from config
	subagentDepth := 1
	if sm.cfg != nil && sm.cfg.SubagentDepth != nil {
		subagentDepth = *sm.cfg.SubagentDepth
	}

	// Wire Compaction config
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

	// Create LLM client and Processor
	client := sm.clientFactory(model)
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:     sessionID,
		Agent:         input.Agent,
		Model:         model,
		SubagentDepth: subagentDepth,
		SystemPrompt:  systemPrompt,
		Compaction:    compactionCfg,
		AgentPerms:    agentPerms,
	}, client, sm.tools, sm.bus)
	proc.SetMessages(existingMsgs)

	// Store processor reference and metadata so the event bridge can use them
	sm.mu.Lock()
	if active, ok := sm.sessions[sessionID]; ok {
		active.processor = proc
		active.model = model
		active.agent = input.Agent
	}
	sm.mu.Unlock()

	// Run the processor (blocks until complete)
	result := proc.Process(ctx, userText)

	// Persist new messages to the database
	if result != nil && len(result.Messages) > len(existingMsgs) {
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
			// Remap assistant message IDs to bridge-generated IDs for TUI consistency
			if idMap != nil {
				if bridgeID, ok := idMap[msg.ID]; ok {
					msg.ID = bridgeID
				}
			}
			if err := ms.Append(&msg); err != nil {
				slog.Error("failed to persist message", "error", err, "role", msg.Role, "sessionID", sessionID)
			}
		}

		// Update session token usage
		// TODO: cost is always 0 — local models have no pricing data.
		// Wire cost calculation when provider-specific pricing info is available.
		store := session.NewStore(sm.db)
		_ = store.UpdateCost(sessionID, 0, session.TokenUsage{
			Input:  result.Usage.Input,
			Output: result.Usage.Output,
		})
	}

	// Report errors
	if result != nil && result.Error != nil {
		slog.Error("processor error", "error", result.Error, "sessionID", sessionID)
		sm.bus.Publish("session.error", map[string]any{
			"sessionID": sessionID,
			"error":     result.Error.Error(),
		})
	}
}
