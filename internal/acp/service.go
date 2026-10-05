package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/server"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

const protocolVersion = 1

// SessionService persists ACP sessions.
type SessionService interface {
	Create(ctx context.Context, input session.CreateInput) (*session.Info, error)
	Get(ctx context.Context, id string) (*session.Info, error)
	List(ctx context.Context, projectID string) ([]*session.Info, error)
	Delete(ctx context.Context, id string) error
	UpdateModel(ctx context.Context, sessionID string, model *session.ModelRef) error
	UpdateAgent(ctx context.Context, sessionID, agent string) error
	ListMessages(ctx context.Context, sessionID string) ([]session.Message, error)
	Fork(ctx context.Context, parentID, title string) (*session.Info, error)
}

// SessionRunner drives agent turns (typically *server.SessionManager).
type SessionRunner interface {
	StartTextPrompt(ctx context.Context, sessionID, text string) error
	Abort(sessionID string)
	IsBusy(sessionID string) bool
}

type Service struct {
	sessions     SessionService
	bus          *bus.Bus
	runner       SessionRunner
	registry     *provider.Registry
	defaultModel string
	defaultCWD   string
	info         AgentInfo
	transport    *StdioTransport

	mu        sync.Mutex
	cancelled map[string]bool
}

type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Config wires ACP service dependencies.
type Config struct {
	Sessions     SessionService
	Bus          *bus.Bus
	Runner       SessionRunner
	Registry     *provider.Registry
	DefaultModel string
	DefaultCWD   string
	AgentInfo    AgentInfo
}

func NewService(sessions SessionService, b *bus.Bus) *Service {
	return NewServiceWithConfig(Config{
		Sessions: sessions,
		Bus:      b,
		AgentInfo: AgentInfo{
			Name:    "tinycode",
			Version: "0.1.0",
		},
	})
}

func NewServiceWithConfig(cfg Config) *Service {
	info := cfg.AgentInfo
	if info.Name == "" {
		info.Name = "tinycode"
	}
	if info.Version == "" {
		info.Version = "0.1.0"
	}
	return &Service{
		sessions:     cfg.Sessions,
		bus:          cfg.Bus,
		runner:       cfg.Runner,
		registry:     cfg.Registry,
		defaultModel: cfg.DefaultModel,
		defaultCWD:   cfg.DefaultCWD,
		info:         info,
		cancelled:    make(map[string]bool),
	}
}

func (s *Service) SetTransport(t *StdioTransport) {
	s.transport = t
}

func (s *Service) HandleRequest(ctx context.Context, method string, params json.RawMessage) (any, *RPCError) {
	switch method {
	case "initialize":
		return s.handleInitialize(ctx, params)
	case "authenticate":
		return s.handleAuthenticate(ctx, params)
	case "newSession", "session/new":
		return s.handleNewSession(ctx, params)
	case "loadSession", "session/load":
		return s.handleLoadSession(ctx, params)
	case "listSessions", "session/list":
		return s.handleListSessions(ctx, params)
	case "resumeSession", "session/resume":
		return s.handleResumeSession(ctx, params)
	case "closeSession", "session/close":
		return s.handleCloseSession(ctx, params)
	case "forkSession", "session/fork":
		return s.handleForkSession(ctx, params)
	case "prompt", "session/prompt":
		return s.handlePrompt(ctx, params)
	case "cancel", "session/cancel":
		return s.handleCancel(ctx, params)
	case "setSessionConfigOption", "session/set_config_option":
		return map[string]any{}, nil
	case "setSessionMode", "session/set_mode":
		return s.handleSetSessionMode(ctx, params)
	case "setSessionModel", "session/set_model":
		return s.handleSetSessionModel(ctx, params)
	default:
		return nil, &RPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("unknown method: %s", method),
		}
	}
}

func (s *Service) handleInitialize(_ context.Context, _ json.RawMessage) (any, *RPCError) {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"agentCapabilities": map[string]any{
			"loadSession": true,
			"promptCapabilities": map[string]any{
				"embeddedContext": false,
				"image":           false,
			},
			"sessionCapabilities": map[string]any{
				"close":  map[string]any{},
				"fork":   map[string]any{},
				"list":   map[string]any{},
				"resume": map[string]any{},
			},
		},
		"authMethods": []any{},
		"agentInfo":   s.info,
	}, nil
}

func (s *Service) handleAuthenticate(_ context.Context, _ json.RawMessage) (any, *RPCError) {
	return map[string]any{}, nil
}

func (s *Service) handleNewSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId,omitempty"`
		CWD       string `json:"cwd,omitempty"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
		}
	}

	dir := req.CWD
	if dir == "" {
		dir = s.defaultCWD
	}
	if dir == "" {
		return nil, &RPCError{Code: InvalidParams, Message: "cwd is required"}
	}

	model := s.resolveDefaultModel()
	projectID := project.IDFromDirectory(dir)

	input := session.CreateInput{
		ProjectID: projectID,
		Directory: dir,
		Title:     "ACP Session",
		Agent:     "build",
		Model:     model,
	}

	info, err := s.sessions.Create(ctx, input)
	if err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	s.bus.Publish("project.updated", project.FromDirectory(dir))
	s.clearCancelled(info.ID)

	return map[string]any{
		"sessionId":     info.ID,
		"configOptions": []any{},
	}, nil
}

func (s *Service) resolveDefaultModel() *session.ModelRef {
	if s.registry != nil {
		return server.ResolveDefaultModelRef(s.registry, s.defaultModel)
	}
	if s.defaultModel == "" {
		return nil
	}
	providerID, modelID := provider.ParseModel(s.defaultModel)
	if providerID == "" || modelID == "" {
		return nil
	}
	return &session.ModelRef{ProviderID: providerID, ModelID: modelID}
}

func (s *Service) handleLoadSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		CWD       string `json:"cwd,omitempty"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	info, err := s.sessions.Get(ctx, req.SessionID)
	if err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	s.clearCancelled(info.ID)
	s.replayHistory(ctx, info.ID)

	return map[string]any{
		"configOptions": []any{},
	}, nil
}

func (s *Service) replayHistory(ctx context.Context, sessionID string) {
	if s.transport == nil {
		return
	}
	msgs, err := s.sessions.ListMessages(ctx, sessionID)
	if err != nil {
		slog.Warn("ACP loadSession: failed to list messages", "sessionID", sessionID, "error", err)
		return
	}
	for _, msg := range msgs {
		if msg.Role != session.RoleUser && msg.Role != session.RoleAssistant {
			continue
		}
		for _, part := range msg.Parts {
			if part.Type != session.PartText && part.Type != session.PartReasoning {
				continue
			}
			sessionUpdate := "agent_message_chunk"
			switch {
			case part.Type == session.PartReasoning:
				sessionUpdate = "agent_thought_chunk"
			case msg.Role == session.RoleUser:
				sessionUpdate = "user_message_chunk"
			}
			s.transport.SendNotification("session/update", map[string]any{
				"sessionId": sessionID,
				"update": map[string]any{
					"sessionUpdate": sessionUpdate,
					"messageId":     msg.ID,
					"content": map[string]any{
						"type": "text",
						"text": part.Text,
					},
				},
			})
		}
	}
}

func (s *Service) handleListSessions(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		CWD string `json:"cwd,omitempty"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &req)
	}
	dir := req.CWD
	if dir == "" {
		dir = s.defaultCWD
	}
	projectID := "acp"
	if dir != "" {
		projectID = project.IDFromDirectory(dir)
	}

	sessions, err := s.sessions.List(ctx, projectID)
	if err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	var sessionList []map[string]any
	for _, sess := range sessions {
		sessionList = append(sessionList, map[string]any{
			"sessionId": sess.ID,
			"cwd":       sess.Directory,
			"title":     sess.Title,
			"updatedAt": sess.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	if sessionList == nil {
		sessionList = []map[string]any{}
	}

	return map[string]any{
		"sessions": sessionList,
	}, nil
}

func (s *Service) handleResumeSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	_, err := s.sessions.Get(ctx, req.SessionID)
	if err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	s.clearCancelled(req.SessionID)
	return map[string]any{
		"configOptions": []any{},
	}, nil
}

func (s *Service) handleCloseSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	if s.runner != nil {
		s.runner.Abort(req.SessionID)
	}
	if err := s.sessions.Delete(ctx, req.SessionID); err != nil {
		slog.Warn("failed to close ACP session", "id", req.SessionID, "error", err)
	}
	s.clearCancelled(req.SessionID)

	return map[string]any{}, nil
}

func (s *Service) handleForkSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	forked, err := s.sessions.Fork(ctx, req.SessionID, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
		}
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	return map[string]any{
		"sessionId": forked.ID,
	}, nil
}

func (s *Service) handlePrompt(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"prompt"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	blocks := req.Prompt
	if len(blocks) == 0 {
		blocks = req.Content
	}

	var text string
	for _, part := range blocks {
		if part.Type == "text" {
			if text != "" {
				text += "\n"
			}
			text += part.Text
		}
	}
	if text == "" {
		return nil, &RPCError{Code: InvalidParams, Message: "empty prompt content"}
	}

	if _, err := s.sessions.Get(ctx, req.SessionID); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	s.clearCancelled(req.SessionID)

	if s.runner == nil {
		// Legacy bus-only path for unit tests without a runner.
		s.bus.Publish("session.prompt", map[string]any{
			"sessionID": req.SessionID,
			"content":   text,
			"source":    "acp",
		})
		return map[string]any{"stopReason": "end_turn"}, nil
	}

	sub := s.bus.Subscribe("session.status")
	defer sub.Unsubscribe()

	if err := s.runner.StartTextPrompt(ctx, req.SessionID, text); err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	// If the turn finished before we observed status events, avoid hanging.
	if !s.runner.IsBusy(req.SessionID) {
		if s.wasCancelled(req.SessionID) {
			return map[string]any{"stopReason": "cancelled"}, nil
		}
		return map[string]any{"stopReason": "end_turn"}, nil
	}

	for {
		select {
		case evt, ok := <-sub.C:
			if !ok {
				return map[string]any{"stopReason": "end_turn"}, nil
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sid, _ := props["sessionID"].(string)
			if sid != req.SessionID {
				continue
			}
			statusType := ""
			if statusMap, ok := props["status"].(map[string]any); ok {
				statusType, _ = statusMap["type"].(string)
			}
			if statusType != "idle" {
				continue
			}
			if s.wasCancelled(req.SessionID) {
				return map[string]any{"stopReason": "cancelled"}, nil
			}
			return map[string]any{"stopReason": "end_turn"}, nil
		case <-ctx.Done():
			s.runner.Abort(req.SessionID)
			return map[string]any{"stopReason": "cancelled"}, nil
		case <-time.After(100 * time.Millisecond):
			if !s.runner.IsBusy(req.SessionID) {
				if s.wasCancelled(req.SessionID) {
					return map[string]any{"stopReason": "cancelled"}, nil
				}
				return map[string]any{"stopReason": "end_turn"}, nil
			}
		}
	}
}

func (s *Service) handleCancel(_ context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	s.markCancelled(req.SessionID)
	if s.runner != nil {
		s.runner.Abort(req.SessionID)
	} else {
		s.bus.Publish("session.abort", map[string]any{
			"sessionID": req.SessionID,
			"source":    "acp",
		})
	}
	return nil, nil
}

func (s *Service) handleSetSessionMode(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		ModeID    string `json:"modeId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}
	if req.ModeID == "" {
		return nil, &RPCError{Code: InvalidParams, Message: "modeId is required"}
	}

	if _, err := s.sessions.Get(ctx, req.SessionID); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	// Session mode maps to the agent name (build/plan/etc).
	if err := s.sessions.UpdateAgent(ctx, req.SessionID, req.ModeID); err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	return map[string]any{}, nil
}

func (s *Service) handleSetSessionModel(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		ModelID   string `json:"modelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}
	if req.ModelID == "" {
		return nil, &RPCError{Code: InvalidParams, Message: "modelId is required"}
	}

	if _, err := s.sessions.Get(ctx, req.SessionID); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	providerID, modelID := provider.ParseModel(req.ModelID)
	if providerID == "" || modelID == "" {
		// Allow bare model IDs paired with a single slash-less form by treating
		// the whole string as modelID when a default provider is configured.
		return nil, &RPCError{Code: InvalidParams, Message: "modelId must be provider/model"}
	}

	if err := s.sessions.UpdateModel(ctx, req.SessionID, &session.ModelRef{
		ProviderID: providerID,
		ModelID:    modelID,
	}); err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	return map[string]any{}, nil
}

func (s *Service) markCancelled(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelled[sessionID] = true
}

func (s *Service) clearCancelled(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancelled, sessionID)
}

func (s *Service) wasCancelled(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled[sessionID]
}
