package server

import (
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

func TestStatus_ReturnsEmptyMapWhenNoActiveSessions(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	status := sm.Status()
	if len(status) != 0 {
		t.Errorf("expected empty status map, got %d entries", len(status))
	}
}

func TestStatus_ReturnsBusyForActiveSessions(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_status_1", nil, "build", "/tmp")
	registerActiveSession(sm, "ses_status_2", nil, "build", "/tmp")

	status := sm.Status()
	if len(status) != 2 {
		t.Fatalf("expected 2 sessions in status, got %d", len(status))
	}
	for sid, s := range status {
		if s.Type != "busy" {
			t.Errorf("session %s: expected type 'busy', got %q", sid, s.Type)
		}
	}
}

func TestGetActive_ReturnsNilForUnknownSession(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	active := sm.getActive("nonexistent")
	if active != nil {
		t.Error("expected nil for unknown session")
	}
}

func TestGetActive_ReturnsSessionWhenRegistered(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_active_1", nil, "build", "/tmp/project")

	active := sm.getActive("ses_active_1")
	if active == nil {
		t.Fatal("expected non-nil active session")
	}
	if active.agent != "build" {
		t.Errorf("expected agent 'build', got %q", active.agent)
	}
	if active.dir != "/tmp/project" {
		t.Errorf("expected dir '/tmp/project', got %q", active.dir)
	}
}

func TestShutdown_ClearsAllSessions(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	registerActiveSession(sm, "ses_shutdown_1", nil, "build", "/tmp")
	registerActiveSession(sm, "ses_shutdown_2", nil, "build", "/tmp")

	sm.Shutdown()

	status := sm.Status()
	if len(status) != 0 {
		t.Errorf("expected all sessions cleared after shutdown, got %d", len(status))
	}
}

func TestStatus_ReflectsModelInfo(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	model := &provider.Model{
		ID:         "my-model",
		ProviderID: "my-provider",
		Name:       "My Model",
	}
	registerActiveSession(sm, "ses_model", model, "build", "/tmp")

	active := sm.getActive("ses_model")
	if active.model == nil {
		t.Fatal("expected model to be set")
	}
	if active.model.ID != "my-model" {
		t.Errorf("expected model ID 'my-model', got %q", active.model.ID)
	}
}

func TestAbort_DoesNotPanicForUnknownSession(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// Should not panic.
	sm.Abort("nonexistent_session")
}

func TestSubscribeProcessorEvents_WiresMessageBridging(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// The SessionManager subscribes to session.message in its constructor.
	// Verify by registering a session and publishing a message event,
	// then checking that message.updated was published (bridged).

	registerActiveSession(sm, "ses_wire", nil, "build", "/tmp")

	sub := b.Subscribe("message.updated")
	defer sub.Unsubscribe()

	// Publish a session.message event with a user message.
	b.Publish("session.message", map[string]any{
		"sessionID": "ses_wire",
		"message": session.Message{
			ID:    "msg_wire_1",
			Role:  session.RoleUser,
			Parts: []session.Part{session.TextPart("wired test")},
		},
	})

	// Wait briefly for the async goroutine to process.
	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		info := props["info"].(map[string]any)
		if info["role"] != "user" {
			t.Errorf("expected bridged role 'user', got %v", info["role"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bridged message.updated event")
	}
}
