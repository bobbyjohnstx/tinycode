package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseSSEData_ValidEnvelope(t *testing.T) {
	data := `{"directory":"/tmp","payload":{"id":"evt_1","type":"message","properties":{"text":"hello"}}}`
	evt, err := parseSSEData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.ID != "evt_1" {
		t.Errorf("ID = %q, want %q", evt.ID, "evt_1")
	}
	if evt.Type != "message" {
		t.Errorf("Type = %q, want %q", evt.Type, "message")
	}
	if evt.Properties["text"] != "hello" {
		t.Errorf("Properties[text] = %v, want %q", evt.Properties["text"], "hello")
	}
}

func TestParseSSEData_MissingPayload(t *testing.T) {
	data := `{"directory":"/tmp"}`
	evt, err := parseSSEData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.ID != "" {
		t.Errorf("ID should be empty, got %q", evt.ID)
	}
	if evt.Type != "" {
		t.Errorf("Type should be empty, got %q", evt.Type)
	}
}

func TestParseSSEData_InvalidJSON(t *testing.T) {
	_, err := parseSSEData("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseSSEData_EmptyProperties(t *testing.T) {
	data := `{"directory":"/tmp","payload":{"id":"e1","type":"status","properties":{}}}`
	evt, err := parseSSEData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evt.Properties) != 0 {
		t.Errorf("Properties should be empty, got %v", evt.Properties)
	}
}

func TestSubscribe_ReceivesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement Flusher")
		}

		events := []string{
			`data: {"directory":"/tmp","payload":{"id":"1","type":"msg","properties":{"n":1}}}`,
			`data: {"directory":"/tmp","payload":{"id":"2","type":"done","properties":{}}}`,
		}
		for _, e := range events {
			fmt.Fprintln(w, e)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	var received []ServerEvent
	for evt := range ch {
		received = append(received, evt)
		if len(received) >= 2 {
			cancel()
		}
	}

	if len(received) < 2 {
		t.Fatalf("received %d events, want at least 2", len(received))
	}
	if received[0].ID != "1" || received[0].Type != "msg" {
		t.Errorf("first event = %+v, want ID=1, Type=msg", received[0])
	}
	if received[1].ID != "2" || received[1].Type != "done" {
		t.Errorf("second event = %+v, want ID=2, Type=done", received[1])
	}
}

func TestSubscribe_SkipsHeartbeatsAndComments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		lines := []string{
			": heartbeat 12345",
			": this is a comment",
			"",
			"event: something",
			`data: {"directory":"/tmp","payload":{"id":"real","type":"data","properties":{}}}`,
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	var received []ServerEvent
	for evt := range ch {
		received = append(received, evt)
		cancel()
	}

	if len(received) != 1 {
		t.Fatalf("received %d events, want 1 (only data lines)", len(received))
	}
	if received[0].ID != "real" {
		t.Errorf("event ID = %q, want %q", received[0].ID, "real")
	}
}

func TestSubscribe_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// Keep sending heartbeats until context is cancelled
		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
				fmt.Fprintln(w, ": heartbeat")
				flusher.Flush()
				time.Sleep(10 * time.Millisecond)
			}
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "/tmp")
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	cancel()

	// Channel should close after context cancellation
	select {
	case _, ok := <-ch:
		if ok {
			// Got an event, that's fine, keep draining
		}
	case <-time.After(2 * time.Second):
		t.Error("channel did not close after context cancellation")
	}
}
