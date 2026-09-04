package server

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

type activeSession struct {
	cancel context.CancelFunc
}

// SessionStatus represents the processing state of a session.
type SessionStatus struct {
	Alert   bool `json:"alert"`
	Working bool `json:"working"`
}

type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*activeSession
	bus      *bus.Bus
	registry *provider.Registry
	db       *sql.DB
	dir      string
}

func NewSessionManager(b *bus.Bus, reg *provider.Registry, db *sql.DB, dir string) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*activeSession),
		bus:      b,
		registry: reg,
		db:       db,
		dir:      dir,
	}
	sm.subscribeCommands()
	sm.subscribePrompts()
	return sm
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

type PromptInput struct {
	SessionID  string
	Model      *promptModel
	Agent      string
	Parts      []promptPart
	MessageID  string
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
	if ok && active.cancel != nil {
		active.cancel()
	}
}

func (sm *SessionManager) StartPrompt(ctx context.Context, input PromptInput) {
	sm.mu.Lock()
	if active, ok := sm.sessions[input.SessionID]; ok && active.cancel != nil {
		active.cancel()
	}
	pctx, cancel := context.WithCancel(ctx)
	sm.sessions[input.SessionID] = &activeSession{cancel: cancel}
	sm.mu.Unlock()

	go sm.processPrompt(pctx, input)
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

	var model *provider.Model
	if input.Model != nil {
		m, err := sm.registry.GetModel(input.Model.ProviderID, input.Model.ModelID)
		if err != nil {
			slog.Error("model not found", "provider", input.Model.ProviderID, "model", input.Model.ModelID, "error", err)
			return
		}
		model = m
	}
	if model == nil {
		slog.Error("no model specified for prompt", "sessionID", sessionID)
		return
	}

	userMsgID, _ := id.Ascending("message")
	now := time.Now().UnixMilli()

	sm.bus.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":        userMsgID,
			"sessionID": sessionID,
			"role":      "user",
			"time":      map[string]any{"created": now},
			"agent":     input.Agent,
			"model": map[string]any{
				"providerID": model.ProviderID,
				"modelID":    model.ID,
			},
		},
	})

	userPartID, _ := id.Ascending("part")
	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":        userPartID,
			"sessionID": sessionID,
			"messageID": userMsgID,
			"type":      "text",
			"text":      userText,
			"time":      map[string]any{"start": now, "end": now},
		},
		"time": now,
	})

	ms := session.NewMessageStore(session.NewStore(sm.db))
	ps := session.NewPartStore(sm.db)

	userMsg := &session.Message{
		ID:        userMsgID,
		SessionID: sessionID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart(userText)},
		CreatedAt: time.UnixMilli(now),
	}
	if err := ms.Append(userMsg); err != nil {
		slog.Error("failed to persist user message", "error", err, "sessionID", sessionID)
	}
	_ = ps.Save(session.StoredPart{
		ID:        userPartID,
		MessageID: userMsgID,
		SessionID: sessionID,
		Type:      string(session.PartText),
		Text:      userText,
		Time:      session.PartTime{Start: now, End: now},
	})

	assistantMsgID, _ := id.Ascending("message")
	sm.bus.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":         assistantMsgID,
			"sessionID":  sessionID,
			"role":       "assistant",
			"time":       map[string]any{"created": now},
			"parentID":   userMsgID,
			"modelID":    model.ID,
			"providerID": model.ProviderID,
			"mode":       "build",
			"agent":      input.Agent,
			"path":       map[string]any{"cwd": sm.dir, "root": sm.dir},
			"cost":       0,
			"tokens":     map[string]any{"input": 0, "output": 0},
		},
	})

	textPartID, _ := id.Ascending("part")
	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":        textPartID,
			"sessionID": sessionID,
			"messageID": assistantMsgID,
			"type":      "text",
			"text":      "",
			"time":      map[string]any{"start": now},
		},
		"time": now,
	})

	client := llm.NewOpenAIClient(model.API.URL+"/v1", "")
	req := llm.Request{
		Model: model.API.ID,
		Messages: []llm.Message{
			{Role: "user", Content: userText},
		},
	}

	ch, err := client.Stream(ctx, req)
	if err != nil {
		slog.Error("llm stream failed", "error", err, "sessionID", sessionID)
		sm.bus.Publish("message.updated", map[string]any{
			"sessionID": sessionID,
			"info": map[string]any{
				"id":         assistantMsgID,
				"sessionID":  sessionID,
				"role":       "assistant",
				"time":       map[string]any{"created": now, "completed": time.Now().UnixMilli()},
				"parentID":   userMsgID,
				"modelID":    model.ID,
				"providerID": model.ProviderID,
				"mode":       "build",
				"agent":      input.Agent,
				"path":       map[string]any{"cwd": sm.dir, "root": sm.dir},
				"cost":       0,
				"tokens":     map[string]any{"input": 0, "output": 0},
				"error":      map[string]any{"_tag": "ApiError", "message": err.Error()},
			},
		})
		return
	}

	var totalText string
	var inputTokens, outputTokens int

	for event := range ch {
		select {
		case <-ctx.Done():
			return
		default:
		}

		switch event.Type {
		case llm.EventTextDelta:
			totalText += event.Text
			sm.bus.Publish("message.part.delta", map[string]any{
				"sessionID": sessionID,
				"messageID": assistantMsgID,
				"partID":    textPartID,
				"field":     "text",
				"delta":     event.Text,
			})

		case llm.EventFinish:
			if event.Usage != nil {
				inputTokens = event.Usage.PromptTokens
				outputTokens = event.Usage.CompletionTokens
			}

		case llm.EventError:
			slog.Error("llm stream error", "error", event.Error, "sessionID", sessionID)
		}
	}

	completedAt := time.Now().UnixMilli()

	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":        textPartID,
			"sessionID": sessionID,
			"messageID": assistantMsgID,
			"type":      "text",
			"text":      totalText,
			"time":      map[string]any{"start": now, "end": completedAt},
		},
		"time": completedAt,
	})

	sm.bus.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":         assistantMsgID,
			"sessionID":  sessionID,
			"role":       "assistant",
			"time":       map[string]any{"created": now, "completed": completedAt},
			"parentID":   userMsgID,
			"modelID":    model.ID,
			"providerID": model.ProviderID,
			"mode":       "build",
			"agent":      input.Agent,
			"path":       map[string]any{"cwd": sm.dir, "root": sm.dir},
			"cost":       0,
			"tokens":     map[string]any{"input": inputTokens, "output": outputTokens},
		},
	})

	assistantMsg := &session.Message{
		ID:        assistantMsgID,
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts:     []session.Part{session.TextPart(totalText)},
		Model:     model.ID,
		Tokens: &session.MsgUsage{
			Input:  inputTokens,
			Output: outputTokens,
		},
		CreatedAt: time.UnixMilli(now),
	}
	if err := ms.Append(assistantMsg); err != nil {
		slog.Error("failed to persist assistant message", "error", err, "sessionID", sessionID)
	}
	_ = ps.Save(session.StoredPart{
		ID:        textPartID,
		MessageID: assistantMsgID,
		SessionID: sessionID,
		Type:      string(session.PartText),
		Text:      totalText,
		Time:      session.PartTime{Start: now, End: completedAt},
	})
}
