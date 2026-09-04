package tui

import (
	"testing"
	"time"
)

func TestToastShowSingle(t *testing.T) {
	toast := NewToast(DefaultTheme())

	cmd := toast.Show("hello", false)
	if !toast.IsVisible() {
		t.Fatal("expected toast to be visible after Show")
	}
	if toast.current.Text != "hello" {
		t.Fatalf("expected text 'hello', got %q", toast.current.Text)
	}
	if toast.current.IsError {
		t.Fatal("expected IsError=false")
	}
	if cmd == nil {
		t.Fatal("expected a tick command from first Show")
	}
}

func TestToastShowErrorStyle(t *testing.T) {
	toast := NewToast(DefaultTheme())
	toast.Show("fail", true)

	if !toast.current.IsError {
		t.Fatal("expected IsError=true for error toast")
	}
	view := toast.View()
	if view == "" {
		t.Fatal("expected non-empty view for error toast")
	}
}

func TestToastQueueSequential(t *testing.T) {
	toast := NewToast(DefaultTheme())

	// First show starts immediately.
	toast.Show("first", false)
	// Second is queued.
	cmd := toast.Show("second", true)
	if cmd != nil {
		t.Fatal("expected no command for queued toast")
	}
	if len(toast.queue) != 1 {
		t.Fatalf("expected 1 queued item, got %d", len(toast.queue))
	}

	// Verify current is still "first".
	if toast.current.Text != "first" {
		t.Fatalf("expected current 'first', got %q", toast.current.Text)
	}

	// Simulate expiry of first toast.
	firstID := toast.id
	toast, _ = toast.Update(ToastExpiredMsg{ID: firstID})

	if !toast.IsVisible() {
		t.Fatal("expected toast to still be visible (second in queue)")
	}
	if toast.current.Text != "second" {
		t.Fatalf("expected current 'second', got %q", toast.current.Text)
	}
	if !toast.current.IsError {
		t.Fatal("expected second toast to be error")
	}

	// Expire second toast.
	secondID := toast.id
	toast, _ = toast.Update(ToastExpiredMsg{ID: secondID})

	if toast.IsVisible() {
		t.Fatal("expected toast to be dismissed after queue drained")
	}
}

func TestToastAutoDismissOnExpiry(t *testing.T) {
	toast := NewToast(DefaultTheme())
	toast.Show("auto", false)
	id := toast.id

	// Wrong ID should not dismiss.
	toast, _ = toast.Update(ToastExpiredMsg{ID: id + 99})
	if !toast.IsVisible() {
		t.Fatal("toast should not dismiss for wrong ID")
	}

	// Correct ID dismisses.
	toast, _ = toast.Update(ToastExpiredMsg{ID: id})
	if toast.IsVisible() {
		t.Fatal("toast should dismiss for correct ID")
	}
}

func TestToastViewEmpty(t *testing.T) {
	toast := NewToast(DefaultTheme())
	if toast.View() != "" {
		t.Fatal("expected empty view when no toast is active")
	}
}

func TestToastUpdateFromToastMsg(t *testing.T) {
	toast := NewToast(DefaultTheme())
	toast, cmd := toast.Update(ToastMsg{Text: "via msg", IsError: false})

	if !toast.IsVisible() {
		t.Fatal("expected toast visible after ToastMsg")
	}
	if toast.current.Text != "via msg" {
		t.Fatalf("expected 'via msg', got %q", toast.current.Text)
	}
	if cmd == nil {
		t.Fatal("expected tick command from ToastMsg")
	}
}

func TestToastCustomDuration(t *testing.T) {
	toast := NewToast(DefaultTheme())
	item := ToastItem{
		Text:     "custom",
		IsError:  false,
		Duration: 1 * time.Second,
	}
	toast.current = &item
	toast.id = 1

	// Verify the item stores the custom duration.
	if toast.current.Duration != 1*time.Second {
		t.Fatalf("expected 1s duration, got %v", toast.current.Duration)
	}
}
