package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ToastExpiredMsg signals that a toast's display time has elapsed.
type ToastExpiredMsg struct {
	ID int
}

// ToastItem is a single toast notification waiting to be shown.
type ToastItem struct {
	Text    string
	IsError bool
	// Duration controls how long this toast is visible.
	// Zero uses the default (3 seconds).
	Duration time.Duration
}

const defaultToastDuration = 3 * time.Second

// Toast manages a queue of toast notifications, showing one at a time.
type Toast struct {
	queue   []ToastItem
	current *ToastItem
	id      int
	width   int
	theme   Theme
}

// NewToast creates a Toast with the given theme.
func NewToast(theme Theme) Toast {
	return Toast{theme: theme}
}

// Show enqueues a toast notification. If nothing is currently showing,
// it begins displaying immediately.
func (t *Toast) Show(text string, isError bool) tea.Cmd {
	item := ToastItem{
		Text:    text,
		IsError: isError,
	}
	if t.current == nil {
		t.current = &item
		t.id++
		return t.tickCmd(t.id)
	}
	t.queue = append(t.queue, item)
	return nil
}

// SetSize updates the available width for rendering.
func (t *Toast) SetSize(width int) {
	t.width = width
}

// Init implements tea.Model.
func (t Toast) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (t Toast) Update(msg tea.Msg) (Toast, tea.Cmd) {
	switch msg := msg.(type) {
	case ToastMsg:
		cmd := t.Show(msg.Text, msg.IsError)
		return t, cmd

	case ToastExpiredMsg:
		if msg.ID != t.id {
			return t, nil
		}
		return t, t.advance()
	}
	return t, nil
}

// View renders the current toast, or empty string if none.
func (t Toast) View() string {
	if t.current == nil {
		return ""
	}
	style := t.theme.ToastInfo
	if t.current.IsError {
		style = t.theme.ToastError
	}
	if t.width > 0 {
		style = style.MaxWidth(t.width)
	}
	return style.Render(t.current.Text)
}

// IsVisible reports whether a toast is currently displayed.
func (t Toast) IsVisible() bool {
	return t.current != nil
}

// Dismiss clears the current toast and advances to the next queued item.
func (t *Toast) Dismiss() tea.Cmd {
	return t.advance()
}

// advance moves to the next queued toast, or clears the display.
func (t *Toast) advance() tea.Cmd {
	if len(t.queue) == 0 {
		t.current = nil
		return nil
	}
	next := t.queue[0]
	t.queue = t.queue[1:]
	t.current = &next
	t.id++
	return t.tickCmd(t.id)
}

// tickCmd returns a command that fires ToastExpiredMsg after the toast duration.
func (t *Toast) tickCmd(id int) tea.Cmd {
	dur := defaultToastDuration
	if t.current != nil && t.current.Duration > 0 {
		dur = t.current.Duration
	}
	return tea.Tick(dur, func(_ time.Time) tea.Msg {
		return ToastExpiredMsg{ID: id}
	})
}

// toastOverlay renders the toast above the status bar line.
func toastOverlay(toastView string, width int) string {
	if toastView == "" {
		return ""
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, toastView)
}
