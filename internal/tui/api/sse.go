package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

// ServerEvent represents a parsed SSE event from the tinycode server.
type ServerEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

const (
	initialBackoff = 500 * time.Millisecond
	maxBackoff     = 30 * time.Second
	backoffFactor  = 2.0
)

// Subscribe connects to the server's SSE endpoint and returns a channel of parsed events.
// It automatically reconnects with exponential backoff on disconnect.
// The returned channel is closed when the context is canceled.
func (c *Client) Subscribe(ctx context.Context) (<-chan ServerEvent, error) {
	events := make(chan ServerEvent, 256)

	safego.Go(func() { c.sseLoop(ctx, events) })

	return events, nil
}

func (c *Client) sseLoop(ctx context.Context, events chan<- ServerEvent) {
	defer close(events)

	c.sseBackoff = initialBackoff
	var lastID string

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = c.readSSEStream(ctx, events, &lastID)
		if ctx.Err() != nil {
			return
		}

		// Stream ended (clean EOF or error) — reconnect with backoff.
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.sseBackoff):
		}

		c.sseBackoff = time.Duration(math.Min(
			float64(c.sseBackoff)*backoffFactor,
			float64(maxBackoff),
		))
	}
}

func (c *Client) resetBackoff() {
	c.sseBackoff = initialBackoff
}

func (c *Client) readSSEStream(ctx context.Context, events chan<- ServerEvent, lastID *string) error {
	reqURL := c.baseURL + "/global/event"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if lastID != nil && *lastID != "" {
		req.Header.Set("Last-Event-ID", *lastID)
	}

	client := c.sseHTTP
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE connection returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	c.resetBackoff()

	var wireID string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()

		// Heartbeat
		if strings.HasPrefix(line, ": heartbeat") {
			continue
		}

		// Comment lines
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Blank lines delimit events in SSE protocol; skip them
		if line == "" {
			continue
		}

		// SSE id: field (Last-Event-ID resume)
		if strings.HasPrefix(line, "id:") {
			wireID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			continue
		}

		// Parse "data: {...}" lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]

		evt, err := parseSSEData(data)
		if err != nil {
			continue
		}

		if evt.ID == "" && wireID != "" {
			evt.ID = wireID
		}
		if evt.ID != "" && lastID != nil {
			*lastID = evt.ID
		}

		select {
		case events <- evt:
		case <-ctx.Done():
			return nil
		}
	}

	return scanner.Err()
}

// parseSSEData parses the SSE data payload.
// Wire format: {"directory":"...","payload":{"id":"...","type":"...","properties":{...}}}
func parseSSEData(data string) (ServerEvent, error) {
	var envelope struct {
		Directory string `json:"directory"`
		Payload   struct {
			ID         string         `json:"id"`
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
		} `json:"payload"`
	}

	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return ServerEvent{}, fmt.Errorf("parsing SSE data: %w", err)
	}

	return ServerEvent{
		ID:         envelope.Payload.ID,
		Type:       envelope.Payload.Type,
		Properties: envelope.Payload.Properties,
	}, nil
}
