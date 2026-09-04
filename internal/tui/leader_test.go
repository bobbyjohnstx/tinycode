package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestLeader() LeaderState {
	return NewLeaderState(DefaultKeyMap())
}

func keyMsg(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func ctrlKeyMsg(k tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: k}
}

func TestLeaderCtrlXSetsPending(t *testing.T) {
	l := newTestLeader()
	if l.IsPending() {
		t.Fatal("expected not pending initially")
	}

	action, consumed := l.HandleKey(ctrlKeyMsg(tea.KeyCtrlX))
	if !consumed {
		t.Fatal("ctrl+x should be consumed")
	}
	if action != "" {
		t.Fatalf("ctrl+x should not produce action, got %q", action)
	}
	if !l.IsPending() {
		t.Fatal("expected pending after ctrl+x")
	}
}

func TestLeaderValidKeyReturnsAction(t *testing.T) {
	tests := []struct {
		key    string
		action string
	}{
		{"b", LeaderActionSidebar},
		{"n", LeaderActionNewSession},
		{"o", LeaderActionSessionList},
		{"m", LeaderActionModelList},
		{"a", LeaderActionAgentList},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			l := newTestLeader()
			l.HandleKey(ctrlKeyMsg(tea.KeyCtrlX))

			action, consumed := l.HandleKey(keyMsg(tt.key))
			if !consumed {
				t.Fatalf("follow-up key %q should be consumed", tt.key)
			}
			if action != tt.action {
				t.Fatalf("expected action %q, got %q", tt.action, action)
			}
			if l.IsPending() {
				t.Fatal("should not be pending after valid follow-up")
			}
		})
	}
}

func TestLeaderInvalidKeyResets(t *testing.T) {
	l := newTestLeader()
	l.HandleKey(ctrlKeyMsg(tea.KeyCtrlX))

	action, consumed := l.HandleKey(keyMsg("z"))
	if !consumed {
		t.Fatal("invalid follow-up should still be consumed")
	}
	if action != "" {
		t.Fatalf("invalid follow-up should not produce action, got %q", action)
	}
	if l.IsPending() {
		t.Fatal("should reset pending after invalid follow-up")
	}
}

func TestLeaderTimeoutResets(t *testing.T) {
	l := newTestLeader()
	l.HandleKey(ctrlKeyMsg(tea.KeyCtrlX))

	l.HandleTimeout()
	if l.IsPending() {
		t.Fatal("should not be pending after timeout")
	}
}

func TestLeaderTimeoutNotPending(t *testing.T) {
	l := newTestLeader()
	// HandleTimeout when not pending should be a no-op.
	l.HandleTimeout()
	if l.IsPending() {
		t.Fatal("should not be pending")
	}
}

func TestLeaderNonLeaderKeyNotConsumed(t *testing.T) {
	l := newTestLeader()
	action, consumed := l.HandleKey(keyMsg("x"))
	if consumed {
		t.Fatal("non-leader key should not be consumed when not pending")
	}
	if action != "" {
		t.Fatalf("unexpected action %q", action)
	}
}

func TestLeaderDeadlineExpiry(t *testing.T) {
	l := newTestLeader()
	l.HandleKey(ctrlKeyMsg(tea.KeyCtrlX))

	// Force the deadline to be in the past.
	l.deadline = time.Now().Add(-1 * time.Second)

	action, consumed := l.HandleKey(keyMsg("b"))
	if !consumed {
		t.Fatal("expired follow-up should be consumed")
	}
	if action != "" {
		t.Fatal("expired follow-up should not produce action")
	}
	if l.IsPending() {
		t.Fatal("should not be pending after expired follow-up")
	}
}

func TestLeaderTimeoutCmdReturnsCmd(t *testing.T) {
	l := newTestLeader()
	cmd := l.TimeoutCmd()
	if cmd == nil {
		t.Fatal("expected non-nil timeout command")
	}
}
