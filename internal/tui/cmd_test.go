package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

func sessionInfoFixture() session.Info {
	return session.Info{
		ID:       "ses-api",
		Title:    "API Session",
		Agent:    "executor",
		ParentID: "ses-parent",
		Model:    &session.ModelRef{ModelID: "test-model", ProviderID: "test-provider"},
		Time:     session.TimeInfo{Created: 1000, Updated: 2000},
	}
}

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

// --- fetchSessions tests ---

func TestFetchSessions_ReturnsSessionList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/session") {
			http.Error(w, "not found", 404)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "s1", "title": "Session One", "time": map[string]any{"created": 1000, "updated": 2000}},
			{"id": "s2", "title": "Session Two", "time": map[string]any{"created": 3000, "updated": 4000}},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchSessions(client, 50, 0)
	msg := cmd()

	loaded, ok := msg.(SessionListMsg)
	if !ok {
		t.Fatalf("expected SessionListMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(loaded.Sessions))
	}
	if loaded.Sessions[0].ID != "s1" {
		t.Errorf("expected first session ID 's1', got %q", loaded.Sessions[0].ID)
	}
}

func TestFetchSessions_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchSessions(client, 50, 0)
	msg := cmd()

	loaded, ok := msg.(SessionListMsg)
	if !ok {
		t.Fatalf("expected SessionListMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchProviders tests ---

func TestFetchProviders_ReturnsProviderList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"all": []map[string]any{
				{
					"id":   "openrouter",
					"name": "OpenRouter",
					"models": map[string]any{
						"gpt-4": map[string]any{
							"name": "GPT-4",
							"cost": map[string]any{"input": 0.5, "output": 1.0},
						},
					},
				},
			},
			"connected": []string{"openrouter"},
			"default":   map[string]string{"openrouter": "gpt-4"},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchProviders(client)
	msg := cmd()

	loaded, ok := msg.(ProvidersLoadedMsg)
	if !ok {
		t.Fatalf("expected ProvidersLoadedMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(loaded.Providers))
	}
	if loaded.Providers[0].ID != "openrouter" {
		t.Errorf("expected provider ID 'openrouter', got %q", loaded.Providers[0].ID)
	}
	if loaded.DefaultProvider != "openrouter" {
		t.Errorf("expected default provider 'openrouter', got %q", loaded.DefaultProvider)
	}
	if loaded.DefaultModel != "gpt-4" {
		t.Errorf("expected default model 'gpt-4', got %q", loaded.DefaultModel)
	}
}

func TestFetchProviders_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "provider error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchProviders(client)
	msg := cmd()

	loaded, ok := msg.(ProvidersLoadedMsg)
	if !ok {
		t.Fatalf("expected ProvidersLoadedMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchMCPStatus tests ---

func TestFetchMCPStatus_ReturnsServerList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]any{
			"filesystem": {"status": "connected", "toolCount": float64(5)},
			"broken":     {"status": "error", "error": "connection refused"},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchMCPStatus(client)
	msg := cmd()

	loaded, ok := msg.(MCPStatusLoadedMsg)
	if !ok {
		t.Fatalf("expected MCPStatusLoadedMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(loaded.Servers))
	}
	// Find filesystem server
	var found bool
	for _, s := range loaded.Servers {
		if s.Name == "filesystem" {
			found = true
			if s.Status != "connected" {
				t.Errorf("expected status 'connected', got %q", s.Status)
			}
			if s.ToolCount != 5 {
				t.Errorf("expected toolCount 5, got %d", s.ToolCount)
			}
		}
	}
	if !found {
		t.Error("filesystem server not found")
	}
}

func TestFetchMCPStatus_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "mcp error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchMCPStatus(client)
	msg := cmd()

	loaded, ok := msg.(MCPStatusLoadedMsg)
	if !ok {
		t.Fatalf("expected MCPStatusLoadedMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchLSPStatus tests ---

func TestFetchLSPStatus_ReturnsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":  true,
			"errors":   3,
			"warnings": 7,
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchLSPStatus(client)
	msg := cmd()

	loaded, ok := msg.(LSPStatusLoadedMsg)
	if !ok {
		t.Fatalf("expected LSPStatusLoadedMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if loaded.Status.Disabled {
		t.Error("expected Disabled=false for enabled LSP")
	}
	if loaded.Status.Errors != 3 {
		t.Errorf("expected 3 errors, got %d", loaded.Status.Errors)
	}
	if loaded.Status.Warnings != 7 {
		t.Errorf("expected 7 warnings, got %d", loaded.Status.Warnings)
	}
}

func TestFetchLSPStatus_DisabledLSP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":  false,
			"errors":   0,
			"warnings": 0,
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchLSPStatus(client)
	msg := cmd()

	loaded, ok := msg.(LSPStatusLoadedMsg)
	if !ok {
		t.Fatalf("expected LSPStatusLoadedMsg, got %T", msg)
	}
	if !loaded.Status.Disabled {
		t.Error("expected Disabled=true for disabled LSP")
	}
}

func TestFetchLSPStatus_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "lsp error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchLSPStatus(client)
	msg := cmd()

	loaded, ok := msg.(LSPStatusLoadedMsg)
	if !ok {
		t.Fatalf("expected LSPStatusLoadedMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchAllAgents tests ---

func TestFetchAllAgents_ReturnsAgentList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"name": "executor", "mode": "tool", "native": true},
			{"name": "custom", "mode": "tool", "disabled": true},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchAllAgents(client)
	msg := cmd()

	loaded, ok := msg.(AgentListMsg)
	if !ok {
		t.Fatalf("expected AgentListMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(loaded.Agents))
	}
	if loaded.Agents[0].Name != "executor" {
		t.Errorf("expected first agent 'executor', got %q", loaded.Agents[0].Name)
	}
}

func TestFetchAllAgents_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "agent error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchAllAgents(client)
	msg := cmd()

	loaded, ok := msg.(AgentListMsg)
	if !ok {
		t.Fatalf("expected AgentListMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchCommands tests ---

func TestFetchCommands_ReturnsCommandList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"name": "help", "description": "Show help"},
			{"name": "connect", "description": "Configure models"},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchCommands(client)
	msg := cmd()

	loaded, ok := msg.(CommandListMsg)
	if !ok {
		t.Fatalf("expected CommandListMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(loaded.Commands))
	}
	if loaded.Commands[0].Name != "help" {
		t.Errorf("expected first command 'help', got %q", loaded.Commands[0].Name)
	}
}

func TestFetchCommands_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "command error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchCommands(client)
	msg := cmd()

	loaded, ok := msg.(CommandListMsg)
	if !ok {
		t.Fatalf("expected CommandListMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchMessages tests ---

func TestFetchMessages_ReturnsMessageList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "msg-1", "role": "user"},
			{"id": "msg-2", "role": "assistant"},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchMessages(client, "ses-1")
	msg := cmd()

	loaded, ok := msg.(MessageListMsg)
	if !ok {
		t.Fatalf("expected MessageListMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if loaded.SessionID != "ses-1" {
		t.Errorf("expected SessionID 'ses-1', got %q", loaded.SessionID)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
}

func TestFetchMessages_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "message error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchMessages(client, "ses-1")
	msg := cmd()

	loaded, ok := msg.(MessageListMsg)
	if !ok {
		t.Fatalf("expected MessageListMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if loaded.SessionID != "ses-1" {
		t.Errorf("expected SessionID 'ses-1' even on error, got %q", loaded.SessionID)
	}
}

// --- fetchPlugins tests ---

func TestFetchPlugins_ReturnsPluginList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "omt", "name": "omt"},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchPlugins(client)
	msg := cmd()

	loaded, ok := msg.(PluginListMsg)
	if !ok {
		t.Fatalf("expected PluginListMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(loaded.Plugins))
	}
	if loaded.Plugins[0].Name != "omt" {
		t.Errorf("expected plugin name 'omt', got %q", loaded.Plugins[0].Name)
	}
}

func TestFetchPlugins_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "plugin error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchPlugins(client)
	msg := cmd()

	loaded, ok := msg.(PluginListMsg)
	if !ok {
		t.Fatalf("expected PluginListMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fetchProviderBalance tests ---

func TestFetchProviderBalance_WithLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remaining := 42.5
		usage := 7.5
		json.NewEncoder(w).Encode(map[string]any{
			"provider":  "openrouter",
			"remaining": remaining,
			"usage":     usage,
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchProviderBalance(client, "openrouter")
	msg := cmd()

	loaded, ok := msg.(ProviderBalanceMsg)
	if !ok {
		t.Fatalf("expected ProviderBalanceMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if loaded.Balance == nil {
		t.Fatal("expected non-nil Balance")
	}
	if loaded.Balance.Provider != "openrouter" {
		t.Errorf("expected provider 'openrouter', got %q", loaded.Balance.Provider)
	}
	if loaded.Balance.Remaining != 42.5 {
		t.Errorf("expected remaining 42.5, got %f", loaded.Balance.Remaining)
	}
	if !loaded.Balance.HasLimit {
		t.Error("expected HasLimit=true when remaining is present")
	}
	if loaded.Balance.Usage != 7.5 {
		t.Errorf("expected usage 7.5, got %f", loaded.Balance.Usage)
	}
}

func TestFetchProviderBalance_NoLimitNoUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"provider": "local",
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchProviderBalance(client, "local")
	msg := cmd()

	loaded, ok := msg.(ProviderBalanceMsg)
	if !ok {
		t.Fatalf("expected ProviderBalanceMsg, got %T", msg)
	}
	// When there's no limit and no usage, Balance should be nil
	if loaded.Balance != nil {
		t.Errorf("expected nil Balance when no limit and no usage, got %+v", loaded.Balance)
	}
}

func TestFetchProviderBalance_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "balance error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := fetchProviderBalance(client, "openrouter")
	msg := cmd()

	loaded, ok := msg.(ProviderBalanceMsg)
	if !ok {
		t.Fatalf("expected ProviderBalanceMsg, got %T", msg)
	}
	if loaded.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- sendPrompt tests ---

func TestSendPrompt_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	input := api.PromptInput{
		Parts: []api.PromptPart{{Type: "text", Text: "hello"}},
	}
	cmd := sendPrompt(client, "ses-1", input)
	msg := cmd()

	sent, ok := msg.(PromptSentMsg)
	if !ok {
		t.Fatalf("expected PromptSentMsg, got %T", msg)
	}
	if sent.Err != nil {
		t.Errorf("expected no error, got %v", sent.Err)
	}
}

func TestSendPrompt_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "prompt error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	input := api.PromptInput{
		Parts: []api.PromptPart{{Type: "text", Text: "hello"}},
	}
	cmd := sendPrompt(client, "ses-1", input)
	msg := cmd()

	sent, ok := msg.(PromptSentMsg)
	if !ok {
		t.Fatalf("expected PromptSentMsg, got %T", msg)
	}
	if sent.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- abortSession tests ---

func TestAbortSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := abortSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(AbortSentMsg)
	if !ok {
		t.Fatalf("expected AbortSentMsg, got %T", msg)
	}
	if sent.Err != nil {
		t.Errorf("expected no error, got %v", sent.Err)
	}
}

func TestAbortSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "abort error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := abortSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(AbortSentMsg)
	if !ok {
		t.Fatalf("expected AbortSentMsg, got %T", msg)
	}
	if sent.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- createSession tests ---

func TestCreateSession_ReturnsSessionInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "new-ses",
			"title": "New Session",
			"time":  map[string]any{"created": 1000, "updated": 2000},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := createSession(client, api.SessionCreateInput{Title: "New Session"})
	msg := cmd()

	created, ok := msg.(SessionCreatedLocalMsg)
	if !ok {
		t.Fatalf("expected SessionCreatedLocalMsg, got %T", msg)
	}
	if created.Err != nil {
		t.Fatalf("unexpected error: %v", created.Err)
	}
	if created.Session == nil {
		t.Fatal("expected non-nil Session")
	}
	if created.Session.ID != "new-ses" {
		t.Errorf("expected session ID 'new-ses', got %q", created.Session.ID)
	}
}

func TestCreateSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "create error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := createSession(client, api.SessionCreateInput{Title: "New"})
	msg := cmd()

	created, ok := msg.(SessionCreatedLocalMsg)
	if !ok {
		t.Fatalf("expected SessionCreatedLocalMsg, got %T", msg)
	}
	if created.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- revertSession tests ---

func TestRevertSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := revertSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(RevertSentMsg)
	if !ok {
		t.Fatalf("expected RevertSentMsg, got %T", msg)
	}
	if sent.Err != nil {
		t.Errorf("expected no error, got %v", sent.Err)
	}
}

func TestRevertSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "revert error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := revertSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(RevertSentMsg)
	if !ok {
		t.Fatalf("expected RevertSentMsg, got %T", msg)
	}
	if sent.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- unrevertSession tests ---

func TestUnrevertSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := unrevertSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(UnrevertSentMsg)
	if !ok {
		t.Fatalf("expected UnrevertSentMsg, got %T", msg)
	}
	if sent.Err != nil {
		t.Errorf("expected no error, got %v", sent.Err)
	}
}

func TestUnrevertSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "unrevert error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := unrevertSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(UnrevertSentMsg)
	if !ok {
		t.Fatalf("expected UnrevertSentMsg, got %T", msg)
	}
	if sent.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- branchSession tests ---

func TestBranchSession_ReturnsNewSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":       "branch-ses",
			"title":    "Branch",
			"parentID": "ses-1",
			"time":     map[string]any{"created": 5000, "updated": 5000},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := branchSession(client, "ses-1", "Branch")
	msg := cmd()

	done, ok := msg.(BranchDoneMsg)
	if !ok {
		t.Fatalf("expected BranchDoneMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Fatalf("unexpected error: %v", done.Err)
	}
	if done.Session == nil {
		t.Fatal("expected non-nil Session")
	}
	if done.Session.ID != "branch-ses" {
		t.Errorf("expected session ID 'branch-ses', got %q", done.Session.ID)
	}
	if done.Session.ParentID != "ses-1" {
		t.Errorf("expected parentID 'ses-1', got %q", done.Session.ParentID)
	}
}

func TestBranchSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "branch error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := branchSession(client, "ses-1", "Branch")
	msg := cmd()

	done, ok := msg.(BranchDoneMsg)
	if !ok {
		t.Fatalf("expected BranchDoneMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- rewindSession tests ---

func TestRewindSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := rewindSession(client, "ses-1", "msg-5", 3)
	msg := cmd()

	done, ok := msg.(RewindDoneMsg)
	if !ok {
		t.Fatalf("expected RewindDoneMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
	if done.TurnIndex != 3 {
		t.Errorf("expected TurnIndex 3, got %d", done.TurnIndex)
	}
}

func TestRewindSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "rewind error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := rewindSession(client, "ses-1", "msg-5", 3)
	msg := cmd()

	done, ok := msg.(RewindDoneMsg)
	if !ok {
		t.Fatalf("expected RewindDoneMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if done.TurnIndex != 3 {
		t.Errorf("expected TurnIndex preserved on error, got %d", done.TurnIndex)
	}
}

// --- summarizeSession tests ---

func TestSummarizeSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := summarizeSession(client, "ses-1")
	msg := cmd()

	done, ok := msg.(CompactDoneMsg)
	if !ok {
		t.Fatalf("expected CompactDoneMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestSummarizeSession_ReturnsWrappedErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "summarize error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := summarizeSession(client, "ses-1")
	msg := cmd()

	done, ok := msg.(CompactDoneMsg)
	if !ok {
		t.Fatalf("expected CompactDoneMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(done.Err.Error(), "context compaction failed") {
		t.Errorf("expected wrapped error message, got %q", done.Err.Error())
	}
}

// --- archiveSession tests ---

func TestArchiveSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := archiveSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(ArchiveSentMsg)
	if !ok {
		t.Fatalf("expected ArchiveSentMsg, got %T", msg)
	}
	if sent.Err != nil {
		t.Errorf("expected no error, got %v", sent.Err)
	}
}

func TestArchiveSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "archive error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := archiveSession(client, "ses-1")
	msg := cmd()

	sent, ok := msg.(ArchiveSentMsg)
	if !ok {
		t.Fatalf("expected ArchiveSentMsg, got %T", msg)
	}
	if sent.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- renameSession tests ---

func TestRenameSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := renameSession(client, "ses-1", "New Title")
	msg := cmd()

	done, ok := msg.(SessionRenamedMsg)
	if !ok {
		t.Fatalf("expected SessionRenamedMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestRenameSession_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "rename error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := renameSession(client, "ses-1", "New Title")
	msg := cmd()

	done, ok := msg.(SessionRenamedMsg)
	if !ok {
		t.Fatalf("expected SessionRenamedMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- patchSessionAgent tests ---

func TestPatchSessionAgent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := patchSessionAgent(client, "ses-1", "executor")
	msg := cmd()

	done, ok := msg.(SessionAgentPatchedMsg)
	if !ok {
		t.Fatalf("expected SessionAgentPatchedMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestPatchSessionAgent_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "patch error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := patchSessionAgent(client, "ses-1", "executor")
	msg := cmd()

	done, ok := msg.(SessionAgentPatchedMsg)
	if !ok {
		t.Fatalf("expected SessionAgentPatchedMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- reconnectMCP tests ---

func TestReconnectMCP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := reconnectMCP(client, "filesystem")
	msg := cmd()

	result, ok := msg.(MCPReconnectResultMsg)
	if !ok {
		t.Fatalf("expected MCPReconnectResultMsg, got %T", msg)
	}
	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
	if result.Name != "filesystem" {
		t.Errorf("expected name 'filesystem', got %q", result.Name)
	}
}

func TestReconnectMCP_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "reconnect error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := reconnectMCP(client, "filesystem")
	msg := cmd()

	result, ok := msg.(MCPReconnectResultMsg)
	if !ok {
		t.Fatalf("expected MCPReconnectResultMsg, got %T", msg)
	}
	if result.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.Name != "filesystem" {
		t.Errorf("expected name preserved on error, got %q", result.Name)
	}
}

// --- patchScopedModels tests ---

func TestPatchScopedModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := patchScopedModels(client, []string{"gpt-4", "claude-3"})
	msg := cmd()

	done, ok := msg.(ModelScopedDoneMsg)
	if !ok {
		t.Fatalf("expected ModelScopedDoneMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestPatchScopedModels_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "config error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := patchScopedModels(client, []string{"gpt-4"})
	msg := cmd()

	done, ok := msg.(ModelScopedDoneMsg)
	if !ok {
		t.Fatalf("expected ModelScopedDoneMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- storeOpenRouterAuth tests ---

func TestStoreOpenRouterAuth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := storeOpenRouterAuth(client, "sk-test-key")
	msg := cmd()

	done, ok := msg.(AuthStoredMsg)
	if !ok {
		t.Fatalf("expected AuthStoredMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestStoreOpenRouterAuth_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "auth error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := storeOpenRouterAuth(client, "sk-test-key")
	msg := cmd()

	done, ok := msg.(AuthStoredMsg)
	if !ok {
		t.Fatalf("expected AuthStoredMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- toggleAgent tests ---

func TestToggleAgent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := toggleAgent(client, "executor", true)
	msg := cmd()

	done, ok := msg.(AgentToggleDoneMsg)
	if !ok {
		t.Fatalf("expected AgentToggleDoneMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestToggleAgent_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "toggle error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := toggleAgent(client, "executor", false)
	msg := cmd()

	done, ok := msg.(AgentToggleDoneMsg)
	if !ok {
		t.Fatalf("expected AgentToggleDoneMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- replyPermission tests ---

func TestReplyPermission_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := replyPermission(client, "ses-1", "perm-1", "once")
	msg := cmd()

	done, ok := msg.(PermissionRepliedMsg)
	if !ok {
		t.Fatalf("expected PermissionRepliedMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Errorf("expected no error, got %v", done.Err)
	}
}

func TestReplyPermission_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "permission error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := replyPermission(client, "ses-1", "perm-1", "reject")
	msg := cmd()

	done, ok := msg.(PermissionRepliedMsg)
	if !ok {
		t.Fatalf("expected PermissionRepliedMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- forkSessionAtTurn tests ---

func TestForkSessionAtTurn_ReturnsNewSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":       "fork-ses",
			"title":    "Fork at turn 2",
			"parentID": "ses-1",
			"time":     map[string]any{"created": 6000, "updated": 6000},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := forkSessionAtTurn(client, "ses-1", "msg-3", "Fork at turn 2", 2)
	msg := cmd()

	done, ok := msg.(ForkDoneMsg)
	if !ok {
		t.Fatalf("expected ForkDoneMsg, got %T", msg)
	}
	if done.Err != nil {
		t.Fatalf("unexpected error: %v", done.Err)
	}
	if done.Session == nil {
		t.Fatal("expected non-nil Session")
	}
	if done.Session.ID != "fork-ses" {
		t.Errorf("expected session ID 'fork-ses', got %q", done.Session.ID)
	}
	if done.TurnIndex != 2 {
		t.Errorf("expected TurnIndex 2, got %d", done.TurnIndex)
	}
}

func TestForkSessionAtTurn_ReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "fork error"})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "/tmp", "")
	cmd := forkSessionAtTurn(client, "ses-1", "msg-3", "Fork", 2)
	msg := cmd()

	done, ok := msg.(ForkDoneMsg)
	if !ok {
		t.Fatalf("expected ForkDoneMsg, got %T", msg)
	}
	if done.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if done.TurnIndex != 2 {
		t.Errorf("expected TurnIndex preserved on error, got %d", done.TurnIndex)
	}
}

// --- runUserShell tests ---

func TestRunUserShell_ReturnsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}

	cmd := runUserShell("echo hello", "/tmp")
	msg := cmd()

	result, ok := msg.(ShellResultMsg)
	if !ok {
		t.Fatalf("expected ShellResultMsg, got %T", msg)
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Command != "echo hello" {
		t.Errorf("expected command 'echo hello', got %q", result.Command)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", result.Output)
	}
}

func TestRunUserShell_ReturnsErrorForBadCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}

	cmd := runUserShell("false", "/tmp")
	msg := cmd()

	result, ok := msg.(ShellResultMsg)
	if !ok {
		t.Fatalf("expected ShellResultMsg, got %T", msg)
	}
	if result.Err == nil {
		t.Fatal("expected error for failing command, got nil")
	}
}

func TestRunUserShell_CapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}

	cmd := runUserShell("echo errout >&2", "/tmp")
	msg := cmd()

	result, ok := msg.(ShellResultMsg)
	if !ok {
		t.Fatalf("expected ShellResultMsg, got %T", msg)
	}
	if !strings.Contains(result.Output, "errout") {
		t.Errorf("expected output to contain stderr 'errout', got %q", result.Output)
	}
}

// --- waitForSSE tests ---

func TestWaitForSSE_ReturnsEventMsg(t *testing.T) {
	ch := make(chan api.ServerEvent, 1)
	ch <- api.ServerEvent{Type: "session.created", Properties: map[string]any{"sessionID": "ses-1"}}

	cmd := waitForSSE(ch)
	msg := cmd()

	evt, ok := msg.(SSEEventMsg)
	if !ok {
		t.Fatalf("expected SSEEventMsg, got %T", msg)
	}
	if evt.Event.Type != "session.created" {
		t.Errorf("expected event type 'session.created', got %q", evt.Event.Type)
	}
}

func TestWaitForSSE_ReturnsDisconnectedOnClosedChannel(t *testing.T) {
	ch := make(chan api.ServerEvent)
	close(ch)

	cmd := waitForSSE(ch)
	msg := cmd()

	_, ok := msg.(SSEDisconnectedMsg)
	if !ok {
		t.Fatalf("expected SSEDisconnectedMsg, got %T", msg)
	}
}

// --- sessionInfoFromAPI tests ---

func TestSessionInfoFromAPI_FullInfo(t *testing.T) {
	info := sessionInfoFromAPI(sessionInfoFixture())
	if info.ID != "ses-api" {
		t.Errorf("expected ID 'ses-api', got %q", info.ID)
	}
	if info.Title != "API Session" {
		t.Errorf("expected Title 'API Session', got %q", info.Title)
	}
	if info.ModelID != "test-model" {
		t.Errorf("expected ModelID 'test-model', got %q", info.ModelID)
	}
	if info.ProviderID != "test-provider" {
		t.Errorf("expected ProviderID 'test-provider', got %q", info.ProviderID)
	}
}

func TestSessionInfoFromAPI_NoModel(t *testing.T) {
	s := sessionInfoFixture()
	s.Model = nil
	info := sessionInfoFromAPI(s)
	if info.ModelID != "" {
		t.Errorf("expected empty ModelID, got %q", info.ModelID)
	}
	if info.ProviderID != "" {
		t.Errorf("expected empty ProviderID, got %q", info.ProviderID)
	}
}
