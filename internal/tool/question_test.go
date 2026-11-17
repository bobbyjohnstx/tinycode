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

func TestExecuteQuestion_NilBusFailClosed(t *testing.T) {
	tc := &Context{SessionID: "ses-q-nil", Bus: nil}
	result, err := executeQuestion(context.Background(), tc, json.RawMessage(`{"question":"Continue?"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %+v, want IsError when Bus is nil", result)
	}
	if result.Output == "" || result.Output[0] == '[' {
		t.Fatalf("output = %q, want fail-closed message (not fake success)", result.Output)
	}
}

func TestExecuteQuestion_LegacyShapeStillWorks(t *testing.T) {
	b := bus.New()
	defer b.Close()

	asked := b.Subscribe("question.asked")
	defer asked.Unsubscribe()

	tc := &Context{SessionID: "ses-q-legacy", Bus: b}
	done := make(chan struct{})
	var result *ExecuteResult
	go func() {
		result, _ = executeQuestion(context.Background(), tc,
			json.RawMessage(`{"question":"Pick a color","options":["red","blue"]}`))
		close(done)
	}()

	var questionID string
	select {
	case evt := <-asked.C:
		props := evt.Properties.(map[string]any)
		questionID, _ = props["id"].(string)
		questions, _ := props["questions"].([]map[string]any)
		if len(questions) != 1 {
			t.Fatalf("expected 1 question, got %d", len(questions))
		}
		if questions[0]["question"] != "Pick a color" {
			t.Errorf("question = %v", questions[0]["question"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question.asked")
	}

	b.Publish("question.replied", map[string]any{
		"requestID": questionID,
		"answer":    "red",
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "red" {
		t.Errorf("answer = %q, want red", result.Output)
	}
}

func TestExecuteQuestion_StructuredThreeQuestions(t *testing.T) {
	b := bus.New()
	defer b.Close()

	asked := b.Subscribe("question.asked")
	defer asked.Unsubscribe()

	tc := &Context{SessionID: "ses-q-struct", Bus: b}
	done := make(chan struct{})
	var result *ExecuteResult
	go func() {
		result, _ = executeQuestion(context.Background(), tc, json.RawMessage(`{
			"questions": [
				{"id":"q1","header":"Style","question":"Pick a style","options":[{"label":"A","description":"Option A"},{"label":"B"}]},
				{"id":"q2","header":"Color","question":"Pick a color","options":[{"label":"Red"},{"label":"Blue"}]},
				{"id":"q3","header":"Size","question":"Pick size","options":[{"label":"S"},{"label":"L"}]}
			]
		}`))
		close(done)
	}()

	var questionID string
	select {
	case evt := <-asked.C:
		props := evt.Properties.(map[string]any)
		questionID, _ = props["id"].(string)
		questions, _ := props["questions"].([]map[string]any)
		if len(questions) != 3 {
			t.Fatalf("expected 3 questions, got %d", len(questions))
		}
		if questions[0]["id"] != "q1" {
			t.Errorf("first question id = %v", questions[0]["id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	b.Publish("question.replied", map[string]any{
		"requestID": questionID,
		"answer":    `{"q1":"A","q2":"Red","q3":"S"}`,
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != `{"q1":"A","q2":"Red","q3":"S"}` {
		t.Errorf("answer = %q", result.Output)
	}
}

func TestExecuteQuestion_FourthQuestionRejected(t *testing.T) {
	b := bus.New()
	defer b.Close()

	tc := &Context{SessionID: "ses-q-4", Bus: b}
	result, err := executeQuestion(context.Background(), tc, json.RawMessage(`{
		"questions": [
			{"id":"q1","question":"Q1","options":[{"label":"A"}]},
			{"id":"q2","question":"Q2","options":[{"label":"A"}]},
			{"id":"q3","question":"Q3","options":[{"label":"A"}]},
			{"id":"q4","question":"Q4","options":[{"label":"A"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for 4 questions")
	}
	if result.Output != "Too many questions: 4 (max 3)" {
		t.Errorf("output = %q", result.Output)
	}
}

func TestExecuteQuestion_HeadlessRejects(t *testing.T) {
	// Bus is nil = headless mode.
	tc := &Context{SessionID: "ses-q-headless", Bus: nil}
	result, err := executeQuestion(context.Background(), tc, json.RawMessage(`{
		"questions": [
			{"id":"q1","question":"Q1","options":[{"label":"A"},{"label":"B"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected rejection in headless mode")
	}
}
