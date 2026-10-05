package acp

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const permissionTimeout = 120 * time.Second

type EventRelay struct {
	bus       *bus.Bus
	transport *StdioTransport
	permSvc   *permission.Service
}

func NewEventRelay(b *bus.Bus, transport *StdioTransport) *EventRelay {
	return &EventRelay{
		bus:       b,
		transport: transport,
	}
}

func (r *EventRelay) SetPermissionService(permSvc *permission.Service) {
	r.permSvc = permSvc
}

// Run relays bus events for all sessions until ctx is cancelled.
func (r *EventRelay) Run(ctx context.Context) {
	sub := r.bus.SubscribeAll()
	defer sub.Unsubscribe()

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

// RunSession relays bus events filtered to a single session (used by tests).
func (r *EventRelay) RunSession(ctx context.Context, sessionID string) {
	sub := r.bus.SubscribeAll()
	defer sub.Unsubscribe()

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
	case "session.status":
		r.relaySessionStatus(sessionID, props)
	case "session.tool.begin":
		r.relayToolBegin(sessionID, props)
	case "session.tool.end":
		r.relayToolEnd(sessionID, props)
	}
}

func (r *EventRelay) relayMessagePart(sessionID string, props map[string]any) {
	partType, text := extractPart(props)
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

func extractPart(props map[string]any) (partType, text string) {
	if part, ok := props["part"].(map[string]any); ok {
		partType, _ = part["type"].(string)
		text, _ = part["text"].(string)
		if text == "" {
			text, _ = part["content"].(string)
		}
		return partType, text
	}
	// Legacy flat shape (tests / older publishers).
	partType, _ = props["type"].(string)
	text, _ = props["content"].(string)
	if text == "" {
		text, _ = props["text"].(string)
	}
	return partType, text
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
	r.sendSessionUpdate(sessionID, map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    toolCallID,
		"status":        "completed",
	})
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
	case "bash", "shell", "external_directory":
		return "execute"
	case "webfetch", "websearch":
		return "fetch"
	case "edit", "write":
		return "edit"
	case "read":
		return "read"
	case "grep", "glob":
		return "search"
	default:
		return "other"
	}
}
