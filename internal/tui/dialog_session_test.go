package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSessionDialog_InitiallyHidden(t *testing.T) {
	d := NewSessionDialog()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden initially")
	}
}

func TestSessionDialog_ShowMakesVisible(t *testing.T) {
	d := NewSessionDialog()
	sessions := []SessionInfo{
		{ID: "s1", Title: "First session"},
	}
	d.Show(sessions, false)
	if !d.IsVisible() {
		t.Fatal("expected dialog to be visible after Show")
	}
	if d.selected != 0 {
		t.Errorf("selected should start at 0, got %d", d.selected)
	}
}

func TestSessionDialog_HideMakesInvisible(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1", Title: "Test"}}, false)
	d.Hide()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden after Hide")
	}
}

func TestSessionDialog_ShowResetsSelection(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}, {ID: "s2"}}, false)
	// Move selection down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 1 {
		t.Fatalf("expected selected=1 after j, got %d", d.selected)
	}
	// Re-show should reset
	d.Show([]SessionInfo{{ID: "s3"}}, false)
	if d.selected != 0 {
		t.Errorf("expected selected=0 after re-Show, got %d", d.selected)
	}
}

func TestSessionDialog_NavigateDown(t *testing.T) {
	d := NewSessionDialog()
	// "New Session" at index 0, then two sessions at 1 and 2
	d.Show([]SessionInfo{{ID: "s1"}, {ID: "s2"}}, false)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 1 {
		t.Errorf("expected selected=1 after j, got %d", d.selected)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 2 {
		t.Errorf("expected selected=2 after jj, got %d", d.selected)
	}
}

func TestSessionDialog_NavigateDownClampsAtBottom(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}}, false)
	// itemCount = 2 (New Session + 1 session), max index = 1
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 1 {
		t.Errorf("expected selected to clamp at 1, got %d", d.selected)
	}
}

func TestSessionDialog_NavigateUp(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}, {ID: "s2"}}, false)
	// Move down then up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.selected != 0 {
		t.Errorf("expected selected=0 after k, got %d", d.selected)
	}
}

func TestSessionDialog_NavigateUpClampsAtTop(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}}, false)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.selected != 0 {
		t.Errorf("expected selected to stay at 0, got %d", d.selected)
	}
}

func TestSessionDialog_EnterOnNewSessionReturnsEmptyID(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}}, false)
	// selected=0 is "New Session"
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.IsVisible() {
		t.Fatal("expected dialog to close after enter on New Session")
	}
	if cmd == nil {
		t.Fatal("expected command from enter")
	}
	msg := cmd()
	switched, ok := msg.(SessionSwitchedMsg)
	if !ok {
		t.Fatalf("expected SessionSwitchedMsg, got %T", msg)
	}
	if switched.SessionID != "" {
		t.Errorf("expected empty SessionID for new session, got %q", switched.SessionID)
	}
}

func TestSessionDialog_EnterOnExistingSessionReturnsID(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1", Title: "First"}, {ID: "s2", Title: "Second"}}, false)
	// Move to first session (index 1, after "New Session")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.IsVisible() {
		t.Fatal("expected dialog to close after enter")
	}
	if cmd == nil {
		t.Fatal("expected command from enter")
	}
	msg := cmd()
	switched, ok := msg.(SessionSwitchedMsg)
	if !ok {
		t.Fatalf("expected SessionSwitchedMsg, got %T", msg)
	}
	if switched.SessionID != "s1" {
		t.Errorf("expected SessionID=%q, got %q", "s1", switched.SessionID)
	}
}

func TestSessionDialog_EscapeCloses(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}}, false)
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on esc")
	}
	if cmd != nil {
		t.Error("escape should not emit a command")
	}
}

func TestSessionDialog_QCloses(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}}, false)
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on q")
	}
	if cmd != nil {
		t.Error("q should not emit a command")
	}
}

func TestSessionDialog_InvisibleIgnoresKeys(t *testing.T) {
	d := NewSessionDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invisible dialog should not emit a command")
	}
}

func TestSessionDialog_IgnoresNonKeyMessages(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}}, false)
	d, cmd := d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Error("non-key message should not produce a command")
	}
	if !d.IsVisible() {
		t.Error("non-key message should not close dialog")
	}
}

func TestTruncate_ShortStringUnchanged(t *testing.T) {
	result := truncate("hello", 10)
	if result != "hello" {
		t.Errorf("expected %q, got %q", "hello", result)
	}
}

func TestTruncate_ExactLengthUnchanged(t *testing.T) {
	result := truncate("hello", 5)
	if result != "hello" {
		t.Errorf("expected %q, got %q", "hello", result)
	}
}

func TestTruncate_LongStringTruncatedWithEllipsis(t *testing.T) {
	result := truncate("hello world", 8)
	if result != "hello..." {
		t.Errorf("expected %q, got %q", "hello...", result)
	}
}

func TestTruncate_MinLenFloorAtFour(t *testing.T) {
	result := truncate("hello world", 2)
	// maxLen clamped to 4, so "h..."
	if result != "h..." {
		t.Errorf("expected %q, got %q", "h...", result)
	}
}

func TestRelativeTime_ZeroReturnsEmpty(t *testing.T) {
	result := relativeTime(0)
	if result != "" {
		t.Errorf("expected empty string for 0, got %q", result)
	}
}

func TestRelativeTime_JustNow(t *testing.T) {
	now := time.Now().UnixMilli()
	result := relativeTime(now)
	if result != "just now" {
		t.Errorf("expected %q, got %q", "just now", result)
	}
}

func TestRelativeTime_MinutesAgo(t *testing.T) {
	fiveMinAgo := time.Now().Add(-5 * time.Minute).UnixMilli()
	result := relativeTime(fiveMinAgo)
	if result != "5m ago" {
		t.Errorf("expected %q, got %q", "5m ago", result)
	}
}

func TestRelativeTime_OneMinuteAgo(t *testing.T) {
	oneMinAgo := time.Now().Add(-1 * time.Minute).UnixMilli()
	result := relativeTime(oneMinAgo)
	if result != "1m ago" {
		t.Errorf("expected %q, got %q", "1m ago", result)
	}
}

func TestRelativeTime_HoursAgo(t *testing.T) {
	threeHrsAgo := time.Now().Add(-3 * time.Hour).UnixMilli()
	result := relativeTime(threeHrsAgo)
	if result != "3h ago" {
		t.Errorf("expected %q, got %q", "3h ago", result)
	}
}

func TestRelativeTime_OneHourAgo(t *testing.T) {
	oneHrAgo := time.Now().Add(-1 * time.Hour).UnixMilli()
	result := relativeTime(oneHrAgo)
	if result != "1h ago" {
		t.Errorf("expected %q, got %q", "1h ago", result)
	}
}

func TestRelativeTime_DaysAgo(t *testing.T) {
	twoDaysAgo := time.Now().Add(-48 * time.Hour).UnixMilli()
	result := relativeTime(twoDaysAgo)
	if result != "2d ago" {
		t.Errorf("expected %q, got %q", "2d ago", result)
	}
}

func TestRelativeTime_OneDayAgo(t *testing.T) {
	oneDayAgo := time.Now().Add(-25 * time.Hour).UnixMilli()
	result := relativeTime(oneDayAgo)
	if result != "1d ago" {
		t.Errorf("expected %q, got %q", "1d ago", result)
	}
}

func TestSessionDialog_ViewEmptyWhenHidden(t *testing.T) {
	d := NewSessionDialog()
	if d.View() != "" {
		t.Error("expected empty view when hidden")
	}
}

func TestSessionDialog_ViewNonEmptyWhenVisible(t *testing.T) {
	d := NewSessionDialog()
	d.SetSize(80, 24)
	d.Show([]SessionInfo{{ID: "s1", Title: "Test Session"}}, false)
	view := d.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestSessionDialog_ArrowKeys(t *testing.T) {
	d := NewSessionDialog()
	d.Show([]SessionInfo{{ID: "s1"}, {ID: "s2"}}, false)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != 1 {
		t.Errorf("expected selected=1 after down, got %d", d.selected)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.selected != 0 {
		t.Errorf("expected selected=0 after up, got %d", d.selected)
	}
}

func TestSessionDialog_LoadMore_EmitsMessageAtBottom(t *testing.T) {
	d := NewSessionDialog()
	d.SetSize(80, 24)
	d.Show([]SessionInfo{{ID: "s1"}}, true) // hasMore=true
	// Move to last session (index 1)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.selected != 1 {
		t.Fatalf("expected selected=1, got %d", d.selected)
	}
	// Press down at the bottom — should emit SessionsLoadMoreMsg
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Fatal("expected load-more command at bottom")
	}
	msg := cmd()
	if _, ok := msg.(SessionsLoadMoreMsg); !ok {
		t.Fatalf("expected SessionsLoadMoreMsg, got %T", msg)
	}
	if !d.loading {
		t.Error("expected loading=true after load-more request")
	}
}

func TestSessionDialog_LoadMore_NoMessageWhenNotHasMore(t *testing.T) {
	d := NewSessionDialog()
	d.SetSize(80, 24)
	d.Show([]SessionInfo{{ID: "s1"}}, false) // hasMore=false
	// Move to last session
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// Press down at bottom — should NOT emit
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("expected no command when hasMore=false")
	}
	if d.selected != 1 {
		t.Errorf("expected selected to clamp at 1, got %d", d.selected)
	}
}

func TestSessionDialog_LoadMore_NoDoubleRequest(t *testing.T) {
	d := NewSessionDialog()
	d.SetSize(80, 24)
	d.Show([]SessionInfo{{ID: "s1"}}, true)
	// Move to bottom and trigger load
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // triggers load
	// Try again while loading — should not emit
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("expected no command while already loading")
	}
}

func TestSessionDialog_AppendSessions(t *testing.T) {
	d := NewSessionDialog()
	d.SetSize(80, 24)
	d.Show([]SessionInfo{{ID: "s1"}}, true)
	d.loading = true
	d.AppendSessions([]SessionInfo{{ID: "s2"}, {ID: "s3"}}, false)
	if len(d.sessions) != 3 {
		t.Fatalf("expected 3 sessions after append, got %d", len(d.sessions))
	}
	if d.loading {
		t.Error("expected loading=false after append")
	}
	if d.hasMore {
		t.Error("expected hasMore=false after final page")
	}
}

func TestSessionDialog_ScrollAdjustsViewport(t *testing.T) {
	d := NewSessionDialog()
	// height=10 -> maxVisible = 10-6 = 4
	d.SetSize(80, 10)
	sessions := make([]SessionInfo, 8)
	for i := range sessions {
		sessions[i] = SessionInfo{ID: fmt.Sprintf("s%d", i+1), Title: fmt.Sprintf("Session %d", i+1)}
	}
	d.Show(sessions, false)

	// Navigate down through all sessions
	for i := 0; i < 6; i++ {
		d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	// selected=6 -> sessions[5], maxVisible=4
	// scrollTop should be >= 2 so sessions[5] is visible
	if d.scrollTop < 2 {
		t.Errorf("expected scrollTop >= 2, got %d", d.scrollTop)
	}

	// Navigate back up
	for i := 0; i < 6; i++ {
		d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	}
	// selected=0 (New Session) -> scrollTop should be 0
	if d.scrollTop != 0 {
		t.Errorf("expected scrollTop=0 after scrolling up, got %d", d.scrollTop)
	}
}
