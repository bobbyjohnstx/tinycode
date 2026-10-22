package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/server"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

const protocolVersion = 1

// discoveryWait is how long session/new polls for a model after async provider discovery.
// Tests may shorten this.
var discoveryWait = 3 * time.Second

const noModelError = "no model available; run `tinycode doctor` or pass `--model provider/model`"

const configOptionModel = "model"

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
	agents       *agent.Registry
	defaultModel string
	defaultCWD   string
	defaultTitle string
	info         AgentInfo
	transport    *StdioTransport

	mu        sync.Mutex
	cancelled map[string]bool
	failed    map[string]bool
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
	Agents       *agent.Registry
	DefaultModel string
	DefaultCWD   string
	DefaultTitle string
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
		agents:       cfg.Agents,
		defaultModel: cfg.DefaultModel,
		defaultCWD:   cfg.DefaultCWD,
		defaultTitle: cfg.DefaultTitle,
		info:         info,
		cancelled:    make(map[string]bool),
		failed:       make(map[string]bool),
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
		return s.handleSetSessionConfigOption(ctx, params)
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

func (s *Service) handleInitialize(_ context.Context, params json.RawMessage) (any, *RPCError) {
	s.recordClientCapabilities(params)
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

// recordClientCapabilities stores elicitation support advertised by the client.
// Form mode is supported only when clientCapabilities.elicitation.form is present and non-null.
func (s *Service) recordClientCapabilities(params json.RawMessage) {
	form := false
	if len(params) > 0 {
		var req struct {
			ClientCapabilities *struct {
				Elicitation json.RawMessage `json:"elicitation"`
			} `json:"clientCapabilities"`
		}
		if err := json.Unmarshal(params, &req); err == nil && req.ClientCapabilities != nil {
			form = elicitationFormSupported(req.ClientCapabilities.Elicitation)
		}
	}
	if s.transport != nil {
		s.transport.SetFormElicitation(form)
	}
}

func elicitationFormSupported(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var elicitation map[string]any
	if err := json.Unmarshal(raw, &elicitation); err != nil {
		return false
	}
	form, ok := elicitation["form"]
	if !ok || form == nil {
		return false
	}
	return true
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

	model := s.resolveModelOrWait(ctx)
	if model == nil {
		return nil, &RPCError{Code: InvalidParams, Message: noModelError}
	}

	projectID := project.IDFromDirectory(dir)
	title := s.defaultTitle
	if title == "" {
		title = "ACP Session"
	}

	input := session.CreateInput{
		ProjectID: projectID,
		Directory: dir,
		Title:     title,
		Agent:     "build",
		Model:     model,
	}

	info, err := s.sessions.Create(ctx, input)
	if err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	s.bus.Publish("project.updated", project.FromDirectory(dir))
	s.clearCancelled(info.ID)
	s.clearFailed(info.ID)

	return s.sessionSetupResponse(info), nil
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

// resolveModelOrWait resolves a model immediately, then briefly polls while
// async discovery (e.g. OpenRouter) may still be registering models.
func (s *Service) resolveModelOrWait(ctx context.Context) *session.ModelRef {
	if model := s.resolveDefaultModel(); model != nil {
		return model
	}
	if s.registry == nil {
		return nil
	}

	deadline := time.Now().Add(discoveryWait)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return s.resolveDefaultModel()
		case <-ticker.C:
			if model := s.resolveDefaultModel(); model != nil {
				return model
			}
			if time.Now().After(deadline) {
				return nil
			}
		}
	}
}

func (s *Service) sessionSetupResponse(info *session.Info) map[string]any {
	resp := map[string]any{
		"sessionId":     info.ID,
		"configOptions": s.buildConfigOptions(info),
	}
	if modes := s.buildModes(info); modes != nil {
		resp["modes"] = modes
	}
	return resp
}

func (s *Service) buildModes(info *session.Info) map[string]any {
	current := "build"
	if info != nil && info.Agent != "" {
		current = info.Agent
	}

	var available []map[string]any
	if s.agents != nil {
		for _, a := range s.agents.List(current) {
			if a.Mode != agent.ModePrimary && a.Mode != agent.ModeAll {
				continue
			}
			available = append(available, map[string]any{
				"id":          a.Name,
				"name":        a.Name,
				"description": a.Description,
			})
		}
	}
	if len(available) == 0 {
		available = []map[string]any{
			{"id": "build", "name": "build", "description": "Default implementation agent"},
			{"id": "plan", "name": "plan", "description": "Planning agent"},
		}
	}

	return map[string]any{
		"currentModeId":  current,
		"availableModes": available,
	}
}

func (s *Service) buildConfigOptions(info *session.Info) []map[string]any {
	current := ""
	if info != nil && info.Model != nil {
		current = info.Model.ProviderID + "/" + info.Model.ModelID
	}
	if current == "" {
		current = s.defaultModel
	}

	options := s.modelConfigSelectOptions()
	if current == "" && len(options) > 0 {
		current, _ = options[0]["value"].(string)
	}
	if current == "" {
		return []map[string]any{}
	}
	if len(options) == 0 {
		options = []map[string]any{
			{"value": current, "name": current},
		}
	}

	return []map[string]any{
		{
			"type":         "select",
			"id":           configOptionModel,
			"name":         "Model",
			"category":     "model",
			"currentValue": current,
			"options":      options,
		},
	}
}

func (s *Service) modelConfigSelectOptions() []map[string]any {
	if s.registry == nil {
		return nil
	}
	models := s.registry.ListModels()
	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderID != models[j].ProviderID {
			return models[i].ProviderID < models[j].ProviderID
		}
		return models[i].ID < models[j].ID
	})
	var options []map[string]any
	for _, m := range models {
		if m == nil || strings.Contains(strings.ToLower(m.ID), "embed") {
			continue
		}
		value := m.ProviderID + "/" + m.ID
		name := m.Name
		if name == "" {
			name = value
		}
		options = append(options, map[string]any{
			"value": value,
			"name":  name,
		})
	}
	return options
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
	s.clearFailed(info.ID)
	s.replayHistory(ctx, info.ID)

	resp := s.sessionSetupResponse(info)
	delete(resp, "sessionId")
	return resp, nil
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
		switch msg.Role {
		case session.RoleUser, session.RoleAssistant:
			s.replayTextParts(sessionID, msg)
		case session.RoleTool:
			s.replayToolParts(sessionID, msg)
		}
	}
}

func (s *Service) replayTextParts(sessionID string, msg session.Message) {
	for _, part := range msg.Parts {
		switch part.Type {
		case session.PartText, session.PartReasoning:
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
		case session.PartToolCall:
			update := map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    part.ToolCallID,
				"title":         part.ToolName,
				"kind":          mapToolKind(part.ToolName),
				"status":        "completed",
			}
			var raw any
			if part.ToolArgs != "" {
				if err := json.Unmarshal([]byte(part.ToolArgs), &raw); err == nil {
					update["rawInput"] = raw
				} else {
					update["rawInput"] = part.ToolArgs
				}
			}
			enrichEditToolFields(update, part.ToolName, raw)
			s.transport.SendNotification("session/update", map[string]any{
				"sessionId": sessionID,
				"update":    update,
			})
		}
	}
}

func (s *Service) replayToolParts(sessionID string, msg session.Message) {
	for _, part := range msg.Parts {
		if part.Type != session.PartToolResult {
			continue
		}
		status := "completed"
		if part.ToolError {
			status = "failed"
		}
		update := map[string]any{
			"sessionUpdate": "tool_call_update",
			"toolCallId":    part.ToolCallID,
			"status":        status,
		}
		if part.ToolResult != "" {
			update["rawOutput"] = part.ToolResult
			update["content"] = []map[string]any{
				{
					"type": "content",
					"content": map[string]any{
						"type": "text",
						"text": part.ToolResult,
					},
				},
			}
		}
		s.transport.SendNotification("session/update", map[string]any{
			"sessionId": sessionID,
			"update":    update,
		})
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

	info, err := s.sessions.Get(ctx, req.SessionID)
	if err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	s.clearCancelled(req.SessionID)
	s.clearFailed(req.SessionID)
	resp := s.sessionSetupResponse(info)
	delete(resp, "sessionId")
	return resp, nil
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
	s.clearFailed(req.SessionID)

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
	s.clearFailed(req.SessionID)

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
	errSub := s.bus.Subscribe("session.error")
	defer errSub.Unsubscribe()

	if err := s.runner.StartTextPrompt(ctx, req.SessionID, text); err != nil {
		return nil, &RPCError{Code: InternalError, Message: err.Error()}
	}

	// If the turn finished before we observed status events, avoid hanging.
	if !s.runner.IsBusy(req.SessionID) {
		s.drainSessionErrors(errSub, req.SessionID)
		return map[string]any{"stopReason": s.promptStopReason(req.SessionID)}, nil
	}

	for {
		select {
		case evt, ok := <-errSub.C:
			if ok {
				s.noteSessionError(evt, req.SessionID)
			}
		case evt, ok := <-sub.C:
			if !ok {
				s.drainSessionErrors(errSub, req.SessionID)
				return map[string]any{"stopReason": s.promptStopReason(req.SessionID)}, nil
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
			s.drainSessionErrors(errSub, req.SessionID)
			return map[string]any{"stopReason": s.promptStopReason(req.SessionID)}, nil
		case <-ctx.Done():
			s.runner.Abort(req.SessionID)
			return map[string]any{"stopReason": "cancelled"}, nil
		case <-time.After(100 * time.Millisecond):
			if !s.runner.IsBusy(req.SessionID) {
				s.drainSessionErrors(errSub, req.SessionID)
				return map[string]any{"stopReason": s.promptStopReason(req.SessionID)}, nil
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
	}
	// Always publish so EventRelay can cancel in-flight permission/elicitation RPCs
	// (Abort alone leaves SendRequest waiting until AskTimeout → orphaned IDE modals).
	s.bus.Publish("session.abort", map[string]any{
		"sessionID": req.SessionID,
		"source":    "acp",
	})
	return nil, nil
}

func (s *Service) handleSetSessionConfigOption(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var req struct {
		SessionID string `json:"sessionId"`
		ConfigID  string `json:"configId"`
		Value     string `json:"value"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "invalid params"}
	}
	if req.SessionID == "" || req.ConfigID == "" {
		return nil, &RPCError{Code: InvalidParams, Message: "sessionId and configId are required"}
	}

	info, err := s.sessions.Get(ctx, req.SessionID)
	if err != nil {
		return nil, &RPCError{Code: InvalidParams, Message: "session not found"}
	}

	switch req.ConfigID {
	case configOptionModel:
		if req.Value == "" {
			return nil, &RPCError{Code: InvalidParams, Message: "value is required"}
		}
		providerID, modelID := provider.ParseModel(req.Value)
		if providerID == "" || modelID == "" {
			return nil, &RPCError{Code: InvalidParams, Message: "model value must be provider/model"}
		}
		if err := s.sessions.UpdateModel(ctx, req.SessionID, &session.ModelRef{
			ProviderID: providerID,
			ModelID:    modelID,
		}); err != nil {
			return nil, &RPCError{Code: InternalError, Message: err.Error()}
		}
		info.Model = &session.ModelRef{ProviderID: providerID, ModelID: modelID}
	default:
		return nil, &RPCError{
			Code:    InvalidParams,
			Message: fmt.Sprintf("unsupported config option %q; supported: %s", req.ConfigID, configOptionModel),
		}
	}

	return map[string]any{
		"configOptions": s.buildConfigOptions(info),
	}, nil
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

func (s *Service) markFailed(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[sessionID] = true
}

func (s *Service) clearFailed(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failed, sessionID)
}

func (s *Service) wasFailed(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed[sessionID]
}

// promptStopReason prefers cancelled, then refusal on session.error, else end_turn.
func (s *Service) promptStopReason(sessionID string) string {
	if s.wasCancelled(sessionID) {
		return "cancelled"
	}
	if s.wasFailed(sessionID) {
		return "refusal"
	}
	return "end_turn"
}

func (s *Service) noteSessionError(evt bus.Event, sessionID string) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sid, _ := props["sessionID"].(string)
	if sid != sessionID {
		return
	}
	s.markFailed(sessionID)
}

func (s *Service) drainSessionErrors(sub *bus.Subscription, sessionID string) {
	for {
		select {
		case evt, ok := <-sub.C:
			if !ok {
				return
			}
			s.noteSessionError(evt, sessionID)
		default:
			return
		}
	}
}
