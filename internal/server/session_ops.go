package server

import (
	"context"
	"log/slog"

	"github.com/bobbyjohnstx/tinycode-go/internal/command"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"

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

		sm.bus.Publish("session.status", map[string]any{
			"sessionID": sessionID,
			"status":    map[string]any{"type": "idle"},
		})
	}()

	sm.bus.Publish("session.status", map[string]any{
		"sessionID": sessionID,
		"status":    map[string]any{"type": "busy"},
	})

	// Reset task round flag so this prompt gets a fresh round of task calls.
	if sm.tools != nil {
		sm.tools.ResetTaskRound()
	sm.tools.ResetBudget(20)
	}

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

	expandResult := command.ExpandSlashCommand(userText)
	llmText := expandResult.Text
	if expandResult.DisplayText != "" {
		userText = expandResult.DisplayText
	}

	model, err := sm.resolvePromptModel(sessionID, input)
	if err != nil {
		return
	}

	// Use the session's stored directory so prompts run in the correct project.
	sessionDir := sm.dir
	var isChild bool
	var currentTitle string
	{
		store := session.NewStore(sm.db)
		if info, err := store.Get(sessionID); err == nil {
			if info.Directory != "" {
				sessionDir = info.Directory
			}
			isChild = info.ParentID != ""
			currentTitle = info.Title
		}
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

	client := sm.clientFactory(model)
	sessionTools := sm.tools
	if expandResult.AutoApprove {
		sessionTools = sessionTools.WithAutoApprove().WithOnlyTools("task")
	}
	displayText := ""
	if userText != llmText {
		displayText = userText
	}
	maxIter := swarmMaxIterations(expandResult.AutoApprove)
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:       sessionID,
		Agent:           input.Agent,
		Model:           model,
		SystemPrompt:    systemPrompt,
		Compaction:      sm.buildCompactionConfig(),
		AgentPerms:      agentPerms,
		Perms:           sm.perms,
		UserDisplayText: displayText,
		MaxIterations:   maxIter,
	}, client, sessionTools, sm.bus)
	proc.SetMessages(existingMsgs)

	sm.mu.Lock()
	if active, ok := sm.sessions[sessionID]; ok {
		active.processor = proc
		active.model = model
		active.agent = input.Agent
		active.dir = sessionDir
	}
	sm.mu.Unlock()

	result := proc.Process(ctx, llmText)

	if len(existingMsgs) == 0 && !isChild {
		title := autoTitle(userText)
		if title != "" && isDefaultTitle(currentTitle) {
			store := session.NewStore(sm.db)
			if err := store.UpdateTitle(sessionID, title); err == nil {
				if updatedInfo, err := store.Get(sessionID); err == nil {
					sm.bus.Publish("session.updated", map[string]any{
						"sessionID": sessionID,
						"info":      updatedInfo,
					})
				}
			}
		}
	}

	sm.persistPromptResult(result, existingMsgs, ms, sessionID)

	if result != nil && result.Error != nil {
		slog.Error("processor error", "error", result.Error, "sessionID", sessionID)
		sm.bus.Publish("session.error", map[string]any{
			"sessionID": sessionID,
			"error":     sessionErrorPayload("UnknownError", result.Error.Error()),
		})
	}
}

// swarmMaxIterations returns the MaxIterations cap for the parent processor.
// In /swarm mode (autoApprove=true), the parent is limited to 3 iterations:
// 1) spawn tasks, 2) see results and synthesize, 3) final output.
// This prevents small models from retrying in unbounded loops.
func swarmMaxIterations(autoApprove bool) int {
	if autoApprove {
		return 3
	}
	return 0 // use default (200)
}
