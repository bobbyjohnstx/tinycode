package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// LeaderTimeoutMsg signals that the leader key window has expired.
type LeaderTimeoutMsg struct{}

// LeaderActionMsg carries the resolved leader action name.
type LeaderActionMsg struct {
	Action string
}

const leaderTimeout = 500 * time.Millisecond

// Leader actions returned by HandleKey.
const (
	LeaderActionSidebar       = "toggle-sidebar"
	LeaderActionNewSession    = "new-session"
	LeaderActionSessionList   = "session-list"
	LeaderActionModelList     = "model-list"
	LeaderActionAgentList     = "agent-list"
	LeaderActionExportSession = "export-session"
	LeaderActionCopyResponse  = "copy-response"
)

// LeaderState implements the ctrl+x leader key state machine.
// When ctrl+x is pressed, the state becomes pending and a 500ms deadline
// is set. The next keypress within that window is matched against leader
// bindings; unrecognized keys or timeout reset the state.
type LeaderState struct {
	pending  bool
	deadline time.Time
	keys     KeyMap
}

// NewLeaderState creates a LeaderState with the given key bindings.
func NewLeaderState(keys KeyMap) LeaderState {
	return LeaderState{keys: keys}
}

// IsPending reports whether the leader key has been pressed and we are
// waiting for the follow-up key.
func (l LeaderState) IsPending() bool {
	return l.pending
}

// HandleKey processes a key message through the leader state machine.
// It returns (action, consumed). When consumed is true, the caller
// should not process the key further.
func (l *LeaderState) HandleKey(msg tea.KeyMsg) (string, bool) {
	// If pending, try to match the follow-up key.
	if l.pending {
		l.pending = false

		// Check timeout (belt-and-suspenders with LeaderTimeoutMsg).
		if !l.deadline.IsZero() && time.Now().After(l.deadline) {
			return "", true // expired, consume the key silently
		}

		action := l.matchBinding(msg)
		if action != "" {
			return action, true
		}
		// Unrecognized follow-up key: reset, consume so it doesn't
		// leak through as a normal keypress.
		return "", true
	}

	// Not pending: check for leader key press.
	if key.Matches(msg, l.keys.LeaderKey) {
		l.pending = true
		l.deadline = time.Now().Add(leaderTimeout)
		return "", true
	}

	return "", false
}

// TimeoutCmd returns a command that fires LeaderTimeoutMsg after the
// leader timeout window. Call this when entering pending state.
func (l *LeaderState) TimeoutCmd() tea.Cmd {
	return tea.Tick(leaderTimeout, func(_ time.Time) tea.Msg {
		return LeaderTimeoutMsg{}
	})
}

// HandleTimeout resets the pending state if the timeout fires.
func (l *LeaderState) HandleTimeout() {
	if l.pending {
		l.pending = false
	}
}

// matchBinding maps a follow-up key to a leader action.
func (l *LeaderState) matchBinding(msg tea.KeyMsg) string {
	switch {
	case key.Matches(msg, l.keys.ToggleSidebar):
		return LeaderActionSidebar
	case key.Matches(msg, l.keys.NewSession):
		return LeaderActionNewSession
	case key.Matches(msg, l.keys.SessionList):
		return LeaderActionSessionList
	case key.Matches(msg, l.keys.ModelList):
		return LeaderActionModelList
	case key.Matches(msg, l.keys.AgentList):
		return LeaderActionAgentList
	case key.Matches(msg, l.keys.ExportSession):
		return LeaderActionExportSession
	case key.Matches(msg, l.keys.CopyResponse):
		return LeaderActionCopyResponse
	default:
		return ""
	}
}
