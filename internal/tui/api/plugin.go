package api

// PluginEventInput is the request body for POST /plugin/event.
type PluginEventInput struct {
	Event     string `json:"event"`
	SessionID string `json:"sessionID"`
	Data      any    `json:"data,omitempty"`
}

// SendPluginEvent fires a plugin lifecycle event to the server.
func (c *Client) SendPluginEvent(event, sessionID string, data any) error {
	body := PluginEventInput{
		Event:     event,
		SessionID: sessionID,
		Data:      data,
	}
	return c.postNoResp("/plugin/event", body)
}
