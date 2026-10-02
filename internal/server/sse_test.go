package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestNewSSEWriter_SetsHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	sse, ok := NewSSEWriter(w)
	if !ok {
		t.Fatal("expected ok=true with httptest.ResponseRecorder")
	}
	if sse == nil {
		t.Fatal("expected non-nil SSEWriter")
	}

	expected := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	}
	for key, want := range expected {
		got := w.Header().Get(key)
		if got != want {
			t.Errorf("header %q = %q, want %q", key, got, want)
		}
	}
}

func TestSSEWriter_SendFullEvent(t *testing.T) {
	w := httptest.NewRecorder()
	sse, _ := NewSSEWriter(w)

	err := sse.Send(SSEEvent{
		ID:    "evt-1",
		Event: "message",
		Data:  "hello world",
	})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "id: evt-1\n") {
		t.Errorf("missing id line in: %q", body)
	}
	if !strings.Contains(body, "event: message\n") {
		t.Errorf("missing event line in: %q", body)
	}
	if !strings.Contains(body, "data: hello world\n\n") {
		t.Errorf("missing data line in: %q", body)
	}
}

func TestSSEWriter_SendStructData(t *testing.T) {
	w := httptest.NewRecorder()
	sse, _ := NewSSEWriter(w)

	err := sse.Send(SSEEvent{
		Event: "update",
		Data:  map[string]int{"count": 42},
	})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, `data: {"count":42}`) {
		t.Errorf("expected JSON-marshaled data in: %q", body)
	}
}

func TestSSEWriter_SendNilData(t *testing.T) {
	w := httptest.NewRecorder()
	sse, _ := NewSSEWriter(w)

	err := sse.Send(SSEEvent{Event: "ping", Data: nil})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: ping\n") {
		t.Errorf("missing event line in: %q", body)
	}
	if !strings.Contains(body, "data: \n\n") {
		t.Errorf("expected empty data line in: %q", body)
	}
}

func TestSSEWriter_SendDataOnly(t *testing.T) {
	w := httptest.NewRecorder()
	sse, _ := NewSSEWriter(w)

	err := sse.Send(SSEEvent{Data: "just data"})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	body := w.Body.String()
	if strings.Contains(body, "id:") {
		t.Errorf("should not have id line in: %q", body)
	}
	if strings.Contains(body, "event:") {
		t.Errorf("should not have event line in: %q", body)
	}
	if !strings.Contains(body, "data: just data\n\n") {
		t.Errorf("expected data line in: %q", body)
	}
}

func TestSSEWriter_Heartbeat(t *testing.T) {
	w := httptest.NewRecorder()
	sse, _ := NewSSEWriter(w)

	err := sse.Heartbeat()
	if err != nil {
		t.Fatalf("Heartbeat error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, ": heartbeat\n\n") {
		t.Errorf("expected heartbeat comment in: %q", body)
	}
}

func TestSSEEvent_Fields(t *testing.T) {
	evt := SSEEvent{
		Event: "test.event",
		Data:  "payload",
		ID:    "id-123",
	}
	if evt.Event != "test.event" {
		t.Errorf("Event = %q, want test.event", evt.Event)
	}
	if evt.ID != "id-123" {
		t.Errorf("ID = %q, want id-123", evt.ID)
	}
}

func TestStreamEvents_FiltersSubagentEvents(t *testing.T) {
	eventBus := bus.New()
	defer eventBus.Close()

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		StreamEvents(ctx, w, eventBus, "")
		close(done)
	}()

	// Allow the goroutine to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)

	// Publish a normal event (should be forwarded).
	eventBus.Publish("session.update", map[string]any{
		"sessionID": "sess-abc",
		"text":      "hello",
	})

	// Publish a subagent event (should be filtered out).
	eventBus.Publish("session.update", map[string]any{
		"sessionID": "sess-abc:executor-A",
		"text":      "subagent msg",
	})

	// Publish another normal event to confirm stream continues.
	eventBus.Publish("session.update", map[string]any{
		"sessionID": "sess-def",
		"text":      "world",
	})

	// Give time for events to be processed.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()

	if !strings.Contains(body, "sess-abc") || !strings.Contains(body, "hello") {
		t.Errorf("expected normal event with sess-abc, got: %s", body)
	}
	if strings.Contains(body, "executor-A") || strings.Contains(body, "subagent msg") {
		t.Errorf("subagent event should be filtered out, got: %s", body)
	}
	if !strings.Contains(body, "sess-def") || !strings.Contains(body, "world") {
		t.Errorf("expected normal event with sess-def, got: %s", body)
	}
}
