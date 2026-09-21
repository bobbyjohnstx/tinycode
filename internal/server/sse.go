package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
)

const (
	sseHeartbeatInterval = 10 * time.Second
)

type SSEEvent struct {
	Event string `json:"event,omitempty"`
	Data  any    `json:"data,omitempty"`
	ID    string `json:"id,omitempty"`
}

type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &SSEWriter{w: w, flusher: flusher}, true
}

func (s *SSEWriter) Send(event SSEEvent) error {
	if event.ID != "" {
		if _, err := fmt.Fprintf(s.w, "id: %s\n", event.ID); err != nil {
			return err
		}
	}

	if event.Event != "" {
		if _, err := fmt.Fprintf(s.w, "event: %s\n", event.Event); err != nil {
			return err
		}
	}

	var dataStr string
	switch v := event.Data.(type) {
	case string:
		dataStr = v
	case nil:
		dataStr = ""
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		dataStr = string(b)
	}

	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", dataStr); err != nil {
		return err
	}

	s.flusher.Flush()
	return nil
}

func (s *SSEWriter) Heartbeat() error {
	if _, err := fmt.Fprint(s.w, ": heartbeat\n\n"); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func StreamEvents(ctx context.Context, w http.ResponseWriter, eventBus *bus.Bus, sessionID string) {
	sse, ok := NewSSEWriter(w)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sse.Send(SSEEvent{
		Event: "server.connected",
		Data:  map[string]any{"timestamp": time.Now().UnixMilli()},
	})

	var sub *bus.Subscription
	if sessionID != "" {
		sub = eventBus.Subscribe(sessionID)
	} else {
		sub = eventBus.SubscribeAll()
	}
	defer sub.Unsubscribe()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub.C:
			if !ok {
				sse.Send(SSEEvent{
					Event: "global.disposed",
					Data:  map[string]any{"timestamp": time.Now().UnixMilli()},
				})
				return
			}
			// Skip subagent events — they're forwarded to parent by the event bridge
			if props, ok := evt.Properties.(map[string]any); ok {
				if sid, ok := props["sessionID"].(string); ok && strings.Contains(sid, ":") {
					continue
				}
			}
			if err := sse.Send(SSEEvent{
				Event: evt.Type,
				Data:  evt.Properties,
				ID:    evt.ID,
			}); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := sse.Heartbeat(); err != nil {
				return
			}
		}
	}
}
