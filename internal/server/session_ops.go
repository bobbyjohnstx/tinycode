package server

import (
	"context"
	"log/slog"

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

	model, err := sm.resolvePromptModel(sessionID, input)
	if err != nil {
		return
	}

	agentPerms, systemPrompt := sm.buildPromptSystemPrompt(input, model)

	if sm.mcpSvc != nil {
		mcpTools := sm.mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			sm.tools.Register(def)
		}
	}

	ms := session.NewMessageStore(session.NewStore(sm.db))
	existingMsgs, err := ms.List(sessionID)
	if err != nil {
		slog.Warn("failed to load existing messages, starting with empty history", "sessionID", sessionID, "error", err)
		existingMsgs = nil
	}

	subagentDepth := 1
	if sm.cfg != nil && sm.cfg.SubagentDepth != nil {
		subagentDepth = *sm.cfg.SubagentDepth
	}

	client := sm.clientFactory(model)
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:     sessionID,
		Agent:         input.Agent,
		Model:         model,
		SubagentDepth: subagentDepth,
		SystemPrompt:  systemPrompt,
		Compaction:    sm.buildCompactionConfig(),
		AgentPerms:    agentPerms,
	}, client, sm.tools, sm.bus)
	proc.SetMessages(existingMsgs)

	sm.mu.Lock()
	if active, ok := sm.sessions[sessionID]; ok {
		active.processor = proc
		active.model = model
		active.agent = input.Agent
	}
	sm.mu.Unlock()

	result := proc.Process(ctx, userText)

	sm.persistPromptResult(result, existingMsgs, ms, sessionID)

	if result != nil && result.Error != nil {
		slog.Error("processor error", "error", result.Error, "sessionID", sessionID)
		sm.bus.Publish("session.error", map[string]any{
			"sessionID": sessionID,
			"error":     result.Error.Error(),
		})
	}
}
