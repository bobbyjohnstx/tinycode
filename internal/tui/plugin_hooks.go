package tui

import (
	"fmt"
	"log"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

// PluginHooks fires lifecycle events to the server via the API client.
// Events are fire-and-forget: errors are logged but do not block the TUI.
type PluginHooks struct {
	client *api.Client
}

// NewPluginHooks creates a PluginHooks that sends events through the given client.
// If client is nil, all hook calls are no-ops.
func NewPluginHooks(client *api.Client) *PluginHooks {
	return &PluginHooks{client: client}
}

// OnSessionStart fires when a new session is created.
func (p *PluginHooks) OnSessionStart(sessionID string) {
	p.fire("session.start", sessionID, nil)
}

// OnSessionEnd fires when a session is deleted.
func (p *PluginHooks) OnSessionEnd(sessionID string) {
	p.fire("session.end", sessionID, nil)
}

// OnSessionSwitch fires when the user switches to a different session.
func (p *PluginHooks) OnSessionSwitch(sessionID string) {
	p.fire("session.switch", sessionID, nil)
}

// OnModelChange fires when the model is changed for a session.
func (p *PluginHooks) OnModelChange(sessionID, modelID string) {
	p.fire("session.model.change", sessionID, map[string]string{"modelID": modelID})
}

// fire sends a plugin event to the server. Errors are logged, not returned.
func (p *PluginHooks) fire(event, sessionID string, data any) {
	if p.client == nil {
		return
	}
	if err := p.client.SendPluginEvent(event, sessionID, data); err != nil {
		log.Printf("plugin hook %s failed: %v", event, err)
	}
}

// PluginLoadedMsg signals that a plugin was loaded by the server.
type PluginLoadedMsg struct {
	Name string
}

// PluginEventMsg carries a plugin event from the server.
type PluginEventMsg struct {
	Name string
	Data any
}

// String returns a human-readable representation of PluginLoadedMsg.
func (m PluginLoadedMsg) String() string {
	return fmt.Sprintf("plugin loaded: %s", m.Name)
}
