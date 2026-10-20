package acp

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const permissionTimeout = 120 * time.Second
const questionTimeout = 120 * time.Second

type EventRelay struct {
	bus       *bus.Bus
	transport *StdioTransport
	permSvc   *permission.Service

	mu            sync.Mutex
	partTypes     map[string]string // partID → "text" | "reasoning"
	streamedParts map[string]bool   // partIDs that already streamed deltas
}

func NewEventRelay(b *bus.Bus, transport *StdioTransport) *EventRelay {
	return &EventRelay{
		bus:           b,
		transport:     transport,
		partTypes:     make(map[string]string),
		streamedParts: make(map[string]bool),
	}
}

func (r *EventRelay) SetPermissionService(permSvc *permission.Service) {
	r.permSvc = permSvc
}

// Run relays bus events for all sessions until ctx is cancelled.
func (r *EventRelay) Run(ctx context.Context) {
	r.run(ctx, nil)
}

// RunSession relays bus events filtered to a single session (used by tests).
func (r *EventRelay) RunSession(ctx context.Context, sessionID string) {
	r.runSession(ctx, sessionID, nil)
}

// run relays all sessions. If ready is non-nil it is closed once after
// SubscribeAll returns so tests can wait before publishing.
func (r *EventRelay) run(ctx context.Context, ready chan struct{}) {
	sub := r.bus.SubscribeAll()
	defer sub.Unsubscribe()
	if ready != nil {
		close(ready)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub.C:
			if !ok {
				return
			}
			r.relayEvent(evt)
		}
	}
}

// runSession is like RunSession with an optional ready signal after subscribe.
func (r *EventRelay) runSession(ctx context.Context, sessionID string, ready chan struct{}) {
	sub := r.bus.SubscribeAll()
	defer sub.Unsubscribe()
	if ready != nil {
		close(ready)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub.C:
			if !ok {
				return
			}
			if req, ok := evt.Properties.(permission.Request); ok {
				if sessionID != "" && req.SessionID != sessionID {
					continue
				}
				r.relayEvent(evt)
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			evtSessionID, _ := props["sessionID"].(string)
			if sessionID != "" && evtSessionID != "" && evtSessionID != sessionID {
				continue
			}
			r.relayEvent(evt)
		}
	}
}

func (r *EventRelay) relayEvent(evt bus.Event) {
	switch evt.Type {
	case "permission.asked":
		req, ok := evt.Properties.(permission.Request)
		if !ok {
			// Also accept map form if published that way.
			if props, ok := evt.Properties.(map[string]any); ok {
				data, _ := json.Marshal(props)
				_ = json.Unmarshal(data, &req)
			} else {
				return
			}
		}
		safego.Go(func() {
			r.handlePermissionAsked(context.Background(), req)
		})
		return
	case "question.asked":
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			return
		}
		safego.Go(func() {
			r.handleQuestionAsked(context.Background(), props)
		})
		return
	}

	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}

	sessionID, _ := props["sessionID"].(string)
	if sessionID == "" {
		return
	}

	switch evt.Type {
	case "message.part.updated":
		r.relayMessagePart(sessionID, props)
	case "message.part.delta":
		r.relayMessageDelta(sessionID, props)
	case "session.status":
		r.relaySessionStatus(sessionID, props)
	case "session.error":
		r.relaySessionError(sessionID, props)
	case "session.tool.begin":
		r.relayToolBegin(sessionID, props)
	case "session.tool.end":
		r.relayToolEnd(sessionID, props)
	}
}

func (r *EventRelay) relayMessagePart(sessionID string, props map[string]any) {
	if part, ok := props["part"].(map[string]any); ok {
		if partType, _ := part["type"].(string); partType == "tool" {
			r.relayToolResult(sessionID, part)
			return
		}
	}

	partType, text, partID := extractPart(props)
	if partID != "" && (partType == "text" || partType == "reasoning") {
		r.rememberPartType(partID, partType)
	}

	// Empty initial part (stream start) — remember type, skip chunk.
	if text == "" {
		return
	}

	// Avoid double-dump when deltas already streamed for this part.
	if partID != "" && r.wasStreamed(partID) {
		return
	}

	switch partType {
	case "text":
		r.sendSessionUpdate(sessionID, map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{
				"type": "text",
				"text": text,
			},
		})
	case "reasoning":
		r.sendSessionUpdate(sessionID, map[string]any{
			"sessionUpdate": "agent_thought_chunk",
			"content": map[string]any{
				"type": "text",
				"text": text,
			},
		})
	default:
		slog.Debug("unhandled message part type in ACP relay", "type", partType)
	}
}

func (r *EventRelay) relayMessageDelta(sessionID string, props map[string]any) {
	delta, _ := props["delta"].(string)
	if delta == "" {
		return
	}
	partID, _ := props["partID"].(string)
	partType := r.partType(partID)
	if partType == "" {
		// Default to agent message when type not yet registered.
		partType = "text"
	}
	if partID != "" {
		r.markStreamed(partID)
	}

	sessionUpdate := "agent_message_chunk"
	if partType == "reasoning" {
		sessionUpdate = "agent_thought_chunk"
	}
	r.sendSessionUpdate(sessionID, map[string]any{
		"sessionUpdate": sessionUpdate,
		"content": map[string]any{
			"type": "text",
			"text": delta,
		},
	})
}

func (r *EventRelay) rememberPartType(partID, partType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.partTypes[partID] = partType
}

func (r *EventRelay) partType(partID string) string {
	if partID == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.partTypes[partID]
}

func (r *EventRelay) markStreamed(partID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streamedParts[partID] = true
}

func (r *EventRelay) wasStreamed(partID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.streamedParts[partID]
}

func extractPart(props map[string]any) (partType, text, partID string) {
	if part, ok := props["part"].(map[string]any); ok {
		partType, _ = part["type"].(string)
		text, _ = part["text"].(string)
		if text == "" {
			text, _ = part["content"].(string)
		}
		partID, _ = part["id"].(string)
		return partType, text, partID
	}
	// Legacy flat shape (tests / older publishers).
	partType, _ = props["type"].(string)
	text, _ = props["content"].(string)
	if text == "" {
		text, _ = props["text"].(string)
	}
	partID, _ = props["partID"].(string)
	return partType, text, partID
}

func (r *EventRelay) relaySessionStatus(sessionID string, props map[string]any) {
	var status string
	if statusMap, ok := props["status"].(map[string]any); ok {
		status, _ = statusMap["type"].(string)
	}
	r.sendSessionUpdate(sessionID, map[string]any{
		"sessionUpdate": "session_info_update",
		"status":        status,
	})
}

func (r *EventRelay) relaySessionError(sessionID string, props map[string]any) {
	msg := extractSessionErrorMessage(props)
	if msg == "" {
		msg = "session error"
	}
	r.sendSessionUpdate(sessionID, map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"content": map[string]any{
			"type": "text",
			"text": "Error: " + msg,
		},
	})
}

func extractSessionErrorMessage(props map[string]any) string {
	raw := props["error"]
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	if obj, ok := raw.(map[string]any); ok {
		if data, ok := obj["data"].(map[string]any); ok {
			if msg, ok := data["message"].(string); ok && msg != "" {
				return msg
			}
		}
		if msg, ok := obj["message"].(string); ok && msg != "" {
			return msg
		}
		if name, ok := obj["name"].(string); ok && name != "" {
			return name
		}
	}
	return ""
}

func (r *EventRelay) relayToolBegin(sessionID string, props map[string]any) {
	toolName, _ := props["toolName"].(string)
	if toolName == "" {
		toolName, _ = props["tool"].(string)
	}
	toolCallID, _ := props["toolCallID"].(string)
	r.sendSessionUpdate(sessionID, map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    toolCallID,
		"title":         toolName,
		"kind":          mapToolKind(toolName),
		"status":        "pending",
	})
}

func (r *EventRelay) relayToolEnd(sessionID string, props map[string]any) {
	toolCallID, _ := props["toolCallID"].(string)
	// Arg stream finished — tool is now executing, not completed.
	r.sendSessionUpdate(sessionID, map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    toolCallID,
		"status":        "in_progress",
	})
}

func (r *EventRelay) relayToolResult(sessionID string, part map[string]any) {
	toolCallID, _ := part["callID"].(string)
	if toolCallID == "" {
		toolCallID, _ = part["toolCallID"].(string)
	}
	state, _ := part["state"].(map[string]any)
	if state == nil {
		return
	}

	// Tool-call declarations (no output/error) are not execution results.
	_, hasOutput := state["output"]
	_, hasErr := state["error"]
	status, _ := state["status"].(string)
	if !hasOutput && !hasErr && status != "error" {
		if input, ok := state["input"]; ok && toolCallID != "" {
			r.sendSessionUpdate(sessionID, map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    toolCallID,
				"rawInput":      input,
			})
		}
		return
	}

	acpStatus := "completed"
	if status == "error" || hasErr {
		acpStatus = "failed"
	}

	update := map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    toolCallID,
		"status":        acpStatus,
	}
	if hasOutput {
		update["rawOutput"] = state["output"]
		if out, ok := state["output"].(string); ok && out != "" {
			update["content"] = []map[string]any{
				{
					"type": "content",
					"content": map[string]any{
						"type": "text",
						"text": out,
					},
				},
			}
		}
	} else if hasErr {
		update["rawOutput"] = state["error"]
		if errStr, ok := state["error"].(string); ok && errStr != "" {
			update["content"] = []map[string]any{
				{
					"type": "content",
					"content": map[string]any{
						"type": "text",
						"text": errStr,
					},
				},
			}
		}
	}
	r.sendSessionUpdate(sessionID, update)
}

func (r *EventRelay) sendSessionUpdate(sessionID string, update map[string]any) {
	r.transport.SendNotification("session/update", map[string]any{
		"sessionId": sessionID,
		"update":    update,
	})
}

func (r *EventRelay) handlePermissionAsked(ctx context.Context, req permission.Request) {
	if r.transport == nil || r.permSvc == nil {
		return
	}

	toolCallID := ""
	title := req.Permission
	if req.Tool != nil {
		toolCallID = req.Tool.CallID
	}
	if len(req.Patterns) > 0 {
		title = req.Permission + ": " + req.Patterns[0]
	}

	params := map[string]any{
		"sessionId": req.SessionID,
		"toolCall": map[string]any{
			"toolCallId": toolCallID,
			"title":      title,
			"kind":       mapToolKind(req.Permission),
			"status":     "pending",
			"rawInput":   req.Metadata,
		},
		"options": []map[string]any{
			{"optionId": "allow_once", "name": "Allow Once", "kind": "allow_once"},
			{"optionId": "allow_always", "name": "Allow Always", "kind": "allow_always"},
			{"optionId": "reject_once", "name": "Reject", "kind": "reject_once"},
		},
	}

	reqCtx, cancel := context.WithTimeout(ctx, permissionTimeout)
	defer cancel()

	result, err := r.transport.SendRequest(reqCtx, "session/request_permission", params)
	if err != nil {
		slog.Warn("ACP permission request failed; denying", "id", req.ID, "error", err)
		_ = r.permSvc.RespondToAsk(permission.ReplyInput{
			RequestID: req.ID,
			Reply:     permission.ReplyReject,
		})
		return
	}

	reply := parsePermissionOutcome(result)
	if err := r.permSvc.RespondToAsk(permission.ReplyInput{
		RequestID: req.ID,
		Reply:     reply,
	}); err != nil {
		slog.Warn("ACP permission reply failed", "id", req.ID, "error", err)
	}
}

func (r *EventRelay) handleQuestionAsked(ctx context.Context, props map[string]any) {
	questionID, _ := props["id"].(string)
	sessionID, _ := props["sessionID"].(string)
	if questionID == "" || sessionID == "" {
		return
	}

	reject := func() {
		r.bus.Publish("question.rejected", map[string]any{
			"sessionID": sessionID,
			"requestID": questionID,
		})
	}

	if r.transport == nil || !r.transport.FormElicitation() {
		slog.Info("ACP client lacks form elicitation; rejecting question", "id", questionID)
		reject()
		return
	}

	question, options := parseAskedQuestion(props)
	if question == "" {
		reject()
		return
	}

	answerProp := map[string]any{
		"type":        "string",
		"description": "Your answer",
	}
	if len(options) > 0 {
		answerProp["enum"] = options
	}
	params := map[string]any{
		"sessionId": sessionID,
		"mode":      "form",
		"message":   question,
		"requestedSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": answerProp,
			},
			"required": []string{"answer"},
		},
	}

	reqCtx, cancel := context.WithTimeout(ctx, questionTimeout)
	defer cancel()

	result, err := r.transport.SendRequest(reqCtx, "elicitation/create", params)
	if err != nil {
		slog.Warn("ACP elicitation failed; rejecting question", "id", questionID, "error", err)
		reject()
		return
	}

	action, answer := parseElicitationResult(result)
	switch action {
	case "accept":
		r.bus.Publish("question.replied", map[string]any{
			"sessionID": sessionID,
			"requestID": questionID,
			"answer":    answer,
		})
	default:
		reject()
	}
}

func parseElicitationResult(result json.RawMessage) (action, answer string) {
	if len(result) == 0 {
		return "cancel", ""
	}
	var resp struct {
		Action  string         `json:"action"`
		Content map[string]any `json:"content"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "cancel", ""
	}
	action = resp.Action
	if action == "" {
		return "cancel", ""
	}
	if action != "accept" {
		return action, ""
	}
	if resp.Content != nil {
		if a, ok := resp.Content["answer"].(string); ok {
			answer = a
		}
	}
	return action, answer
}

func parseAskedQuestion(props map[string]any) (string, []string) {
	first := firstQuestionItem(props["questions"])
	if first == nil {
		if q, _ := props["question"].(string); q != "" {
			return q, nil
		}
		return "", nil
	}
	question, _ := first["question"].(string)
	return question, parseQuestionOptions(first["options"])
}

func firstQuestionItem(raw any) map[string]any {
	switch v := raw.(type) {
	case []any:
		if len(v) == 0 {
			return nil
		}
		m, _ := v[0].(map[string]any)
		return m
	case []map[string]any:
		if len(v) == 0 {
			return nil
		}
		return v[0]
	default:
		return nil
	}
}

func parseQuestionOptions(raw any) []string {
	switch opts := raw.(type) {
	case []any:
		var options []string
		for _, o := range opts {
			switch v := o.(type) {
			case string:
				if v != "" {
					options = append(options, v)
				}
			case map[string]any:
				if label, ok := v["label"].(string); ok && label != "" {
					options = append(options, label)
				}
			}
		}
		return options
	case []map[string]any:
		var options []string
		for _, o := range opts {
			if label, ok := o["label"].(string); ok && label != "" {
				options = append(options, label)
			}
		}
		return options
	case []string:
		return append([]string(nil), opts...)
	default:
		return nil
	}
}

func parsePermissionOutcome(result json.RawMessage) permission.Reply {
	if len(result) == 0 {
		return permission.ReplyReject
	}
	var resp struct {
		Outcome any `json:"outcome"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return permission.ReplyReject
	}

	switch v := resp.Outcome.(type) {
	case string:
		return mapOutcomeString(v)
	case map[string]any:
		if outcome, _ := v["outcome"].(string); outcome == "cancelled" {
			return permission.ReplyReject
		}
		if outcome, _ := v["outcome"].(string); outcome == "selected" {
			optionID, _ := v["optionId"].(string)
			return mapOutcomeString(optionID)
		}
		if optionID, _ := v["optionId"].(string); optionID != "" {
			return mapOutcomeString(optionID)
		}
	}
	return permission.ReplyReject
}

func mapOutcomeString(s string) permission.Reply {
	switch s {
	case "allow_once", "once", "allow-once", "allow":
		return permission.ReplyOnce
	case "allow_always", "always", "allow-always":
		return permission.ReplyAlways
	default:
		return permission.ReplyReject
	}
}

func mapToolKind(toolName string) string {
	switch toolName {
	case "bash", "shell", "external_directory", "monitor":
		return "execute"
	case "webfetch", "websearch":
		return "fetch"
	case "edit", "write", "apply_patch", "todowrite":
		return "edit"
	case "read":
		return "read"
	case "grep", "glob":
		return "search"
	default:
		return "other"
	}
}
