package acp

import (
	"context"
	"log/slog"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

type EventRelay struct {
	bus       *bus.Bus
	transport *StdioTransport
}

func NewEventRelay(b *bus.Bus, transport *StdioTransport) *EventRelay {
	return &EventRelay{
		bus:       b,
		transport: transport,
	}
}

func (r *EventRelay) Run(ctx context.Context, sessionID string) {
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
			r.relayEvent(sessionID, evt)
		}
	}
}

func (r *EventRelay) relayEvent(sessionID string, evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}

	evtSessionID, _ := props["sessionID"].(string)
	if evtSessionID != "" && evtSessionID != sessionID {
		return
	}

	switch {
	case evt.Type == "message.part.updated":
		r.relayMessagePart(sessionID, props)
	case evt.Type == "session.status":
		r.relaySessionStatus(sessionID, props)
	case strings.HasPrefix(evt.Type, "tool."):
		r.relayToolEvent(sessionID, evt.Type, props)
	case evt.Type == "permission.updated":
		r.relayPermission(sessionID, props)
	}
}

func (r *EventRelay) relayMessagePart(sessionID string, props map[string]any) {
	partType, _ := props["type"].(string)
	content, _ := props["content"].(string)

	switch partType {
	case "text":
		r.transport.SendNotification("sessionUpdate", map[string]any{
			"sessionId": sessionID,
			"type":      "agent_message_chunk",
			"chunk":     content,
		})
	case "reasoning":
		r.transport.SendNotification("sessionUpdate", map[string]any{
			"sessionId": sessionID,
			"type":      "agent_thought_chunk",
			"chunk":     content,
		})
	default:
		slog.Debug("unhandled message part type in ACP relay", "type", partType)
	}
}

func (r *EventRelay) relaySessionStatus(sessionID string, props map[string]any) {
	var status string
	if statusMap, ok := props["status"].(map[string]any); ok {
		status, _ = statusMap["type"].(string)
	}
	r.transport.SendNotification("sessionUpdate", map[string]any{
		"sessionId": sessionID,
		"type":      "status",
		"status":    status,
	})
}

func (r *EventRelay) relayToolEvent(sessionID string, eventType string, props map[string]any) {
	toolName, _ := props["tool"].(string)
	toolCallID, _ := props["toolCallID"].(string)

	switch eventType {
	case "tool.running":
		r.transport.SendNotification("sessionUpdate", map[string]any{
			"sessionId":  sessionID,
			"type":       "tool_call",
			"toolCallId": toolCallID,
			"name":       toolName,
			"kind":       mapToolKind(toolName),
		})
	case "tool.completed", "tool.error":
		r.transport.SendNotification("sessionUpdate", map[string]any{
			"sessionId":  sessionID,
			"type":       "tool_call_update",
			"toolCallId": toolCallID,
			"status":     eventType,
		})
	}
}

func (r *EventRelay) relayPermission(sessionID string, props map[string]any) {
	r.transport.SendNotification("requestPermission", map[string]any{
		"sessionId": sessionID,
		"props":     props,
	})
}

func mapToolKind(toolName string) string {
	switch {
	case toolName == "bash" || toolName == "shell":
		return "execute"
	case toolName == "webfetch" || toolName == "websearch":
		return "fetch"
	case toolName == "edit" || toolName == "write":
		return "edit"
	case toolName == "read":
		return "read"
	case toolName == "grep" || toolName == "glob":
		return "search"
	default:
		return "other"
	}
}
