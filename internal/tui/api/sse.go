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

	go c.sseLoop(ctx, events)

	return events, nil
}

func (c *Client) sseLoop(ctx context.Context, events chan<- ServerEvent) {
	defer close(events)

	c.sseBackoff = initialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.readSSEStream(ctx, events)
		if err == nil || ctx.Err() != nil {
			return
		}

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

func (c *Client) readSSEStream(ctx context.Context, events chan<- ServerEvent) error {
	url := c.baseURL + "/global/event"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
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

		// Parse "data: {...}" lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]

		evt, err := parseSSEData(data)
		if err != nil {
			continue
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
