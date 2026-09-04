package server

import (
	"context"
	"net/http"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
)

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	StreamGlobalEvents(r.Context(), w, s.deps.Bus, s.config.Directory)
}

func (s *Server) handleGlobalEventStreamWrapped(w http.ResponseWriter, r *http.Request) {
	StreamGlobalEvents(r.Context(), w, s.deps.Bus, s.config.Directory)
}

func (s *Server) handleSessionEventStream(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		respondError(w, http.StatusBadRequest, "session ID required")
		return
	}
	StreamEvents(r.Context(), w, s.deps.Bus, sessionID)
}

func StreamGlobalEvents(ctx context.Context, w http.ResponseWriter, eventBus *bus.Bus, directory string) {
	sse, ok := NewSSEWriter(w)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sse.Send(SSEEvent{
		Data: map[string]any{
			"directory": directory,
			"payload": map[string]any{
				"type":       "server.connected",
				"properties": map[string]any{"timestamp": time.Now().UnixMilli()},
			},
		},
	})

	sub := eventBus.SubscribeAll()
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
					Data: map[string]any{
						"directory": directory,
						"payload": map[string]any{
							"id":         evt.ID,
							"type":       "global.disposed",
							"properties": map[string]any{"timestamp": time.Now().UnixMilli()},
						},
					},
				})
				return
			}
			if err := sse.Send(SSEEvent{
				Data: map[string]any{
					"directory": directory,
					"payload": map[string]any{
						"id":         evt.ID,
						"type":       evt.Type,
						"properties": evt.Properties,
					},
				},
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
