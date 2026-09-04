package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

const protocolVersion = 1

type SessionService interface {
	Create(ctx context.Context, input session.CreateInput) (*session.Info, error)
	Get(ctx context.Context, id string) (*session.Info, error)
	List(ctx context.Context, projectID string) ([]*session.Info, error)
	Delete(ctx context.Context, id string) error
}

type Service struct {
	mu       sync.RWMutex
	sessions SessionService
	bus      *bus.Bus
	info     AgentInfo
}

type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func NewService(sessions SessionService, b *bus.Bus) *Service {
	return &Service{
		sessions: sessions,
		bus:      b,
		info: AgentInfo{
			Name:    "tinycode",
			Version: "0.1.0",
		},
	}
}

func (s *Service) HandleRequest(ctx context.Context, method string, params json.RawMessage) (any, *RPCError) {
	switch method {
	case "initialize":
		return s.handleInitialize(ctx, params)
	case "authenticate":
		return s.handleAuthenticate(ctx, params)
	case "newSession":
		return s.handleNewSession(ctx, params)
	case "loadSession":
		return s.handleLoadSession(ctx, params)
	case "listSessions":
		return s.handleListSessions(ctx, params)
	case "resumeSession":
		return s.handleResumeSession(ctx, params)
	case "closeSession":
		return s.handleCloseSession(ctx, params)
	case "forkSession":
		return s.handleForkSession(ctx, params)
	case "prompt":
		return s.handlePrompt(ctx, params)
	case "cancel":
		return s.handleCancel(ctx, params)
	case "setSessionConfigOption":
		return map[string]any{}, nil
	case "setSessionMode":
		return s.handleSetSessionMode(ctx, params)
	case "setSessionModel":
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
				"embeddedContext": true,
				"image":          true,
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
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	input := session.CreateInput{
		ProjectID: "acp",
		Directory: req.CWD,
		Title:     "ACP Session",
		Agent:     "build",
	}

	info, err := s.sessions.Create(ctx, input)
	if err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	return map[string]any{
		"sessionId":     info.ID,
		"configOptions": map[string]any{},
	}, nil
}

func (s *Service) handleLoadSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
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

	return map[string]any{
		"configOptions": map[string]any{},
	}, nil
}

func (s *Service) handleListSessions(ctx context.Context, _ json.RawMessage) (any, *RPCError) {
	sessions, err := s.sessions.List(ctx, "acp")
	if err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	var sessionList []map[string]any
	for _, sess := range sessions {
		sessionList = append(sessionList, map[string]any{
			"id":        sess.ID,
			"title":     sess.Title,
			"createdAt": sess.CreatedAt,
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

	return map[string]any{
		"configOptions": map[string]any{},
	}, nil
}

func (s *Service) handleCloseSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	if err := s.sessions.Delete(ctx, req.SessionID); err != nil {
		slog.Warn("failed to close ACP session", "id", req.SessionID, "error", err)
	}

	return map[string]any{}, nil
}

func (s *Service) handleForkSession(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	parent, err := s.sessions.Get(ctx, req.SessionID)
	if err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	input := session.CreateInput{
		ProjectID: parent.ProjectID,
		Directory: parent.Directory,
		Title:     parent.Title + " (fork)",
		Agent:     "build",
		ParentID:  parent.ID,
	}

	forked, err := s.sessions.Create(ctx, input)
	if err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	return map[string]any{
		"sessionId": forked.ID,
	}, nil
}

func (s *Service) handlePrompt(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	var text string
	for _, part := range req.Content {
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

	s.bus.Publish("session.prompt", map[string]any{
		"sessionID": req.SessionID,
		"content":   text,
		"source":    "acp",
	})

	return map[string]any{}, nil
}

func (s *Service) handleCancel(_ context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	s.bus.Publish("session.abort", map[string]any{
		"sessionID": req.SessionID,
		"source":    "acp",
	})

	return nil, nil
}

func (s *Service) handleSetSessionMode(_ context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		ModeID    string `json:"modeId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	s.bus.Publish("session.mode", map[string]any{
		"sessionID": req.SessionID,
		"mode":      req.ModeID,
	})

	return map[string]any{}, nil
}

func (s *Service) handleSetSessionModel(_ context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		ModelID   string `json:"modelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}

	s.bus.Publish("session.model", map[string]any{
		"sessionID": req.SessionID,
		"model":     req.ModelID,
	})

	return map[string]any{}, nil
}
