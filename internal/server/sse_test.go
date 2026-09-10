package server

import (
	"net/http/httptest"
	"strings"
	"testing"
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
