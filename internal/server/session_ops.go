package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/command"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

// PromptInput describes a user prompt to be processed by a session.
type PromptInput struct {
	SessionID      string
	Model          *promptModel
	Agent          string
	Parts          []promptPart
	MessageID      string
	ThinkingBudget *int
	MaxTokens      *int
	SystemPrefix   string
	MaxIterations  *int
}

type promptModel struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type promptPart struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	Text      string `json:"text,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

func (sm *SessionManager) subscribeCommands() {
	sub := sm.trackSub(sm.bus.Subscribe("session.command"))
	safego.Go(func() {
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
	})
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
	sub := sm.trackSub(sm.bus.Subscribe("session.prompt"))
	safego.Go(func() {
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
					ModelID:    info.Model.ModelID,
				},
				Agent: info.Agent,
				Parts: []promptPart{{Type: "text", Text: content}},
			})
		}
	})
}

// StartTextPrompt loads the session's model/agent from the store and starts a prompt turn.
func (sm *SessionManager) StartTextPrompt(ctx context.Context, sessionID, text string) error {
	store := session.NewStore(sm.db)
	info, err := store.Get(sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if info.Model == nil {
		return fmt.Errorf("session %s has no model configured", sessionID)
	}
	agentName := info.Agent
	if agentName == "" {
		agentName = "build"
	}
	sm.StartPrompt(ctx, PromptInput{
		SessionID: sessionID,
		Model: &promptModel{
			ProviderID: info.Model.ProviderID,
			ModelID:    info.Model.ModelID,
		},
		Agent: agentName,
		Parts: []promptPart{{Type: "text", Text: text}},
	})
	return nil
}

// IsBusy reports whether a prompt turn is currently active for the session.
func (sm *SessionManager) IsBusy(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.sessions[sessionID]
	return ok
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

	safego.Go(func() { sm.processPrompt(pctx, input, done) })
}

func (sm *SessionManager) processPrompt(ctx context.Context, input PromptInput, done chan struct{}) {
	defer close(done)
	sessionID := input.SessionID
	myDone := done // capture this goroutine's done channel
	defer func() {
		sm.mu.Lock()
		if active, ok := sm.sessions[sessionID]; ok && active.done == myDone {
			delete(sm.sessions, sessionID)
		}
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

	// Create a per-prompt snapshot so concurrent sessions don't race on
	// shared tool registry state (ResetTaskRound / ResetBudget mutations).
	var promptTools *tool.Registry
	if sm.tools != nil {
		promptTools = sm.tools.Snapshot()
		promptTools.ResetTaskRound()
		promptTools.ResetBudget(20)
	}

	var userText string
	var imageParts []session.Part
	for _, p := range input.Parts {
		switch p.Type {
		case "text":
			if p.Text != "" {
				userText += p.Text
			} else if p.Content != "" {
				userText += p.Content
			}
		case "image":
			data := p.Content
			if data == "" {
				data = p.Text
			}
			if data != "" {
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "image/png"
				}
				imageParts = append(imageParts, session.ImagePart(data, mediaType))
			}
		}
	}
	if userText == "" && len(imageParts) == 0 {
		return
	}
	if userText == "" {
		userText = "[image attached]"
	}

	var skillPaths []string
	if sm.cfg != nil && sm.cfg.Skills != nil {
		skillPaths = sm.cfg.Skills.Paths
	}
	expandResult := command.ExpandSlashCommand(userText, config.ConfigDir(), sm.dir, skillPaths...)
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

	agentPerms, agentRuleset, systemPrompt := sm.buildPromptSystemPrompt(input, model, sessionDir)

	// Prepend effort-level system prefix if provided.
	if input.SystemPrefix != "" {
		systemPrompt = input.SystemPrefix + "\n\n" + systemPrompt
	}

	if agentRuleset != nil && promptTools != nil {
		promptTools = promptTools.WithAgentRules(agentRuleset)
	}

	if sm.mcpSvc != nil && promptTools != nil {
		mcpTools := sm.mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			promptTools.Register(def)
		}
	}

	ms := session.NewMessageStore(session.NewStore(sm.db))
	existingMsgs, err := ms.List(sessionID)
	if err != nil {
		slog.Warn("failed to load existing messages, starting with empty history", "sessionID", sessionID, "error", err)
		existingMsgs = nil
	}

	// On first prompt, fire session.start hook synchronously and inject
	// any additionalContext into the system prompt.
	if len(existingMsgs) == 0 && sm.sessionStartHook != nil {
		if hookCtx := sm.sessionStartHook(sessionID); len(hookCtx) > 0 {
			systemPrompt += "\n\n[Hook Context]\n" + strings.Join(hookCtx, "\n")
		}
	}

	client := sm.clientFactory(model)
	sessionTools := promptTools
	if expandResult.AutoApprove {
		sessionTools = sessionTools.WithAutoApprove().WithOnlyTools("task")
	}
	displayText := ""
	if userText != llmText {
		displayText = userText
	}
	maxIter := swarmMaxIterations(expandResult.AutoApprove)
	if input.MaxIterations != nil && *input.MaxIterations > 0 && maxIter == 0 {
		maxIter = *input.MaxIterations
	}
	smallModel := ""
	if sm.cfg != nil {
		smallModel = sm.cfg.SmallModel
	}
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:       sessionID,
		Agent:           input.Agent,
		Model:           model,
		SystemPrompt:    systemPrompt,
		Compaction:      sm.buildCompactionConfig(),
		AgentPerms:      agentPerms,
		Ruleset:         agentRuleset,
		Perms:           sm.perms,
		Directory:       sessionDir,
		UserDisplayText: displayText,
		MaxIterations:   maxIter,
		MaxTokens:       input.MaxTokens,
		ThinkingBudget:  input.ThinkingBudget,
		TokenBudget:     sm.tokenBudget,
		SmallModel:      smallModel,
	}, client, sessionTools, sm.bus)
	proc.SetMessages(existingMsgs)
	if len(imageParts) > 0 {
		proc.SetUserExtraParts(imageParts)
	}

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

	// Drain buffered monitor output and inject as a follow-up prompt
	// for the next turn. Published on the bus so the normal prompt
	// subscriber picks it up after this processPrompt returns.
	if promptTools != nil {
		if monitorOutput := promptTools.DrainMonitorOutput(); monitorOutput != "" {
			slog.Info("injecting monitor output at turn boundary", "sessionID", sessionID)
			sm.bus.Publish("session.prompt", map[string]any{
				"sessionID": sessionID,
				"content":   "Background monitor output:\n\n" + monitorOutput,
			})
		}
	}

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
