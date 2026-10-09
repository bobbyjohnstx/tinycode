package tui

import (
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

func TestParsePartView_UnifiedToolPart(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":        "part_1",
			"sessionID": "ses_1",
			"messageID": "msg_1",
			"type":      "tool",
			"callID":    "tc_1",
			"tool":      "bash",
			"state": map[string]any{
				"status":   "completed",
				"input":    map[string]any{"args": `{"command":"ls"}`},
				"output":   "file1.go\nfile2.go",
				"title":    "bash",
				"metadata": map[string]any{"output": "file1.go\nfile2.go"},
				"time":     map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}

	pv := parsePartView(props)

	if pv.Type != "tool" {
		t.Errorf("expected type 'tool', got %q", pv.Type)
	}
	if pv.ToolName != "bash" {
		t.Errorf("expected ToolName 'bash', got %q", pv.ToolName)
	}
	if pv.ToolArgs != `{"command":"ls"}` {
		t.Errorf("expected ToolArgs from state.input.args, got %q", pv.ToolArgs)
	}
	if pv.Text != "file1.go\nfile2.go" {
		t.Errorf("expected Text from state.output, got %q", pv.Text)
	}
	if pv.ToolError {
		t.Error("expected ToolError false for completed status")
	}
	if pv.Time == nil {
		t.Fatal("expected Time from state.time")
	}
	if pv.Time["start"] != float64(1000) {
		t.Errorf("expected time.start 1000, got %v", pv.Time["start"])
	}
}

func TestParsePartView_ToolPartWithErrorState(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":        "part_2",
			"sessionID": "ses_1",
			"messageID": "msg_1",
			"type":      "tool",
			"callID":    "tc_2",
			"tool":      "bash",
			"state": map[string]any{
				"status": "error",
				"input":  map[string]any{},
				"error":  "command not found",
				"time":   map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}

	pv := parsePartView(props)

	if !pv.ToolError {
		t.Error("expected ToolError true for error status")
	}
	if pv.Text != "command not found" {
		t.Errorf("expected Text from state.error, got %q", pv.Text)
	}
}

func TestParsePartView_ToolPartRunningState(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":        "part_3",
			"sessionID": "ses_1",
			"messageID": "msg_1",
			"type":      "tool",
			"callID":    "tc_3",
			"tool":      "read",
			"state": map[string]any{
				"status": "running",
				"input":  map[string]any{},
				"time":   map[string]any{"start": float64(1000)},
			},
		},
	}

	pv := parsePartView(props)

	if pv.Type != "tool" {
		t.Errorf("expected type 'tool', got %q", pv.Type)
	}
	if pv.ToolName != "read" {
		t.Errorf("expected ToolName 'read', got %q", pv.ToolName)
	}
	if pv.Time == nil {
		t.Fatal("expected Time from state.time")
	}
	// Running state should not have end time
	if _, ok := pv.Time["end"]; ok {
		t.Error("running state should not have end time")
	}
}

func TestMapSSEToMsg_SessionStatusBusy(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.status",
		Properties: map[string]any{
			"sessionID": "ses_1",
			"status":    map[string]any{"type": "busy"},
		},
	}

	msg := mapSSEToMsg(evt)
	statusMsg, ok := msg.(SessionStatusMsg)
	if !ok {
		t.Fatalf("expected SessionStatusMsg, got %T", msg)
	}
	if !statusMsg.Status.Working {
		t.Error("expected Working=true for status type=busy")
	}
}

func TestMapSSEToMsg_SessionStatusIdle(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.status",
		Properties: map[string]any{
			"sessionID": "ses_1",
			"status":    map[string]any{"type": "idle"},
		},
	}

	msg := mapSSEToMsg(evt)
	statusMsg, ok := msg.(SessionStatusMsg)
	if !ok {
		t.Fatalf("expected SessionStatusMsg, got %T", msg)
	}
	if statusMsg.Status.Working {
		t.Error("expected Working=false for status type=idle")
	}
}

func TestMapSSEToMsg_MCPStatus(t *testing.T) {
	evt := api.ServerEvent{
		Type: "mcp.status",
		Properties: map[string]any{
			"server": map[string]any{
				"name":      "filesystem",
				"status":    "connected",
				"toolCount": float64(5),
			},
		},
	}

	msg := mapSSEToMsg(evt)
	mcpMsg, ok := msg.(MCPStatusMsg)
	if !ok {
		t.Fatalf("expected MCPStatusMsg, got %T", msg)
	}
	if mcpMsg.Server.Name != "filesystem" {
		t.Errorf("expected name 'filesystem', got %q", mcpMsg.Server.Name)
	}
	if mcpMsg.Server.Status != "connected" {
		t.Errorf("expected status 'connected', got %q", mcpMsg.Server.Status)
	}
	if mcpMsg.Server.ToolCount != 5 {
		t.Errorf("expected toolCount 5, got %d", mcpMsg.Server.ToolCount)
	}
}

func TestMapSSEToMsg_MCPStatusWithError(t *testing.T) {
	evt := api.ServerEvent{
		Type: "mcp.status",
		Properties: map[string]any{
			"server": map[string]any{
				"name":   "broken",
				"status": "error",
				"error":  "connection refused",
			},
		},
	}

	msg := mapSSEToMsg(evt)
	mcpMsg, ok := msg.(MCPStatusMsg)
	if !ok {
		t.Fatalf("expected MCPStatusMsg, got %T", msg)
	}
	if mcpMsg.Server.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %q", mcpMsg.Server.Error)
	}
}

func TestParsePartView_SubagentLabel(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":            "part_sa",
			"sessionID":     "ses_1",
			"messageID":     "msg_1",
			"type":          "tool",
			"tool":          "bash",
			"subagentLabel": "executor-1",
			"state": map[string]any{
				"status": "completed",
				"input":  map[string]any{"args": `{"command":"ls"}`},
				"time":   map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}
	pv := parsePartView(props)
	if pv.SubagentLabel != "executor-1" {
		t.Errorf("expected SubagentLabel 'executor-1', got %q", pv.SubagentLabel)
	}
}

func TestParsePartView_NoSubagentLabel(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":        "part_no_sa",
			"sessionID": "ses_1",
			"messageID": "msg_1",
			"type":      "tool",
			"tool":      "bash",
			"state": map[string]any{
				"status": "completed",
				"input":  map[string]any{},
				"time":   map[string]any{"start": float64(1000), "end": float64(2000)},
			},
		},
	}
	pv := parsePartView(props)
	if pv.SubagentLabel != "" {
		t.Errorf("expected empty SubagentLabel, got %q", pv.SubagentLabel)
	}
}

func TestMapSSEToMsg_SubagentCompleted(t *testing.T) {
	evt := api.ServerEvent{
		Type: "subagent.completed",
		Properties: map[string]any{
			"sessionID":    "ses_parent",
			"label":        "executor-1",
			"agent":        "executor",
			"inputTokens":  float64(800),
			"outputTokens": float64(400),
		},
	}
	msg := mapSSEToMsg(evt)
	scMsg, ok := msg.(SubagentCompletedMsg)
	if !ok {
		t.Fatalf("expected SubagentCompletedMsg, got %T", msg)
	}
	if scMsg.ParentSessionID != "ses_parent" {
		t.Errorf("expected ParentSessionID 'ses_parent', got %q", scMsg.ParentSessionID)
	}
	if scMsg.Label != "executor-1" {
		t.Errorf("expected Label 'executor-1', got %q", scMsg.Label)
	}
	if scMsg.Agent != "executor" {
		t.Errorf("expected Agent 'executor', got %q", scMsg.Agent)
	}
	if scMsg.InputTokens != 800 {
		t.Errorf("expected InputTokens 800, got %d", scMsg.InputTokens)
	}
	if scMsg.OutputTokens != 400 {
		t.Errorf("expected OutputTokens 400, got %d", scMsg.OutputTokens)
	}
}

// --- parseMessageView tests ---

func TestParseMessageView_FullProperties(t *testing.T) {
	props := map[string]any{
		"info": map[string]any{
			"id":        "msg-1",
			"sessionID": "ses-1",
			"role":      "assistant",
			"agent":     "build",
			"model": map[string]any{
				"modelID":    "ornith-1.0-9b",
				"providerID": "lm-studio",
			},
			"createdAt": "2024-01-01T00:00:00Z",
			"tokens": map[string]any{
				"input":  float64(100),
				"output": float64(200),
			},
			"cost": 0.005,
		},
	}
	mv := parseMessageView(props)
	if mv.Info.ID != "msg-1" {
		t.Errorf("expected ID 'msg-1', got %q", mv.Info.ID)
	}
	if mv.Info.SessionID != "ses-1" {
		t.Errorf("expected SessionID 'ses-1', got %q", mv.Info.SessionID)
	}
	if mv.Info.Role != "assistant" {
		t.Errorf("expected Role 'assistant', got %q", mv.Info.Role)
	}
	if mv.Info.Agent != "build" {
		t.Errorf("expected Agent 'build', got %q", mv.Info.Agent)
	}
	if mv.Info.ModelID != "ornith-1.0-9b" {
		t.Errorf("expected ModelID 'ornith-1.0-9b', got %q", mv.Info.ModelID)
	}
	if mv.Info.ProviderID != "lm-studio" {
		t.Errorf("expected ProviderID 'lm-studio', got %q", mv.Info.ProviderID)
	}
	if mv.Info.CreatedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("expected CreatedAt, got %q", mv.Info.CreatedAt)
	}
	if mv.Info.Tokens.Input != 100 {
		t.Errorf("expected input tokens 100, got %d", mv.Info.Tokens.Input)
	}
	if mv.Info.Tokens.Output != 200 {
		t.Errorf("expected output tokens 200, got %d", mv.Info.Tokens.Output)
	}
	if mv.Info.Cost != 0.005 {
		t.Errorf("expected cost 0.005, got %f", mv.Info.Cost)
	}
}

func TestParseMessageView_NilInfo(t *testing.T) {
	props := map[string]any{"info": nil}
	mv := parseMessageView(props)
	if mv.Info.ID != "" {
		t.Errorf("expected empty MessageView for nil info, got ID %q", mv.Info.ID)
	}
}

func TestParseMessageView_MissingInfoKey(t *testing.T) {
	props := map[string]any{}
	mv := parseMessageView(props)
	if mv.Info.ID != "" {
		t.Errorf("expected empty MessageView for missing info, got ID %q", mv.Info.ID)
	}
}

func TestParseMessageView_LegacyModelFields(t *testing.T) {
	props := map[string]any{
		"info": map[string]any{
			"id":         "msg-2",
			"role":       "assistant",
			"modelID":    "legacy-model",
			"providerID": "legacy-provider",
		},
	}
	mv := parseMessageView(props)
	if mv.Info.ModelID != "legacy-model" {
		t.Errorf("expected legacy ModelID, got %q", mv.Info.ModelID)
	}
	if mv.Info.ProviderID != "legacy-provider" {
		t.Errorf("expected legacy ProviderID, got %q", mv.Info.ProviderID)
	}
}

func TestParseMessageView_CreatedAtFromTimeMap(t *testing.T) {
	props := map[string]any{
		"info": map[string]any{
			"id":   "msg-3",
			"role": "user",
			"time": map[string]any{
				"created": float64(1700000000000),
			},
		},
	}
	mv := parseMessageView(props)
	if mv.Info.CreatedAt == "" {
		t.Error("expected CreatedAt to be populated from time.created")
	}
	// Should be an RFC3339-formatted timestamp.
	if !strings.Contains(mv.Info.CreatedAt, "2023") {
		t.Errorf("expected CreatedAt to contain year 2023, got %q", mv.Info.CreatedAt)
	}
}

func TestParseMessageView_ModelFieldTakesPrecedenceOverLegacy(t *testing.T) {
	props := map[string]any{
		"info": map[string]any{
			"id":   "msg-4",
			"role": "assistant",
			"model": map[string]any{
				"modelID":    "nested-model",
				"providerID": "nested-provider",
			},
			"modelID":    "legacy-model",
			"providerID": "legacy-provider",
		},
	}
	mv := parseMessageView(props)
	if mv.Info.ModelID != "nested-model" {
		t.Errorf("expected nested model to take precedence, got %q", mv.Info.ModelID)
	}
	if mv.Info.ProviderID != "nested-provider" {
		t.Errorf("expected nested provider to take precedence, got %q", mv.Info.ProviderID)
	}
}

// --- parseModelInfo tests ---

func TestParseModelInfo_WithCostsAndLimits(t *testing.T) {
	raw := map[string]any{
		"name": "Ornith 1.0",
		"id":   "ornith-1.0-9b",
		"limit": map[string]any{
			"context": float64(131072),
		},
		"cost": map[string]any{
			"input":  0.5,
			"output": 1.5,
		},
	}
	m := parseModelInfo("ornith-1.0-9b-mlx", "lm-studio", raw)
	if m.Name != "Ornith 1.0" {
		t.Errorf("expected Name 'Ornith 1.0', got %q", m.Name)
	}
	if m.ID != "ornith-1.0-9b" {
		t.Errorf("expected ID overridden by map, got %q", m.ID)
	}
	if m.ProviderID != "lm-studio" {
		t.Errorf("expected ProviderID 'lm-studio', got %q", m.ProviderID)
	}
	if m.ContextLimit != 131072 {
		t.Errorf("expected ContextLimit 131072, got %d", m.ContextLimit)
	}
	if m.CostInput != 0.5 {
		t.Errorf("expected CostInput 0.5, got %f", m.CostInput)
	}
	if m.CostOutput != 1.5 {
		t.Errorf("expected CostOutput 1.5, got %f", m.CostOutput)
	}
}

func TestParseModelInfo_MinimalRawData(t *testing.T) {
	m := parseModelInfo("test-model", "test-provider", "not-a-map")
	if m.ID != "test-model" {
		t.Errorf("expected ID from modelID arg, got %q", m.ID)
	}
	if m.Name != "test-model" {
		t.Errorf("expected Name to default to modelID, got %q", m.Name)
	}
	if m.ProviderID != "test-provider" {
		t.Errorf("expected ProviderID from arg, got %q", m.ProviderID)
	}
}

func TestParseModelInfo_EmptyNameFallsBackToModelID(t *testing.T) {
	raw := map[string]any{
		"name": "",
	}
	m := parseModelInfo("fallback-id", "prov", raw)
	if m.Name != "fallback-id" {
		t.Errorf("expected Name to fall back to modelID, got %q", m.Name)
	}
}

func TestParseModelInfo_NoCostOrLimit(t *testing.T) {
	raw := map[string]any{
		"name": "Simple Model",
	}
	m := parseModelInfo("simple", "prov", raw)
	if m.ContextLimit != 0 {
		t.Errorf("expected zero ContextLimit, got %d", m.ContextLimit)
	}
	if m.CostInput != 0 {
		t.Errorf("expected zero CostInput, got %f", m.CostInput)
	}
	if m.CostOutput != 0 {
		t.Errorf("expected zero CostOutput, got %f", m.CostOutput)
	}
}

// --- parseSessionInfo tests ---

func TestParseSessionInfo_FullProperties(t *testing.T) {
	props := map[string]any{
		"info": map[string]any{
			"id":        "ses-1",
			"title":     "Test Session",
			"agent":     "plan",
			"parentID":  "ses-parent",
			"directory": "/home/user",
			"model": map[string]any{
				"id":         "model-1",
				"providerID": "provider-1",
			},
			"time": map[string]any{
				"created": float64(1700000000000),
				"updated": float64(1700001000000),
			},
		},
	}
	si := parseSessionInfo(props)
	if si.ID != "ses-1" {
		t.Errorf("expected ID 'ses-1', got %q", si.ID)
	}
	if si.Title != "Test Session" {
		t.Errorf("expected Title 'Test Session', got %q", si.Title)
	}
	if si.Agent != "plan" {
		t.Errorf("expected Agent 'plan', got %q", si.Agent)
	}
	if si.ParentID != "ses-parent" {
		t.Errorf("expected ParentID 'ses-parent', got %q", si.ParentID)
	}
	if si.Directory != "/home/user" {
		t.Errorf("expected Directory '/home/user', got %q", si.Directory)
	}
	if si.ModelID != "model-1" {
		t.Errorf("expected ModelID 'model-1', got %q", si.ModelID)
	}
	if si.ProviderID != "provider-1" {
		t.Errorf("expected ProviderID 'provider-1', got %q", si.ProviderID)
	}
	if si.CreatedAt != 1700000000000 {
		t.Errorf("expected CreatedAt 1700000000000, got %d", si.CreatedAt)
	}
	if si.UpdatedAt != 1700001000000 {
		t.Errorf("expected UpdatedAt 1700001000000, got %d", si.UpdatedAt)
	}
}

func TestParseSessionInfo_NilInfo(t *testing.T) {
	props := map[string]any{}
	si := parseSessionInfo(props)
	if si.ID != "" {
		t.Errorf("expected empty SessionInfo, got ID %q", si.ID)
	}
}

func TestParseSessionInfo_NoModelOrTime(t *testing.T) {
	props := map[string]any{
		"info": map[string]any{
			"id":    "ses-2",
			"title": "Minimal",
		},
	}
	si := parseSessionInfo(props)
	if si.ID != "ses-2" {
		t.Errorf("expected ID 'ses-2', got %q", si.ID)
	}
	if si.ModelID != "" {
		t.Errorf("expected empty ModelID, got %q", si.ModelID)
	}
	if si.CreatedAt != 0 {
		t.Errorf("expected zero CreatedAt, got %d", si.CreatedAt)
	}
}

// --- extractSessionError tests ---

func TestExtractSessionError_PlainString(t *testing.T) {
	props := map[string]any{"error": "model not found"}
	got := extractSessionError(props)
	if got != "model not found" {
		t.Errorf("expected 'model not found', got %q", got)
	}
}

func TestExtractSessionError_StructuredPayload(t *testing.T) {
	props := map[string]any{
		"error": map[string]any{
			"name": "UnknownError",
			"data": map[string]any{
				"message": "doom loop detected",
			},
		},
	}
	got := extractSessionError(props)
	if got != "doom loop detected" {
		t.Errorf("expected 'doom loop detected', got %q", got)
	}
}

func TestExtractSessionError_FallbackToName(t *testing.T) {
	props := map[string]any{
		"error": map[string]any{
			"name": "TimeoutError",
		},
	}
	got := extractSessionError(props)
	if got != "TimeoutError" {
		t.Errorf("expected 'TimeoutError', got %q", got)
	}
}

func TestExtractSessionError_NilError(t *testing.T) {
	props := map[string]any{}
	got := extractSessionError(props)
	if got != "" {
		t.Errorf("expected empty for nil error, got %q", got)
	}
}

// --- intFromAny tests ---

func TestIntFromAny_Float64(t *testing.T) {
	val, ok := intFromAny(float64(42))
	if !ok || val != 42 {
		t.Errorf("expected (42, true), got (%d, %v)", val, ok)
	}
}

func TestIntFromAny_Int(t *testing.T) {
	val, ok := intFromAny(int(99))
	if !ok || val != 99 {
		t.Errorf("expected (99, true), got (%d, %v)", val, ok)
	}
}

func TestIntFromAny_UnsupportedType(t *testing.T) {
	val, ok := intFromAny("not a number")
	if ok {
		t.Errorf("expected false for string type, got (%d, true)", val)
	}
}

func TestIntFromAny_Nil(t *testing.T) {
	val, ok := intFromAny(nil)
	if ok {
		t.Errorf("expected false for nil, got (%d, true)", val)
	}
}

// --- stringProp tests ---

func TestStringProp_ExtractsValue(t *testing.T) {
	props := map[string]any{"key": "value"}
	if got := stringProp(props, "key"); got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
}

func TestStringProp_MissingKey(t *testing.T) {
	props := map[string]any{}
	if got := stringProp(props, "missing"); got != "" {
		t.Errorf("expected empty for missing key, got %q", got)
	}
}

func TestStringProp_NonStringValue(t *testing.T) {
	props := map[string]any{"num": 42}
	if got := stringProp(props, "num"); got != "" {
		t.Errorf("expected empty for non-string, got %q", got)
	}
}

// --- additional mapSSEToMsg tests ---

func TestMapSSEToMsg_SessionCreated(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.created",
		Properties: map[string]any{
			"sessionID": "ses_new",
			"info": map[string]any{
				"id":    "ses_new",
				"title": "New Session",
			},
		},
	}
	msg := mapSSEToMsg(evt)
	created, ok := msg.(SessionCreatedMsg)
	if !ok {
		t.Fatalf("expected SessionCreatedMsg, got %T", msg)
	}
	if created.SessionID != "ses_new" {
		t.Errorf("expected SessionID 'ses_new', got %q", created.SessionID)
	}
	if created.Info.Title != "New Session" {
		t.Errorf("expected Title 'New Session', got %q", created.Info.Title)
	}
}

func TestMapSSEToMsg_SessionUpdated(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.updated",
		Properties: map[string]any{
			"sessionID": "ses_1",
			"info": map[string]any{
				"id":    "ses_1",
				"title": "Updated Title",
			},
		},
	}
	msg := mapSSEToMsg(evt)
	updated, ok := msg.(SessionUpdatedMsg)
	if !ok {
		t.Fatalf("expected SessionUpdatedMsg, got %T", msg)
	}
	if updated.Info.Title != "Updated Title" {
		t.Errorf("expected Title 'Updated Title', got %q", updated.Info.Title)
	}
}

func TestMapSSEToMsg_SessionDeleted(t *testing.T) {
	evt := api.ServerEvent{
		Type:       "session.deleted",
		Properties: map[string]any{"sessionID": "ses_del"},
	}
	msg := mapSSEToMsg(evt)
	deleted, ok := msg.(SessionDeletedMsg)
	if !ok {
		t.Fatalf("expected SessionDeletedMsg, got %T", msg)
	}
	if deleted.SessionID != "ses_del" {
		t.Errorf("expected SessionID 'ses_del', got %q", deleted.SessionID)
	}
}

func TestMapSSEToMsg_PermissionAsked(t *testing.T) {
	evt := api.ServerEvent{
		Type: "permission.asked",
		Properties: map[string]any{
			"sessionID":  "ses_1",
			"id":         "perm-1",
			"permission": "shell",
			"metadata":   map[string]any{"tool": "bash"},
		},
	}
	msg := mapSSEToMsg(evt)
	perm, ok := msg.(PermissionRequestedMsg)
	if !ok {
		t.Fatalf("expected PermissionRequestedMsg, got %T", msg)
	}
	if perm.Request.ID != "perm-1" {
		t.Errorf("expected ID 'perm-1', got %q", perm.Request.ID)
	}
	if perm.Request.Permission != "shell" {
		t.Errorf("expected Permission 'shell', got %q", perm.Request.Permission)
	}
	if perm.Request.Metadata["tool"] != "bash" {
		t.Errorf("expected tool 'bash', got %v", perm.Request.Metadata["tool"])
	}
}

func TestMapSSEToMsg_MessagePartDelta(t *testing.T) {
	evt := api.ServerEvent{
		Type: "message.part.delta",
		Properties: map[string]any{
			"sessionID": "ses_1",
			"messageID": "msg-1",
			"partID":    "part-1",
			"field":     "text",
			"delta":     "hello ",
		},
	}
	msg := mapSSEToMsg(evt)
	delta, ok := msg.(MessagePartDeltaMsg)
	if !ok {
		t.Fatalf("expected MessagePartDeltaMsg, got %T", msg)
	}
	if delta.MessageID != "msg-1" {
		t.Errorf("expected MessageID 'msg-1', got %q", delta.MessageID)
	}
	if delta.Field != "text" {
		t.Errorf("expected Field 'text', got %q", delta.Field)
	}
	if delta.Delta != "hello " {
		t.Errorf("expected Delta 'hello ', got %q", delta.Delta)
	}
}

func TestMapSSEToMsg_ToastWarning(t *testing.T) {
	evt := api.ServerEvent{
		Type: "toast",
		Properties: map[string]any{
			"sessionID": "ses_1",
			"type":      "warning",
			"message":   "Multiple consecutive tool call failures. Consider switching to a larger model.",
		},
	}
	msg := mapSSEToMsg(evt)
	toast, ok := msg.(ToastMsg)
	if !ok {
		t.Fatalf("expected ToastMsg, got %T", msg)
	}
	if toast.Text != "Multiple consecutive tool call failures. Consider switching to a larger model." {
		t.Errorf("toast text = %q", toast.Text)
	}
	if !toast.IsError {
		t.Error("expected a warning toast to be marked as an error")
	}
}

func TestMapSSEToMsg_ToastEmptyMessage(t *testing.T) {
	evt := api.ServerEvent{
		Type: "toast",
		Properties: map[string]any{
			"type": "warning",
		},
	}
	msg := mapSSEToMsg(evt)
	if _, ok := msg.(SSEEventMsg); !ok {
		t.Fatalf("expected SSEEventMsg for an empty toast, got %T", msg)
	}
}

func TestMapSSEToMsg_SessionCompacted(t *testing.T) {
	evt := api.ServerEvent{
		Type: "session.compacted",
		Properties: map[string]any{
			"compactionNum": float64(3),
		},
	}
	msg := mapSSEToMsg(evt)
	toast, ok := msg.(ToastMsg)
	if !ok {
		t.Fatalf("expected ToastMsg, got %T", msg)
	}
	if toast.Text != "Context compacted (#3)" {
		t.Errorf("expected compaction toast, got %q", toast.Text)
	}
	if toast.IsError {
		t.Error("expected IsError false")
	}
}

func TestMapSSEToMsg_ProviderDiscovered(t *testing.T) {
	for _, evtType := range []string{"provider.discovered", "provider.removed", "provider.reconnected"} {
		evt := api.ServerEvent{Type: evtType, Properties: map[string]any{}}
		msg := mapSSEToMsg(evt)
		if _, ok := msg.(ProvidersRefreshMsg); !ok {
			t.Errorf("expected ProvidersRefreshMsg for %q, got %T", evtType, msg)
		}
	}
}

func TestMapSSEToMsg_SessionReverted(t *testing.T) {
	evt := api.ServerEvent{Type: "session.reverted", Properties: map[string]any{}}
	msg := mapSSEToMsg(evt)
	toast, ok := msg.(ToastMsg)
	if !ok {
		t.Fatalf("expected ToastMsg, got %T", msg)
	}
	if toast.Text != "Changes reverted" {
		t.Errorf("expected 'Changes reverted', got %q", toast.Text)
	}
}

func TestMapSSEToMsg_SessionUnreverted(t *testing.T) {
	evt := api.ServerEvent{Type: "session.unreverted", Properties: map[string]any{}}
	msg := mapSSEToMsg(evt)
	toast, ok := msg.(ToastMsg)
	if !ok {
		t.Fatalf("expected ToastMsg, got %T", msg)
	}
	if toast.Text != "Changes restored" {
		t.Errorf("expected 'Changes restored', got %q", toast.Text)
	}
}

func TestMapSSEToMsg_MessageUpdated(t *testing.T) {
	evt := api.ServerEvent{
		Type: "message.updated",
		Properties: map[string]any{
			"sessionID": "ses_1",
			"info": map[string]any{
				"id":   "msg-1",
				"role": "assistant",
			},
		},
	}
	msg := mapSSEToMsg(evt)
	mu, ok := msg.(MessageUpdatedMsg)
	if !ok {
		t.Fatalf("expected MessageUpdatedMsg, got %T", msg)
	}
	if mu.SessionID != "ses_1" {
		t.Errorf("expected SessionID 'ses_1', got %q", mu.SessionID)
	}
	if mu.Message.Info.ID != "msg-1" {
		t.Errorf("expected message ID 'msg-1', got %q", mu.Message.Info.ID)
	}
}

func TestMapSSEToMsg_UnknownEventType(t *testing.T) {
	evt := api.ServerEvent{Type: "custom.event", Properties: map[string]any{"foo": "bar"}}
	msg := mapSSEToMsg(evt)
	sseEvt, ok := msg.(SSEEventMsg)
	if !ok {
		t.Fatalf("expected SSEEventMsg for unknown type, got %T", msg)
	}
	if sseEvt.Event.Type != "custom.event" {
		t.Errorf("expected event type 'custom.event', got %q", sseEvt.Event.Type)
	}
}

func TestParsePartView_NilPart(t *testing.T) {
	props := map[string]any{}
	pv := parsePartView(props)
	if pv.ID != "" {
		t.Errorf("expected empty PartView, got ID %q", pv.ID)
	}
}

func TestParsePartView_LegacyTextPart(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":       "part-2",
			"type":     "text",
			"text":     "Hello world",
			"toolName": "read",
			"toolArgs": `{"file_path":"/tmp/f.go"}`,
		},
	}
	pv := parsePartView(props)
	if pv.Text != "Hello world" {
		t.Errorf("expected Text 'Hello world', got %q", pv.Text)
	}
	if pv.ToolName != "read" {
		t.Errorf("expected ToolName 'read', got %q", pv.ToolName)
	}
}

func TestParsePartView_LegacyToolResult(t *testing.T) {
	props := map[string]any{
		"part": map[string]any{
			"id":         "part-3",
			"type":       "tool-result",
			"toolResult": "output data",
		},
	}
	pv := parsePartView(props)
	if pv.Text != "output data" {
		t.Errorf("expected Text from toolResult, got %q", pv.Text)
	}
}
