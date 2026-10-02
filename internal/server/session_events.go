package server

import (
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/vcs"
)

// parseSubagentID splits a synthetic subagent session ID (e.g. "ses_1:executor-A")
// into the parent session ID and label. Returns false if the ID is not a subagent.
func parseSubagentID(sessionID string) (parentID, label string, isSubagent bool) {
	idx := strings.LastIndex(sessionID, ":")
	if idx < 0 {
		return "", "", false
	}
	return sessionID[:idx], sessionID[idx+1:], true
}

// resolveSession looks up the active session for a given sessionID. For subagent
// IDs (containing ":"), it falls back to the parent session and lazily creates
// per-subagent streaming state. Returns the session ID to use for published events.
func (sm *SessionManager) resolveSession(sessionID string) (active *activeSession, publishID, subagentLabel string) {
	active = sm.getActive(sessionID)
	if active != nil {
		return active, sessionID, ""
	}
	parentID, label, ok := parseSubagentID(sessionID)
	if !ok {
		return nil, "", ""
	}
	parent := sm.getActive(parentID)
	if parent == nil {
		return nil, "", ""
	}
	sm.mu.Lock()
	sub, exists := sm.subagentStreams[sessionID]
	if !exists {
		sub = &activeSession{
			model: parent.model,
			agent: label,
			dir:   parent.dir,
			idMap: make(map[string]string),
		}
		sm.subagentStreams[sessionID] = sub
	}
	sm.mu.Unlock()
	return sub, parentID, label
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
	active, publishID, _ := sm.resolveSession(sessionID)
	if active == nil {
		return
	}

	now := time.Now().UnixMilli()

	switch msg.Role {
	case session.RoleUser:
		sm.bridgeUserMessage(publishID, active, msg, now)
	case session.RoleAssistant:
		sm.bridgeAssistantMessage(publishID, active, msg, now)
	case session.RoleTool:
		sm.bridgeToolMessage(publishID, msg, now)
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

	var inputTokens, outputTokens, reasoningTokens, cacheRead, cacheWrite int
	if msg.Tokens != nil {
		inputTokens = msg.Tokens.Input
		outputTokens = msg.Tokens.Output
		reasoningTokens = msg.Tokens.Reasoning
		cacheRead = msg.Tokens.Cache.Read
		cacheWrite = msg.Tokens.Cache.Write
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
			"tokens": map[string]any{
					"input":     inputTokens,
					"output":    outputTokens,
					"reasoning": reasoningTokens,
					"cache":     map[string]any{"read": cacheRead, "write": cacheWrite},
				},
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
			tcInput := map[string]any{}
			if part.ToolArgs != "" {
				tcInput["args"] = part.ToolArgs
			}
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":        tcPartID,
					"sessionID": sessionID,
					"messageID": bridgeMsgID,
					"type":      "tool",
					"callID":    part.ToolCallID,
					"tool":      part.ToolName,
					"state": map[string]any{
						"status":   "completed",
						"input":    tcInput,
						"title":    part.ToolName,
						"metadata": map[string]any{},
						"time":     map[string]any{"start": startTime, "end": completedAt},
					},
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
			status := "completed"
			if part.ToolError {
				status = "error"
			}
			state := map[string]any{
				"status":   status,
				"input":    map[string]any{},
				"output":   part.ToolResult,
				"title":    part.ToolName,
				"metadata": map[string]any{"output": part.ToolResult},
				"time":     map[string]any{"start": now, "end": now},
			}
			if part.ToolError {
				state["error"] = part.ToolResult
			}
			sm.bus.Publish("message.part.updated", map[string]any{
				"sessionID": sessionID,
				"part": map[string]any{
					"id":        partID,
					"sessionID": sessionID,
					"messageID": toolMsgID,
					"type":      "tool",
					"callID":    part.ToolCallID,
					"tool":      part.ToolName,
					"state":     state,
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
	active, publishID, _ := sm.resolveSession(sessionID)
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
				"sessionID": publishID,
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
			"sessionID": publishID,
			"info": map[string]any{
				"id":         msgID,
				"sessionID":  publishID,
				"role":       "assistant",
				"time":       map[string]any{"created": startTime},
				"modelID":    modelID,
				"providerID": providerID,
				"mode":       "build",
				"agent":      active.agent,
				"path":       map[string]any{"cwd": sm.dir, "root": sm.dir},
				"cost":       0,
				"tokens": map[string]any{
					"input":     0,
					"output":    0,
					"reasoning": 0,
					"cache":     map[string]any{"read": 0, "write": 0},
				},
			},
		})

		// Emit initial empty text part
		sm.bus.Publish("message.part.updated", map[string]any{
			"sessionID": publishID,
			"part": map[string]any{
				"id":        partID,
				"sessionID": publishID,
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
	// Skip subagent tool events — publishAssistantParts handles them
	// with complete args when the full message arrives, avoiding
	// duplicate "read done" lines without filenames.
	if _, _, isSub := parseSubagentID(sessionID); isSub {
		return
	}
	toolCallID, _ := props["toolCallID"].(string)
	toolName, _ := props["toolName"].(string)
	active, publishID, label := sm.resolveSession(sessionID)
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

	// Store the part ID so bridgeToolEnd can update the same part
	active.mu.Lock()
	if active.toolPartIDs == nil {
		active.toolPartIDs = make(map[string]string)
	}
	active.toolPartIDs[toolCallID] = partID
	active.mu.Unlock()

	part := map[string]any{
		"id":        partID,
		"sessionID": publishID,
		"messageID": msgID,
		"type":      "tool",
		"callID":    toolCallID,
		"tool":      toolName,
		"state": map[string]any{
			"status": "running",
			"input":  map[string]any{},
			"time":   map[string]any{"start": now},
		},
	}
	if label != "" {
		part["subagentLabel"] = label
	}

	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": publishID,
		"part":      part,
		"time":      now,
	})
}

func (sm *SessionManager) bridgeToolEnd(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	if _, _, isSub := parseSubagentID(sessionID); isSub {
		return
	}
	toolCallID, _ := props["toolCallID"].(string)
	toolName, _ := props["toolName"].(string)
	toolArgs, _ := props["toolArgs"].(string)
	active, publishID, label := sm.resolveSession(sessionID)
	if active == nil {
		return
	}

	active.mu.Lock()
	msgID := active.assistMsgID
	active.mu.Unlock()

	if msgID == "" {
		return
	}

	// Reuse the part ID from bridgeToolBegin so the TUI updates
	// the same part instead of creating a duplicate.
	active.mu.Lock()
	partID := active.toolPartIDs[toolCallID]
	delete(active.toolPartIDs, toolCallID)
	active.mu.Unlock()
	if partID == "" {
		partID, _ = id.Ascending("part")
	}

	now := time.Now().UnixMilli()

	input := map[string]any{}
	if toolArgs != "" {
		input["args"] = toolArgs
	}

	part := map[string]any{
		"id":        partID,
		"sessionID": publishID,
		"messageID": msgID,
		"type":      "tool",
		"callID":    toolCallID,
		"tool":      toolName,
		"state": map[string]any{
			"status":   "completed",
			"input":    input,
			"title":    toolName,
			"metadata": map[string]any{},
			"time":     map[string]any{"start": now, "end": now},
		},
	}
	if label != "" {
		part["subagentLabel"] = label
	}

	sm.bus.Publish("message.part.updated", map[string]any{
		"sessionID": publishID,
		"part":      part,
		"time":      now,
	})

	// Publish session.diff after file-modifying tool completions.
	if isFileModifyingTool(toolName) {
		go sm.publishSessionDiff(publishID)
	}
}

// isFileModifyingTool returns true for tools that modify files on disk.
func isFileModifyingTool(name string) bool {
	switch name {
	case "edit", "write", "apply_patch":
		return true
	}
	return false
}

// publishSessionDiff runs git diff for the session's directory and publishes
// a session.diff event with the results.
func (sm *SessionManager) publishSessionDiff(sessionID string) {
	dir := sm.sessionDir(sessionID)
	diff, err := vcs.GitDiff(dir)
	if err != nil {
		return
	}

	files, additions, deletions := parseDiffStats(diff)
	sm.bus.Publish("session.diff", map[string]any{
		"sessionID": sessionID,
		"diff":      files,
		"summary": map[string]any{
			"additions": additions,
			"deletions": deletions,
			"files":     len(files),
		},
	})
}