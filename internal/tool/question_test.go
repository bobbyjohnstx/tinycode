package tool

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestExecuteQuestion_ParentCancelPublishesRejected(t *testing.T) {
	b := bus.New()
	defer b.Close()

	rejected := b.Subscribe("question.rejected")
	defer rejected.Unsubscribe()

	asked := b.Subscribe("question.asked")
	defer asked.Unsubscribe()

	tc := &Context{SessionID: "ses-q-cancel", Bus: b}
	ctx, cancel := context.WithCancel(context.Background())

	var (
		mu      sync.Mutex
		result  *ExecuteResult
		execErr error
	)
	done := make(chan struct{})
	go func() {
		result, execErr = executeQuestion(ctx, tc, json.RawMessage(`{"question":"Continue?"}`))
		close(done)
	}()

	var questionID string
	select {
	case evt := <-asked.C:
		props := evt.Properties.(map[string]any)
		questionID, _ = props["id"].(string)
		if questionID == "" {
			t.Fatal("expected question id in asked event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question.asked")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("executeQuestion did not return after cancel")
	}
	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}
	mu.Lock()
	defer mu.Unlock()
	if result == nil || !result.IsError || result.Output != "Question cancelled" {
		t.Fatalf("result = %+v, want IsError Question cancelled", result)
	}

	select {
	case evt := <-rejected.C:
		props := evt.Properties.(map[string]any)
		if props["requestID"] != questionID {
			t.Errorf("requestID = %v, want %s", props["requestID"], questionID)
		}
		if props["sessionID"] != "ses-q-cancel" {
			t.Errorf("sessionID = %v, want ses-q-cancel", props["sessionID"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question.rejected")
	}
}

func TestExecuteQuestion_RejectedEventUnblocks(t *testing.T) {
	b := bus.New()
	defer b.Close()

	asked := b.Subscribe("question.asked")
	defer asked.Unsubscribe()

	tc := &Context{SessionID: "ses-q-rej", Bus: b}
	done := make(chan struct{})
	var result *ExecuteResult
	var execErr error
	go func() {
		result, execErr = executeQuestion(context.Background(), tc, json.RawMessage(`{"question":"Pick?"}`))
		close(done)
	}()

	var questionID string
	select {
	case evt := <-asked.C:
		props := evt.Properties.(map[string]any)
		questionID, _ = props["id"].(string)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question.asked")
	}

	// asked is published before rejected is subscribed; retry briefly so the
	// wait loop is listening before we deliver the cancel event.
	deadline := time.After(2 * time.Second)
	for {
		b.Publish("question.rejected", map[string]any{
			"sessionID": "ses-q-rej",
			"requestID": questionID,
		})
		select {
		case <-done:
			goto finished
		case <-deadline:
			t.Fatal("executeQuestion did not unblock on question.rejected")
		case <-time.After(20 * time.Millisecond):
		}
	}
finished:
	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if result == nil || !result.IsError || result.Output != "Question cancelled" {
		t.Fatalf("result = %+v, want IsError Question cancelled", result)
	}
}
