package tui

import (
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
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


