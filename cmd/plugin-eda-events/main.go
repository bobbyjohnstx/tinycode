package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	EdaEndpoint       string
	Events            []string
	SensitivePatterns []string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["edaEndpoint"].(string); ok {
		opts.EdaEndpoint = v
	}
	if v, ok := raw["events"].([]any); ok {
		for _, e := range v {
			if s, ok := e.(string); ok {
				opts.Events = append(opts.Events, s)
			}
		}
	}
	if v, ok := raw["sensitivePatterns"].([]any); ok {
		for _, p := range v {
			if s, ok := p.(string); ok {
				opts.SensitivePatterns = append(opts.SensitivePatterns, s)
			}
		}
	}
	return opts
}

var defaultSensitivePatterns = []string{
	`^sha256~`,
	`^sk-`,
	`^ghp_`,
	`^ghs_`,
	`^glpat-`,
	`^Bearer\s+`,
	`^token-`,
	`^eyJ[A-Za-z0-9_-]{10,}`,
}

type edaEvent struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	SessionID string         `json:"sessionId"`
	Data      map[string]any `json:"data"`
}

type deliveryStats struct {
	mu        sync.Mutex
	Sent      int    `json:"sent"`
	Failed    int    `json:"failed"`
	LastError string `json:"lastError"`
}

func sanitize(data map[string]any, patterns []*regexp.Regexp) map[string]any {
	if len(patterns) == 0 {
		return data
	}
	result := make(map[string]any, len(data))
	for k, v := range data {
		switch val := v.(type) {
		case string:
			redacted := false
			for _, p := range patterns {
				if p.MatchString(val) {
					result[k] = "[REDACTED]"
					redacted = true
					break
				}
			}
			if !redacted {
				result[k] = v
			}
		case map[string]any:
			result[k] = sanitize(val, patterns)
		default:
			result[k] = v
		}
	}
	return result
}

type eventSender struct {
	endpoint      string
	patterns      []*regexp.Regexp
	allowedEvents map[string]bool
	stats         deliveryStats
	httpClient    *http.Client
}

func newEventSender(endpoint string, patterns []*regexp.Regexp, allowedEvents []string) *eventSender {
	allowed := map[string]bool{}
	for _, e := range allowedEvents {
		allowed[e] = true
	}
	return &eventSender{
		endpoint:      endpoint,
		patterns:      patterns,
		allowedEvents: allowed,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *eventSender) fire(event edaEvent) {
	if len(s.allowedEvents) > 0 && !s.allowedEvents[event.Type] {
		return
	}

	sanitized := edaEvent{
		Type:      event.Type,
		Timestamp: event.Timestamp,
		SessionID: event.SessionID,
		Data:      sanitize(event.Data, s.patterns),
	}

	body, err := json.Marshal(sanitized)
	if err != nil {
		s.stats.mu.Lock()
		s.stats.Failed++
		s.stats.LastError = err.Error()
		s.stats.mu.Unlock()
		return
	}

	go func() {
		resp, err := s.httpClient.Post(s.endpoint, "application/json", bytes.NewReader(body))
		s.stats.mu.Lock()
		defer s.stats.mu.Unlock()
		if err != nil {
			s.stats.Failed++
			s.stats.LastError = err.Error()
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.stats.Sent++
		} else {
			s.stats.Failed++
			s.stats.LastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}()
}

func newPlugin(opts options) plugin.Plugin {
	if opts.EdaEndpoint == "" {
		return plugin.Plugin{ID: "eda-events"}
	}

	rawPatterns := opts.SensitivePatterns
	if len(rawPatterns) == 0 {
		rawPatterns = defaultSensitivePatterns
	}
	var patterns []*regexp.Regexp
	for _, p := range rawPatterns {
		if re, err := regexp.Compile(p); err == nil {
			patterns = append(patterns, re)
		}
	}

	sender := newEventSender(opts.EdaEndpoint, patterns, opts.Events)

	var sessionID string
	var startTime time.Time

	return plugin.Plugin{
		ID: "eda-events",
		Hooks: plugin.HookHandlers{
			SessionStart: func(_ context.Context, event plugin.SessionStartEvent) error {
				sessionID = event.SessionID
				startTime = time.Now()

				sender.fire(edaEvent{
					Type:      "tinycode.session.started",
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					SessionID: sessionID,
					Data: map[string]any{
						"sessionId":        sessionID,
						"projectDirectory": event.Directory,
					},
				})
				return nil
			},
			SessionEnd: func(_ context.Context, event plugin.SessionEndEvent) error {
				duration := time.Since(startTime).Milliseconds()

				sender.stats.mu.Lock()
				stats := map[string]any{
					"sent":      sender.stats.Sent,
					"failed":    sender.stats.Failed,
					"lastError": sender.stats.LastError,
				}
				sender.stats.mu.Unlock()

				sender.fire(edaEvent{
					Type:      "tinycode.session.ended",
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					SessionID: event.SessionID,
					Data: map[string]any{
						"sessionId": event.SessionID,
						"duration":  duration,
						"delivery":  stats,
					},
				})
				return nil
			},
			ToolExecAfter: func(_ context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
				toolName := input.ToolName
				argsStr := input.Output

				var eventType string
				switch {
				case toolName == "shell" && (strings.Contains(argsStr, "docker build") || strings.Contains(argsStr, "podman build")):
					eventType = "tinycode.image.built"
				case toolName == "edit" && strings.Contains(argsStr, "Dockerfile"):
					eventType = "tinycode.dockerfile.changed"
				case toolName == "edit" && strings.Contains(argsStr, "k8s/") && (strings.HasSuffix(argsStr, ".yml") || strings.HasSuffix(argsStr, ".yaml")):
					eventType = "tinycode.manifest.changed"
				case toolName == "shell" && strings.Contains(argsStr, "git push"):
					eventType = "tinycode.code.pushed"
				}

				if eventType == "" {
					return nil, nil
				}

				slog.Info("eda-events: firing event", "type", eventType, "tool", toolName)
				sender.fire(edaEvent{
					Type:      eventType,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					SessionID: sessionID,
					Data: map[string]any{
						"tool": toolName,
					},
				})
				return nil, nil
			},
			Dispose: func(_ context.Context) error {
				sessionID = ""
				startTime = time.Time{}
				return nil
			},
		},
	}
}

func main() {
	opts := options{}
	plugin.Run(newPlugin(opts))
}
