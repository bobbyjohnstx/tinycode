package tui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// newTestClient creates an api.Client backed by a no-op HTTP server.
func newTestClient(t *testing.T) (*api.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	return api.New(srv.URL, "/tmp", ""), srv.Close
}

func TestPromptQueue_QueuedWhenWorking(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.state.CurrentModel = ModelSelection{
		ProviderID: "lm-studio",
		ModelID:    "ornith-1.0-9b-mlx",
	}
	app.status.SetWorking(true)

	ca := &connectedApp{app: app}

	result, _ := ca.Update(PromptSubmittedMsg{Content: "follow-up question"})
	updated := result.(*connectedApp)

	q := updated.sessionQueues["ses_123"]
	if len(q) != 1 {
		t.Fatalf("expected 1 queued prompt, got %d", len(q))
	}
	if q[0] != "follow-up question" {
		t.Errorf("expected queued prompt 'follow-up question', got %q", q[0])
	}
	if updated.app.status.queueCount != 1 {
		t.Errorf("expected queueCount 1, got %d", updated.app.status.queueCount)
	}
}

func TestPromptQueue_DrainOnIdle(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.status.SetWorking(true)

	ca := &connectedApp{
		app:    app,
		client: client,
		sessionQueues: map[string][]string{
			"ses_123": {"queued prompt 1", "queued prompt 2"},
		},
	}
	ca.app.status.SetQueueCount(2)

	// Transition from working to idle.
	result, cmd := ca.Update(SessionStatusMsg{
		SessionID: "ses_123",
		Status:    SessionStatus{Working: false},
	})
	updated := result.(*connectedApp)

	// One prompt should have been dequeued.
	q := updated.sessionQueues["ses_123"]
	if len(q) != 1 {
		t.Fatalf("expected 1 remaining queued prompt, got %d", len(q))
	}
	if q[0] != "queued prompt 2" {
		t.Errorf("expected remaining prompt 'queued prompt 2', got %q", q[0])
	}
	if updated.app.status.queueCount != 1 {
		t.Errorf("expected queueCount 1, got %d", updated.app.status.queueCount)
	}

	// The command batch should include a PromptSubmittedMsg for the drained prompt.
	msgs := flattenCmd(cmd)
	submitted, found := findMsg[PromptSubmittedMsg](msgs)
	if !found {
		t.Fatal("expected PromptSubmittedMsg in commands after drain")
	}
	if submitted.Content != "queued prompt 1" {
		t.Errorf("expected drained prompt 'queued prompt 1', got %q", submitted.Content)
	}
}

func TestPromptQueue_ClearQueue(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"

	ca := &connectedApp{
		app: app,
		sessionQueues: map[string][]string{
			"ses_123": {"p1", "p2", "p3"},
		},
	}
	ca.app.status.SetQueueCount(3)

	result, _ := ca.Update(PromptSubmittedMsg{Content: "/clear-queue"})
	updated := result.(*connectedApp)

	if len(updated.sessionQueues["ses_123"]) != 0 {
		t.Errorf("expected empty queue after /clear-queue, got %d", len(updated.sessionQueues["ses_123"]))
	}
	if updated.app.status.queueCount != 0 {
		t.Errorf("expected queueCount 0 after /clear-queue, got %d", updated.app.status.queueCount)
	}
}

func TestPromptQueue_ClearQueueAlt(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"

	ca := &connectedApp{
		app: app,
		sessionQueues: map[string][]string{
			"ses_123": {"p1"},
		},
	}
	ca.app.status.SetQueueCount(1)

	result, _ := ca.Update(PromptSubmittedMsg{Content: "/queue clear"})
	updated := result.(*connectedApp)

	if len(updated.sessionQueues["ses_123"]) != 0 {
		t.Errorf("expected empty queue after /queue clear, got %d", len(updated.sessionQueues["ses_123"]))
	}
}

func TestPromptQueue_ClearQueueRequestMsg(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"

	ca := &connectedApp{
		app: app,
		sessionQueues: map[string][]string{
			"ses_123": {"p1", "p2"},
		},
	}
	ca.app.status.SetQueueCount(2)

	result, _ := ca.Update(ClearQueueRequestMsg{})
	updated := result.(*connectedApp)

	if len(updated.sessionQueues["ses_123"]) != 0 {
		t.Errorf("expected empty queue after ClearQueueRequestMsg, got %d", len(updated.sessionQueues["ses_123"]))
	}
	if updated.app.status.queueCount != 0 {
		t.Errorf("expected queueCount 0 after ClearQueueRequestMsg, got %d", updated.app.status.queueCount)
	}
}

func TestPromptQueue_SlashCommandsBypassQueue(t *testing.T) {
	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.state.CurrentModel = ModelSelection{
		ProviderID: "lm-studio",
		ModelID:    "ornith-1.0-9b-mlx",
	}
	app.state.Agents = []api.AgentInfo{{Name: "build"}}
	app.status.SetWorking(true)

	ca := &connectedApp{app: app}

	// /btw should NOT be queued even when working.
	result, _ := ca.Update(PromptSubmittedMsg{Content: "/btw this is a side note"})
	updated := result.(*connectedApp)

	if len(updated.sessionQueues["ses_123"]) != 0 {
		t.Errorf("expected /btw to bypass queue, but got %d queued", len(updated.sessionQueues["ses_123"]))
	}
}

func TestPromptQueue_NoDrainWhenDifferentSession(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_123"
	app.status.SetWorking(true)

	ca := &connectedApp{
		app:    app,
		client: client,
		sessionQueues: map[string][]string{
			"ses_123": {"queued"},
		},
	}
	ca.app.status.SetQueueCount(1)

	// Status update for a different session should not drain.
	result, _ := ca.Update(SessionStatusMsg{
		SessionID: "ses_other",
		Status:    SessionStatus{Working: false},
	})
	updated := result.(*connectedApp)

	if len(updated.sessionQueues["ses_123"]) != 1 {
		t.Errorf("expected queue unchanged for different session, got %d", len(updated.sessionQueues["ses_123"]))
	}
}

func TestPromptQueue_SessionSwitchRefreshesQueueCount(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	app := NewApp("http://localhost:4096")
	app.width = 100
	app.height = 30
	app.ready = true
	app.state.ActiveSession = "ses_A"
	app.status.SetQueueCount(2)

	ca := &connectedApp{
		app:    app,
		client: client,
		sessionQueues: map[string][]string{
			"ses_A": {"a1", "a2"},
			"ses_B": {"b1"},
		},
	}

	result, _ := ca.Update(SessionSwitchedMsg{SessionID: "ses_B"})
	updated := result.(*connectedApp)
	if updated.app.status.queueCount != 1 {
		t.Errorf("expected queueCount 1 for ses_B, got %d", updated.app.status.queueCount)
	}
	if len(updated.sessionQueues["ses_A"]) != 2 {
		t.Errorf("expected ses_A queue preserved, got %d", len(updated.sessionQueues["ses_A"]))
	}

	result, _ = updated.Update(SessionSwitchedMsg{SessionID: "ses_empty"})
	updated = result.(*connectedApp)
	if updated.app.status.queueCount != 0 {
		t.Errorf("expected queueCount 0 for empty session, got %d", updated.app.status.queueCount)
	}
}
