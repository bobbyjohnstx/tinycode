package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
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

func TestWarmupModel_NoopsWhenModelIsNil(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	called := false
	sm.SetModelWarmup(func(_ context.Context, _ *provider.Model) {
		called = true
	})

	sm.warmupModel(nil)

	if called {
		t.Error("expected warmup not to be called when model is nil")
	}
}

func TestWarmupModel_InvokesCallbackWhenSet(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	var warmedModel *provider.Model
	sm.SetModelWarmup(func(_ context.Context, m *provider.Model) {
		warmedModel = m
	})

	model := &provider.Model{ID: "test-model", ProviderID: "test-provider"}
	sm.warmupModel(model)

	if warmedModel == nil {
		t.Fatal("expected warmup callback to be invoked")
	}
	if warmedModel.ID != "test-model" {
		t.Errorf("expected model ID 'test-model', got %q", warmedModel.ID)
	}
}

func TestWarmupModel_NoopsWhenNoCallbackSet(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// No SetModelWarmup called — should not panic.
	model := &provider.Model{ID: "m1"}
	sm.warmupModel(model)
}

func TestSetModelWarmup_OverridesPreviousCallback(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	var firstCalled, secondCalled bool
	sm.SetModelWarmup(func(_ context.Context, _ *provider.Model) {
		firstCalled = true
	})
	sm.SetModelWarmup(func(_ context.Context, _ *provider.Model) {
		secondCalled = true
	})

	sm.warmupModel(&provider.Model{ID: "m"})

	if firstCalled {
		t.Error("first callback should not be called after override")
	}
	if !secondCalled {
		t.Error("second callback should be called")
	}
}

func TestJobNotification_IdleParent_WakesSession(t *testing.T) {
	b := bus.New()
	defer b.Close()
	_ = newMinimalSM(t, b) // SM must be alive to subscribe to job.completed

	// Subscribe to session.prompt to verify the wake-up.
	promptSub := b.Subscribe("session.prompt")
	defer promptSub.Unsubscribe()

	// Simulate a job.completed event for an idle parent (not in sm.sessions).
	b.Publish("job.completed", map[string]any{
		"jobID":           "job_1",
		"parentSessionID": "ses-idle-parent",
		"status":          "completed",
		"result":          "task output here",
		"error":           "",
	})

	select {
	case evt := <-promptSub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatalf("expected map properties, got %T", evt.Properties)
		}
		if props["sessionID"] != "ses-idle-parent" {
			t.Errorf("expected sessionID 'ses-idle-parent', got %v", props["sessionID"])
		}
		content, _ := props["content"].(string)
		if content == "" {
			t.Fatal("expected non-empty notification content")
		}
		if !strings.Contains(content, "job_1") {
			t.Errorf("expected content to include job ID, got %q", content)
		}
		if !strings.Contains(content, "task output here") {
			t.Errorf("expected content to include result, got %q", content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session.prompt from job notification")
	}
}

func TestJobNotification_BusyParent_QueuedForLater(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// Register the parent as busy.
	registerActiveSession(sm, "ses-busy-parent", nil, "build", "/tmp")

	// Publish a job.completed event.
	b.Publish("job.completed", map[string]any{
		"jobID":           "job_2",
		"parentSessionID": "ses-busy-parent",
		"status":          "completed",
		"result":          "queued result",
		"error":           "",
	})

	// Allow async processing.
	time.Sleep(100 * time.Millisecond)

	// Drain should return the queued notification.
	notification := sm.drainJobNotifications("ses-busy-parent")
	if notification == "" {
		t.Fatal("expected pending notification for busy parent")
	}
	if !strings.Contains(notification, "job_2") {
		t.Errorf("expected notification to include job ID, got %q", notification)
	}
	if !strings.Contains(notification, "queued result") {
		t.Errorf("expected notification to include result, got %q", notification)
	}

	// Second drain should be empty.
	if second := sm.drainJobNotifications("ses-busy-parent"); second != "" {
		t.Errorf("expected empty second drain, got %q", second)
	}
}

func TestJobNotification_AbortedParent_QueuedNotWoken(t *testing.T) {
	b := bus.New()
	defer b.Close()
	sm := newMinimalSM(t, b)

	// Mark the session as aborted.
	sm.mu.Lock()
	sm.abortedSessions["ses-aborted"] = true
	sm.mu.Unlock()

	// Subscribe to session.prompt to verify no wake-up occurs.
	promptSub := b.Subscribe("session.prompt")
	defer promptSub.Unsubscribe()

	// Publish a job.completed event for the aborted session.
	b.Publish("job.completed", map[string]any{
		"jobID":           "job_3",
		"parentSessionID": "ses-aborted",
		"status":          "completed",
		"result":          "aborted result",
		"error":           "",
	})

	// No session.prompt should be published.
	select {
	case evt := <-promptSub.C:
		props, _ := evt.Properties.(map[string]any)
		if props["sessionID"] == "ses-aborted" {
			t.Fatal("expected no session.prompt for aborted parent")
		}
	case <-time.After(200 * time.Millisecond):
		// Good — no wake-up.
	}

	// But the notification should be queued.
	notification := sm.drainJobNotifications("ses-aborted")
	if notification == "" {
		t.Fatal("expected pending notification for aborted parent")
	}
}

func TestJobNotification_ErrorChild_NotifiesParent(t *testing.T) {
	b := bus.New()
	defer b.Close()
	_ = newMinimalSM(t, b) // SM must be alive to subscribe to job.completed

	promptSub := b.Subscribe("session.prompt")
	defer promptSub.Unsubscribe()

	// Publish a failed job.completed event (parent is idle).
	b.Publish("job.completed", map[string]any{
		"jobID":           "job_4",
		"parentSessionID": "ses-err-parent",
		"status":          "failed",
		"result":          "",
		"error":           "something went wrong",
	})

	select {
	case evt := <-promptSub.C:
		props, _ := evt.Properties.(map[string]any)
		content, _ := props["content"].(string)
		if !strings.Contains(content, "failed") {
			t.Errorf("expected content to mention failure, got %q", content)
		}
		if !strings.Contains(content, "something went wrong") {
			t.Errorf("expected content to include error text, got %q", content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error notification")
	}
}

func TestFormatJobNotification_Success(t *testing.T) {
	msg := formatJobNotification("job_42", "completed", "all done", "")
	if !strings.Contains(msg, "job_42") {
		t.Errorf("expected job ID in message, got %q", msg)
	}
	if !strings.Contains(msg, "success") {
		t.Errorf("expected 'success' in message, got %q", msg)
	}
	if !strings.Contains(msg, "all done") {
		t.Errorf("expected result in message, got %q", msg)
	}
}

func TestFormatJobNotification_Failure(t *testing.T) {
	msg := formatJobNotification("job_43", "failed", "", "boom")
	if !strings.Contains(msg, "failed") {
		t.Errorf("expected 'failed' in message, got %q", msg)
	}
	if !strings.Contains(msg, "boom") {
		t.Errorf("expected error text in message, got %q", msg)
	}
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
