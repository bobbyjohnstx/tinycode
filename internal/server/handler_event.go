package server

import (
	"context"
	"net/http"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
)

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	StreamEvents(r.Context(), w, s.deps.Bus, "")
}

func (s *Server) handleSessionEventStream(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		respondError(w, http.StatusBadRequest, "session ID required")
		return
	}
	StreamEvents(r.Context(), w, s.deps.Bus, sessionID)
}

// DirectoryResolver maps a session ID to its project directory.
type DirectoryResolver func(sessionID string) string

func StreamGlobalEvents(ctx context.Context, w http.ResponseWriter, eventBus *bus.Bus, defaultDir string, resolve DirectoryResolver) {
	sse, ok := NewSSEWriter(w)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sse.Send(SSEEvent{
		Data: map[string]any{
			"directory": defaultDir,
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
						"directory": defaultDir,
						"payload": map[string]any{
							"id":         evt.ID,
							"type":       "global.disposed",
							"properties": map[string]any{"timestamp": time.Now().UnixMilli()},
						},
					},
				})
				return
			}
			dir := defaultDir
			if evt.Type == "project.updated" {
				dir = "global"
			} else if resolve != nil {
				if props, ok := evt.Properties.(map[string]any); ok {
					if sid, ok := props["sessionID"].(string); ok && sid != "" {
						if d := resolve(sid); d != "" {
							dir = d
						}
					}
				}
			}
			if err := sse.Send(SSEEvent{
				Data: map[string]any{
					"directory": dir,
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
