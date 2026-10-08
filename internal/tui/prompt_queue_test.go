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

	if len(updated.promptQueue) != 1 {
		t.Fatalf("expected 1 queued prompt, got %d", len(updated.promptQueue))
	}
	if updated.promptQueue[0] != "follow-up question" {
		t.Errorf("expected queued prompt 'follow-up question', got %q", updated.promptQueue[0])
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
		app:         app,
		client:      client,
		promptQueue: []string{"queued prompt 1", "queued prompt 2"},
	}
	ca.app.status.SetQueueCount(2)

	// Transition from working to idle.
	result, cmd := ca.Update(SessionStatusMsg{
		SessionID: "ses_123",
		Status:    SessionStatus{Working: false},
	})
	updated := result.(*connectedApp)

	// One prompt should have been dequeued.
	if len(updated.promptQueue) != 1 {
		t.Fatalf("expected 1 remaining queued prompt, got %d", len(updated.promptQueue))
	}
	if updated.promptQueue[0] != "queued prompt 2" {
		t.Errorf("expected remaining prompt 'queued prompt 2', got %q", updated.promptQueue[0])
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
		app:         app,
		promptQueue: []string{"p1", "p2", "p3"},
	}
	ca.app.status.SetQueueCount(3)

	result, _ := ca.Update(PromptSubmittedMsg{Content: "/clear-queue"})
	updated := result.(*connectedApp)

	if len(updated.promptQueue) != 0 {
		t.Errorf("expected empty queue after /clear-queue, got %d", len(updated.promptQueue))
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
		app:         app,
		promptQueue: []string{"p1"},
	}
	ca.app.status.SetQueueCount(1)

	result, _ := ca.Update(PromptSubmittedMsg{Content: "/queue clear"})
	updated := result.(*connectedApp)

	if len(updated.promptQueue) != 0 {
		t.Errorf("expected empty queue after /queue clear, got %d", len(updated.promptQueue))
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

	if len(updated.promptQueue) != 0 {
		t.Errorf("expected /btw to bypass queue, but got %d queued", len(updated.promptQueue))
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
		app:         app,
		client:      client,
		promptQueue: []string{"queued"},
	}
	ca.app.status.SetQueueCount(1)

	// Status update for a different session should not drain.
	result, _ := ca.Update(SessionStatusMsg{
		SessionID: "ses_other",
		Status:    SessionStatus{Working: false},
	})
	updated := result.(*connectedApp)

	if len(updated.promptQueue) != 1 {
		t.Errorf("expected queue unchanged for different session, got %d", len(updated.promptQueue))
	}
}
