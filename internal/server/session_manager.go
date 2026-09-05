package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

type activeSession struct {
	cancel    context.CancelFunc
	processor *session.Processor
	model     *provider.Model
	agent     string

	mu            sync.Mutex
	assistMsgID   string            // bridge-generated assistant message ID during streaming
	textPartID    string            // bridge-generated text part ID during streaming
	streamStarted bool             // whether initial assistant events have been emitted
	msgStartTime  int64            // timestamp when the current assistant message started
	idMap         map[string]string // processor msg ID → bridge msg ID
}

// SessionStatus represents the processing state of a session.
type SessionStatus struct {
	Alert   bool `json:"alert"`
	Working bool `json:"working"`
}

type SessionManager struct {
	mu            sync.Mutex
	sessions      map[string]*activeSession
	bus           *bus.Bus
	registry      *provider.Registry
	db            *sql.DB
	dir           string
	tools         *tool.Registry
	perms         *permission.Service
	agentRegistry *agent.Registry
	mcpSvc        *mcp.Service
	cfg           *config.Info
	revertState   *RevertState
	clientFactory func(*provider.Model) llm.Client
}

func NewSessionManager(b *bus.Bus, reg *provider.Registry, db *sql.DB, dir string, tools *tool.Registry, perms *permission.Service, agents *agent.Registry, mcpSvc *mcp.Service, cfg *config.Info) *SessionManager {
	sm := &SessionManager{
		sessions:      make(map[string]*activeSession),
		bus:           b,
		registry:      reg,
		db:            db,
		dir:           dir,
		tools:         tools,
		perms:         perms,
		agentRegistry: agents,
		mcpSvc:        mcpSvc,
		cfg:           cfg,
		revertState:   NewRevertState(),
		clientFactory: func(m *provider.Model) llm.Client {
			apiKey := ""
			if m.Options != nil {
				if key, ok := m.Options["api_key"].(string); ok {
					apiKey = key
				}
			}
			return llm.NewOpenAIClient(m.API.URL+"/v1", apiKey)
		},
	}
	sm.subscribeCommands()
	sm.subscribePrompts()
	sm.subscribePermissionReplies()
	sm.subscribeProcessorEvents()
	sm.subscribeRevert()
	sm.subscribeUnrevert()
	sm.subscribeSummarize()
	return sm
}

// SetClientFactory overrides the default LLM client factory.
// This allows tests to inject mock clients without changing the constructor.
func (sm *SessionManager) SetClientFactory(f func(*provider.Model) llm.Client) {
	sm.clientFactory = f
}

// Shutdown cancels all active session processors so they can drain
// before the server exits.
func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for sid, active := range sm.sessions {
		if active.cancel != nil {
			active.cancel()
		}
		delete(sm.sessions, sid)
	}
}

// Status returns the processing status of all active sessions.
func (sm *SessionManager) Status() map[string]SessionStatus {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	result := make(map[string]SessionStatus, len(sm.sessions))
	for sid := range sm.sessions {
		result[sid] = SessionStatus{Working: true}
	}
	return result
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
			sm.StartPrompt(context.Background(), PromptInput{
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

func (sm *SessionManager) subscribePermissionReplies() {
	if sm.perms == nil {
		return
	}
	sub := sm.bus.Subscribe("permission.reply")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			permID, _ := props["permissionID"].(string)
			action, _ := props["action"].(string)
			if permID == "" || action == "" {
				continue
			}

			var reply permission.Reply
			switch action {
			case "allow":
				reply = permission.ReplyOnce
			case "always":
				reply = permission.ReplyAlways
			case "reject":
				reply = permission.ReplyReject
			default:
				continue
			}

			sm.perms.RespondToAsk(permission.ReplyInput{
				RequestID: permID,
				Reply:     reply,
			})
		}
	}()
}

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

func (sm *SessionManager) Abort(sessionID string) {
	sm.mu.Lock()
	active, ok := sm.sessions[sessionID]
	sm.mu.Unlock()
	if !ok {
		return
	}
	if active.cancel != nil {
		active.cancel()
	}
	if active.processor != nil {
		active.processor.Abort()
	}
}

func (sm *SessionManager) StartPrompt(ctx context.Context, input PromptInput) {
	sm.mu.Lock()
	if active, ok := sm.sessions[input.SessionID]; ok && active.cancel != nil {
		active.cancel()
	}
	pctx, cancel := context.WithCancel(ctx)
	sm.sessions[input.SessionID] = &activeSession{
		cancel: cancel,
		idMap:  make(map[string]string),
	}
	sm.mu.Unlock()

	go sm.processPrompt(pctx, input)
}

// subscribeProcessorEvents subscribes to Processor bus events and re-publishes
// them as UI events that the TUI and web clients expect.
func (sm *SessionManager) subscribeProcessorEvents() {
	msgSub := sm.bus.Subscribe("session.message")
	deltaSub := sm.bus.Subscribe("session.text.delta")
	toolBeginSub := sm.bus.Subscribe("session.tool.begin")
	toolEndSub := sm.bus.Subscribe("session.tool.end")
	warnSub := sm.bus.Subscribe("session.warning")

	go func() {
		for evt := range msgSub.C {
			sm.bridgeMessageEvent(evt)
		}
	}()
	go func() {
		for evt := range deltaSub.C {
			sm.bridgeTextDelta(evt)
		}
	}()
	go func() {
		for evt := range toolBeginSub.C {
			sm.bridgeToolBegin(evt)
		}
	}()
	go func() {
		for evt := range toolEndSub.C {
			sm.bridgeToolEnd(evt)
		}
	}()
	go func() {
		for evt := range warnSub.C {
			sm.bridgeWarning(evt)
		}
	}()
}

func (sm *SessionManager) getActive(sessionID string) *activeSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.sessions[sessionID]
}

func (sm *SessionManager) bridgeMessageEvent(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	msg, ok := props["message"].(session.Message)
	if !ok {
		return
	}
	active := sm.getActive(sessionID)
	if active == nil {
		return
	}

	now := time.Now().UnixMilli()

	switch msg.Role {
	case session.RoleUser:
		sm.bridgeUserMessage(sessionID, active, msg, now)
	case session.RoleAssistant:
		sm.bridgeAssistantMessage(sessionID, active, msg, now)
	case session.RoleTool:
		sm.bridgeToolMessage(sessionID, msg, now)
	}
}

func (sm *SessionManager) bridgeUserMessage(sessionID string, active *activeSession, msg session.Message, now int64) {
	modelInfo := map[string]any{}
	if active.model != nil {
		modelInfo["providerID"] = active.model.ProviderID
		modelInfo["modelID"] = active.model.ID
	}

	sm.bus.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":        msg.ID,
			"sessionID": sessionID,
			"role":      "user",
			"time":      map[string]any{"created": now},
			"agent":     active.agent,
			"model":     modelInfo,
		},
	})

	for _, part := range msg.Parts {
		if part.Type == session.PartText {
			partID, _ := id.Ascending("part")
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":        partID,
					"sessionID": sessionID,
					"messageID": msg.ID,
					"type":      "text",
					"text":      part.Text,
					"time":      map[string]any{"start": now, "end": now},
				},
				"time": now,
			})
		}
	}
}

func (sm *SessionManager) bridgeAssistantMessage(sessionID string, active *activeSession, msg session.Message, now int64) {
	active.mu.Lock()
	bridgeMsgID := active.assistMsgID
	bridgePartID := active.textPartID
	startTime := active.msgStartTime

	// Store processor-to-bridge ID mapping for persistence
	if bridgeMsgID != "" {
		active.idMap[msg.ID] = bridgeMsgID
	}

	// Reset streaming state for next potential iteration (tool loop)
	active.streamStarted = false
	active.assistMsgID = ""
	active.textPartID = ""
	active.mu.Unlock()

	// If no streaming happened (no text deltas received), use processor's ID directly
	if bridgeMsgID == "" {
		bridgeMsgID = msg.ID
		startTime = now
	}
	if bridgePartID == "" {
		bridgePartID, _ = id.Ascending("part")
	}

	completedAt := time.Now().UnixMilli()

	var inputTokens, outputTokens int
	if msg.Tokens != nil {
		inputTokens = msg.Tokens.Input
		outputTokens = msg.Tokens.Output
	}

	modelID := ""
	providerID := ""
	if active.model != nil {
		modelID = active.model.ID
		providerID = active.model.ProviderID
	}

	sm.bus.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":         bridgeMsgID,
			"sessionID":  sessionID,
			"role":       "assistant",
			"time":       map[string]any{"created": startTime, "completed": completedAt},
			"modelID":    modelID,
			"providerID": providerID,
			"mode":       "build",
			"agent":      active.agent,
			"path":       map[string]any{"cwd": sm.dir, "root": sm.dir},
			"cost":       0,
			"tokens":     map[string]any{"input": inputTokens, "output": outputTokens},
		},
	})

	for _, part := range msg.Parts {
		switch part.Type {
		case session.PartText:
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":        bridgePartID,
					"sessionID": sessionID,
					"messageID": bridgeMsgID,
					"type":      "text",
					"text":      part.Text,
					"time":      map[string]any{"start": startTime, "end": completedAt},
				},
				"time": completedAt,
			})
		case session.PartToolCall:
			tcPartID, _ := id.Ascending("part")
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":         tcPartID,
					"sessionID":  sessionID,
					"messageID":  bridgeMsgID,
					"type":       "tool-call",
					"toolCallID": part.ToolCallID,
					"toolName":   part.ToolName,
					"toolArgs":   part.ToolArgs,
					"time":       map[string]any{"start": startTime, "end": completedAt},
				},
				"time": completedAt,
			})
		}
	}
}

func (sm *SessionManager) bridgeToolMessage(sessionID string, msg session.Message, now int64) {
	toolMsgID, _ := id.Ascending("message")
	sm.bus.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":        toolMsgID,
			"sessionID": sessionID,
			"role":      "tool",
			"time":      map[string]any{"created": now},
		},
	})
	for _, part := range msg.Parts {
		if part.Type == session.PartToolResult {
			partID, _ := id.Ascending("part")
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":         partID,
					"sessionID":  sessionID,
					"messageID":  toolMsgID,
					"type":       "tool-result",
					"toolCallID": part.ToolCallID,
					"toolName":   part.ToolName,
					"toolResult": part.ToolResult,
					"toolError":  part.ToolError,
					"time":       map[string]any{"start": now, "end": now},
				},
				"time": now,
			})
		}
	}
}

func (sm *SessionManager) bridgeTextDelta(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	text, _ := props["text"].(string)
	active := sm.getActive(sessionID)
	if active == nil {
		return
	}

	active.mu.Lock()
	if !active.streamStarted {
		active.assistMsgID, _ = id.Ascending("message")
		active.textPartID, _ = id.Ascending("part")
		active.msgStartTime = time.Now().UnixMilli()
		active.streamStarted = true
		msgID := active.assistMsgID
		partID := active.textPartID
		startTime := active.msgStartTime
		active.mu.Unlock()

		modelID := ""
		providerID := ""
		if active.model != nil {
			modelID = active.model.ID
			providerID = active.model.ProviderID
		}

		// Emit initial assistant message
		sm.bus.Publish("message.updated", map[string]any{
			"sessionID": sessionID,
			"info": map[string]any{
				"id":         msgID,
				"sessionID":  sessionID,
				"role":       "assistant",
				"time":       map[string]any{"created": startTime},
				"modelID":    modelID,
				"providerID": providerID,
				"mode":       "build",
				"agent":      active.agent,
				"path":       map[string]any{"cwd": sm.dir, "root": sm.dir},
				"cost":       0,
				"tokens":     map[string]any{"input": 0, "output": 0},
			},
		})

		// Emit initial empty text part
		sm.bus.Publish("message.part.updated", map[string]any{
			"sessionID": sessionID,
			"part": map[string]any{
				"id":        partID,
				"sessionID": sessionID,
				"messageID": msgID,
				"type":      "text",
				"text":      "",
				"time":      map[string]any{"start": startTime},
			},
			"time": startTime,
		})

		// Emit the delta
		sm.bus.Publish("message.part.delta", map[string]any{
			"sessionID": sessionID,
			"messageID": msgID,
			"partID":    partID,
			"field":     "text",
			"delta":     text,
		})
		return
	}

	msgID := active.assistMsgID
	partID := active.textPartID
	active.mu.Unlock()

	sm.bus.Publish("message.part.delta", map[string]any{
		"sessionID": sessionID,
		"messageID": msgID,
		"partID":    partID,
		"field":     "text",
		"delta":     text,
	})
}

func (sm *SessionManager) bridgeToolBegin(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	toolCallID, _ := props["toolCallID"].(string)
	toolName, _ := props["toolName"].(string)
	active := sm.getActive(sessionID)
	if active == nil {
		return
	}

	active.mu.Lock()
	msgID := active.assistMsgID
	active.mu.Unlock()

	if msgID == "" {
		return
	}

	partID, _ := id.Ascending("part")
	now := time.Now().UnixMilli()

	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":         partID,
			"sessionID":  sessionID,
			"messageID":  msgID,
			"type":       "tool-call",
			"toolCallID": toolCallID,
			"toolName":   toolName,
			"time":       map[string]any{"start": now},
		},
		"time": now,
	})
}

func (sm *SessionManager) bridgeToolEnd(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	toolCallID, _ := props["toolCallID"].(string)
	toolName, _ := props["toolName"].(string)
	toolArgs, _ := props["toolArgs"].(string)
	active := sm.getActive(sessionID)
	if active == nil {
		return
	}

	active.mu.Lock()
	msgID := active.assistMsgID
	active.mu.Unlock()

	if msgID == "" {
		return
	}

	partID, _ := id.Ascending("part")
	now := time.Now().UnixMilli()

	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":         partID,
			"sessionID":  sessionID,
			"messageID":  msgID,
			"type":       "tool-call",
			"toolCallID": toolCallID,
			"toolName":   toolName,
			"toolArgs":   toolArgs,
			"time":       map[string]any{"start": now, "end": now},
		},
		"time": now,
	})
}

func (sm *SessionManager) bridgeWarning(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	message, _ := props["message"].(string)

	sm.bus.Publish("toast", map[string]any{
		"sessionID": sessionID,
		"type":      "warning",
		"message":   message,
	})
}

func (sm *SessionManager) processPrompt(ctx context.Context, input PromptInput) {
	sessionID := input.SessionID
	defer func() {
		sm.mu.Lock()
		delete(sm.sessions, sessionID)
		sm.mu.Unlock()

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

	// Sync MCP tools before processing
	if sm.mcpSvc != nil {
		mcpTools := sm.mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			sm.tools.Register(def)
		}
	}

	// Load existing messages for this session
	ms := session.NewMessageStore(session.NewStore(sm.db))
	existingMsgs, _ := ms.List(sessionID)

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
			idMap = active.idMap
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

// subscribeRevert listens for session.revert events and stashes the current
// working tree changes.
func (sm *SessionManager) subscribeRevert() {
	sub := sm.bus.Subscribe("session.revert")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			if sessionID == "" {
				continue
			}

			if err := sm.revertState.Stash(sm.dir, sessionID); err != nil {
				slog.Error("revert failed", "sessionID", sessionID, "error", err)
				sm.bus.Publish("session.error", map[string]any{
					"sessionID": sessionID,
					"error":     "revert failed: " + err.Error(),
				})
				continue
			}
			sm.bus.Publish("session.reverted", map[string]any{"sessionID": sessionID})
		}
	}()
}

// subscribeUnrevert listens for session.unrevert events and pops the stash
// created by the corresponding revert.
func (sm *SessionManager) subscribeUnrevert() {
	sub := sm.bus.Subscribe("session.unrevert")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			if sessionID == "" {
				continue
			}

			if err := sm.revertState.Pop(sm.dir, sessionID); err != nil {
				slog.Error("unrevert failed", "sessionID", sessionID, "error", err)
				sm.bus.Publish("session.error", map[string]any{
					"sessionID": sessionID,
					"error":     "unrevert failed: " + err.Error(),
				})
				continue
			}
			sm.bus.Publish("session.unreverted", map[string]any{"sessionID": sessionID})
		}
	}()
}

// subscribeSummarize listens for session.summarize events and publishes
// status + compacted events. Full LLM-driven compaction is handled by the
// Processor; this subscriber signals that a manual summarize was requested.
func (sm *SessionManager) subscribeSummarize() {
	sub := sm.bus.Subscribe("session.summarize")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			if sessionID == "" {
				continue
			}

			sm.bus.Publish("session.status", map[string]any{
				"sessionID": sessionID,
				"status":    map[string]any{"alert": false, "working": true},
			})

			sm.bus.Publish("session.compacted", map[string]any{
				"sessionID": sessionID,
				"message":   "Manual summarize requested",
			})

			sm.bus.Publish("session.status", map[string]any{
				"sessionID": sessionID,
				"status":    map[string]any{"alert": false, "working": false},
			})
		}
	}()
}

// extractAllowedPerms returns the set of permission names that are explicitly
// allowed in the ruleset. If no explicit allows are found, returns nil (meaning
// all tools should be included).
func extractAllowedPerms(ruleset permission.Ruleset) []string {
	var result []string
	for _, rule := range ruleset {
		if rule.Action == permission.ActionAllow {
			result = append(result, rule.Permission)
		}
	}
	return result
}
