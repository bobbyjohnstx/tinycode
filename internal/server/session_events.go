package server

import (
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

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

	// Flush any batched deltas before finalizing
	batcher := active.deltaBatcher
	active.deltaBatcher = nil

	// Reset streaming state for next potential iteration (tool loop)
	active.streamStarted = false
	active.assistMsgID = ""
	active.textPartID = ""
	active.mu.Unlock()

	if batcher != nil {
		batcher.Flush()
	}

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

	sm.publishAssistantParts(sessionID, bridgeMsgID, bridgePartID, msg.Parts, startTime, completedAt)
}

// publishAssistantParts emits message.part.updated events for each part of an assistant message.
func (sm *SessionManager) publishAssistantParts(sessionID, bridgeMsgID, textPartID string, parts []session.Part, startTime, completedAt int64) {
	for _, part := range parts {
		switch part.Type {
		case session.PartText:
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":        textPartID,
					"sessionID": sessionID,
					"messageID": bridgeMsgID,
					"type":      "text",
					"text":      part.Text,
					"time":      map[string]any{"start": startTime, "end": completedAt},
				},
				"time": completedAt,
			})
		case session.PartReasoning:
			rPartID, _ := id.Ascending("part")
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":        rPartID,
					"sessionID": sessionID,
					"messageID": bridgeMsgID,
					"type":      "reasoning",
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

		batcher := newDeltaBatcher(func(batched string) {
			sm.bus.Publish("message.part.delta", map[string]any{
				"sessionID": sessionID,
				"messageID": msgID,
				"partID":    partID,
				"field":     "text",
				"delta":     batched,
			})
		})
		active.deltaBatcher = batcher
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

		batcher.Add(text)
		return
	}

	batcher := active.deltaBatcher
	active.mu.Unlock()

	if batcher != nil {
		batcher.Add(text)
	}
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