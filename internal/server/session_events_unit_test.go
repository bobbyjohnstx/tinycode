package server

import (
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

// newMinimalSM creates a SessionManager with only a bus and registry, suitable
// for testing bridge functions that only publish events.
func newMinimalSM(t *testing.T, b *bus.Bus) *SessionManager {
	t.Helper()
	db := testDB(t)
	reg := provider.NewRegistry()
	return NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)
}

// registerActiveSession inserts a synthetic activeSession into the SessionManager
// so bridge functions can find it via getActive().
func registerActiveSession(sm *SessionManager, sessionID string, model *provider.Model, agent, dir string) {
	sm.mu.Lock()
	sm.sessions[sessionID] = &activeSession{
		model: model,
		agent: agent,
		dir:   dir,
		idMap: make(map[string]string),
	}
	sm.mu.Unlock()
}

func TestBridgeUserMessage_PublishesMessageUpdatedWithUserRole(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	model := &provider.Model{
		ID:         "test-model",
		ProviderID: "test-provider",
	}
	registerActiveSession(sm, "ses_1", model, "build", "/tmp")

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_user_1",
		Role: session.RoleUser,
		Parts: []session.Part{
			session.TextPart("hello"),
		},
	}

	active := sm.getActive("ses_1")
	now := time.Now().UnixMilli()
	sm.bridgeUserMessage("ses_1", active, msg, now)

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_1" {
			t.Errorf("expected sessionID 'ses_1', got %v", props["sessionID"])
		}
		info := props["info"].(map[string]any)
		if info["role"] != "user" {
			t.Errorf("expected role 'user', got %v", info["role"])
		}
		if info["id"] != "msg_user_1" {
			t.Errorf("expected message ID 'msg_user_1', got %v", info["id"])
		}
		timeMap := info["time"].(map[string]any)
		if timeMap["created"] != now {
			t.Errorf("expected time.created %d, got %v", now, timeMap["created"])
		}
		modelMap := info["model"].(map[string]any)
		if modelMap["providerID"] != "test-provider" {
			t.Errorf("expected providerID 'test-provider', got %v", modelMap["providerID"])
		}
		if modelMap["modelID"] != "test-model" {
			t.Errorf("expected modelID 'test-model', got %v", modelMap["modelID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message.updated event")
	}
}

func TestBridgeUserMessage_PublishesTextPartForEachPart(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_2", nil, "build", "/tmp")

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_u2",
		Role: session.RoleUser,
		Parts: []session.Part{
			session.TextPart("part one"),
			session.TextPart("part two"),
		},
	}

	active := sm.getActive("ses_2")
	sm.bridgeUserMessage("ses_2", active, msg, time.Now().UnixMilli())

	// Should receive 2 part events
	for i := 0; i < 2; i++ {
		select {
		case evt := <-sub.C:
			props := evt.Properties.(map[string]any)
			part := props["part"].(map[string]any)
			if part["type"] != "text" {
				t.Errorf("part %d: expected type 'text', got %v", i, part["type"])
			}
			if part["messageID"] != "msg_u2" {
				t.Errorf("part %d: expected messageID 'msg_u2', got %v", i, part["messageID"])
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for part event %d", i)
		}
	}
}

func TestBridgeUserMessage_StoresUserMsgIDOnActiveSession(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_3", nil, "build", "/tmp")

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_parent",
		Role: session.RoleUser,
		Parts: []session.Part{
			session.TextPart("hello"),
		},
	}

	active := sm.getActive("ses_3")
	sm.bridgeUserMessage("ses_3", active, msg, time.Now().UnixMilli())

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map properties on bus event")
		}
		info, _ := props["info"].(map[string]any)
		if info["id"] != "msg_parent" {
			t.Errorf("expected message ID 'msg_parent', got %v", info["id"])
		}
		if info["role"] != "user" {
			t.Errorf("expected role 'user', got %v", info["role"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message.updated bus event")
	}
}

func TestBridgeToolMessage_PublishesMessageUpdatedWithToolRole(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_tool_1",
		Role: session.RoleTool,
		Parts: []session.Part{
			session.ToolResultPart("tc_1", "shell", "output text", false),
		},
	}

	now := time.Now().UnixMilli()
	sm.bridgeToolMessage("ses_tool", msg, now)

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		info := props["info"].(map[string]any)
		if info["role"] != "tool" {
			t.Errorf("expected role 'tool', got %v", info["role"])
		}
		if info["sessionID"] != "ses_tool" {
			t.Errorf("expected sessionID 'ses_tool', got %v", info["sessionID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message.updated event")
	}
}

func TestBridgeToolMessage_PublishesToolResultParts(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_tool_2",
		Role: session.RoleTool,
		Parts: []session.Part{
			session.ToolResultPart("tc_1", "shell", "output text", false),
			session.ToolResultPart("tc_2", "read", "file contents", true),
		},
	}

	sm.bridgeToolMessage("ses_t2", msg, time.Now().UnixMilli())

	for i := 0; i < 2; i++ {
		select {
		case evt := <-sub.C:
			props := evt.Properties.(map[string]any)
			part := props["part"].(map[string]any)
			if part["type"] != "tool" {
				t.Errorf("part %d: expected type 'tool', got %v", i, part["type"])
			}
			if _, ok := part["callID"]; !ok {
				t.Errorf("part %d: expected callID field", i)
			}
			if _, ok := part["tool"]; !ok {
				t.Errorf("part %d: expected tool field", i)
			}
			state, ok := part["state"].(map[string]any)
			if !ok {
				t.Fatalf("part %d: expected state map", i)
			}
			if _, ok := state["output"]; !ok {
				t.Errorf("part %d: expected state.output field", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for tool part %d", i)
		}
	}
}

func TestBridgeToolBegin_PublishesToolPart(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_tb", nil, "build", "/tmp")

	// Set assistMsgID so bridgeToolBegin doesn't bail out.
	active := sm.getActive("ses_tb")
	active.mu.Lock()
	active.assistMsgID = "msg_assist_1"
	active.mu.Unlock()

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	evt := bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_tb",
			"toolCallID": "tc_begin_1",
			"toolName":   "shell",
		},
	}

	sm.bridgeToolBegin(evt)

	select {
	case received := <-sub.C:
		props := received.Properties.(map[string]any)
		part := props["part"].(map[string]any)
		if part["type"] != "tool" {
			t.Errorf("expected type 'tool', got %v", part["type"])
		}
		if part["callID"] != "tc_begin_1" {
			t.Errorf("expected callID 'tc_begin_1', got %v", part["callID"])
		}
		if part["tool"] != "shell" {
			t.Errorf("expected tool 'shell', got %v", part["tool"])
		}
		if part["messageID"] != "msg_assist_1" {
			t.Errorf("expected messageID 'msg_assist_1', got %v", part["messageID"])
		}
		state, ok := part["state"].(map[string]any)
		if !ok {
			t.Fatal("expected state map")
		}
		if state["status"] != "running" {
			t.Errorf("expected state.status 'running', got %v", state["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tool part")
	}
}

func TestBridgeToolBegin_NoopWhenNoAssistantMessage(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_tb2", nil, "build", "/tmp")
	// Do NOT set assistMsgID — it stays empty

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	evt := bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_tb2",
			"toolCallID": "tc_begin_2",
			"toolName":   "read",
		},
	}

	sm.bridgeToolBegin(evt)

	select {
	case <-sub.C:
		t.Error("expected no event when assistMsgID is empty")
	default:
		// Good — no event published
	}
}

func TestBridgeToolEnd_PublishesToolPartWithCompletedState(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_te", nil, "build", "/tmp")

	active := sm.getActive("ses_te")
	active.mu.Lock()
	active.assistMsgID = "msg_assist_2"
	active.mu.Unlock()

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	evt := bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_te",
			"toolCallID": "tc_end_1",
			"toolName":   "shell",
			"toolArgs":   `{"command":"ls -la"}`,
		},
	}

	sm.bridgeToolEnd(evt)

	select {
	case received := <-sub.C:
		props := received.Properties.(map[string]any)
		part := props["part"].(map[string]any)
		if part["type"] != "tool" {
			t.Errorf("expected type 'tool', got %v", part["type"])
		}
		if part["callID"] != "tc_end_1" {
			t.Errorf("expected callID 'tc_end_1', got %v", part["callID"])
		}
		if part["tool"] != "shell" {
			t.Errorf("expected tool 'shell', got %v", part["tool"])
		}
		state, ok := part["state"].(map[string]any)
		if !ok {
			t.Fatal("expected state map")
		}
		if state["status"] != "completed" {
			t.Errorf("expected state.status 'completed', got %v", state["status"])
		}
		input, ok := state["input"].(map[string]any)
		if !ok {
			t.Fatal("expected state.input map")
		}
		if input["args"] != `{"command":"ls -la"}` {
			t.Errorf("expected state.input.args, got %v", input["args"])
		}
		timeMap := state["time"].(map[string]any)
		if _, ok := timeMap["start"]; !ok {
			t.Error("expected state.time.start")
		}
		if _, ok := timeMap["end"]; !ok {
			t.Error("expected state.time.end")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tool-end part")
	}
}

func TestBridgeMessageEvent_RoutesUserMessageCorrectly(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_route", nil, "build", "/tmp")

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	evt := bus.Event{
		Properties: map[string]any{
			"sessionID": "ses_route",
			"message": session.Message{
				ID:    "msg_route_1",
				Role:  session.RoleUser,
				Parts: []session.Part{session.TextPart("test routing")},
			},
		},
	}

	sm.bridgeMessageEvent(evt)

	select {
	case received := <-sub.C:
		props := received.Properties.(map[string]any)
		info := props["info"].(map[string]any)
		if info["role"] != "user" {
			t.Errorf("expected role 'user', got %v", info["role"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for routed event")
	}
}

func TestBridgeMessageEvent_IgnoresUnknownSession(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// Do NOT register a session — "unknown_ses" doesn't exist.

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	evt := bus.Event{
		Properties: map[string]any{
			"sessionID": "unknown_ses",
			"message": session.Message{
				ID:    "msg_1",
				Role:  session.RoleUser,
				Parts: []session.Part{session.TextPart("hello")},
			},
		},
	}

	sm.bridgeMessageEvent(evt)

	select {
	case <-sub.C:
		t.Error("expected no event for unknown session")
	default:
		// Good
	}
}

func TestBridgeToolBegin_PartShapeContract(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_contract", nil, "build", "/tmp")
	active := sm.getActive("ses_contract")
	active.mu.Lock()
	active.assistMsgID = "msg_contract"
	active.mu.Unlock()

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	sm.bridgeToolBegin(bus.Event{
		Properties: map[string]any{
			"sessionID":  "ses_contract",
			"toolCallID": "tc_shape_1",
			"toolName":   "read",
		},
	})

	select {
	case received := <-sub.C:
		props := received.Properties.(map[string]any)
		part := props["part"].(map[string]any)

		// Verify required contract fields exist
		requiredTopLevel := []string{"id", "sessionID", "messageID", "type", "callID", "tool", "state"}
		for _, key := range requiredTopLevel {
			if _, ok := part[key]; !ok {
				t.Errorf("missing required field %q in tool part", key)
			}
		}

		// Verify old fields are NOT present
		for _, oldKey := range []string{"toolCallID", "toolName", "toolArgs"} {
			if _, ok := part[oldKey]; ok {
				t.Errorf("legacy field %q should not be present in new tool part", oldKey)
			}
		}

		state := part["state"].(map[string]any)
		if state["status"] != "running" {
			t.Errorf("begin event should have status 'running', got %v", state["status"])
		}
		if _, ok := state["input"]; !ok {
			t.Error("state should contain 'input' field")
		}
		if _, ok := state["time"]; !ok {
			t.Error("state should contain 'time' field")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBridgeToolMessage_ErrorPartHasErrorStatus(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	sub := b.Subscribe("message.part.updated")
	defer sub.Unsubscribe()

	msg := session.Message{
		ID:   "msg_err",
		Role: session.RoleTool,
		Parts: []session.Part{
			session.ToolResultPart("tc_err", "bash", "command failed", true),
		},
	}
	sm.bridgeToolMessage("ses_err", msg, time.Now().UnixMilli())

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		part := props["part"].(map[string]any)
		if part["type"] != "tool" {
			t.Errorf("expected type 'tool', got %v", part["type"])
		}
		state := part["state"].(map[string]any)
		if state["status"] != "error" {
			t.Errorf("expected state.status 'error', got %v", state["status"])
		}
		if state["error"] != "command failed" {
			t.Errorf("expected state.error 'command failed', got %v", state["error"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBridgeMessageEvent_IgnoresInvalidProperties(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	evt := bus.Event{
		Properties: "not a map",
	}

	sm.bridgeMessageEvent(evt)

	select {
	case <-sub.C:
		t.Error("expected no event for invalid properties")
	default:
		// Good
	}
}
