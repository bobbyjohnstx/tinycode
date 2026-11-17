package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const maxWarnings = 50

// Warning is a single session-scoped warning entry.
type Warning struct {
	Time   time.Time
	Source string
	Text   string
}

// WarningList is a session-scoped, capped list of warnings.
// Safe for concurrent use.
type WarningList struct {
	mu       sync.Mutex
	warnings []Warning
}

// NewWarningList creates an empty warning list.
func NewWarningList() *WarningList {
	return &WarningList{}
}

// Add appends a warning. When the list exceeds maxWarnings, the oldest
// entry is dropped.
func (w *WarningList) Add(source, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = append(w.warnings, Warning{
		Time:   time.Now(),
		Source: source,
		Text:   text,
	})
	if len(w.warnings) > maxWarnings {
		w.warnings = w.warnings[len(w.warnings)-maxWarnings:]
	}
}

// All returns a copy of all current warnings.
func (w *WarningList) All() []Warning {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]Warning, len(w.warnings))
	copy(cp, w.warnings)
	return cp
}

// Clear removes all warnings.
func (w *WarningList) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = w.warnings[:0]
}

// Len returns the current count.
func (w *WarningList) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.warnings)
}

// Render formats all warnings for display in a debug dialog.
func (w *WarningList) Render() string {
	items := w.All()
	if len(items) == 0 {
		return "No warnings."
	}
	var sb strings.Builder
	for _, warn := range items {
		fmt.Fprintf(&sb, "[%s] %s: %s\n", warn.Time.Format("15:04:05"), warn.Source, warn.Text)
	}
	return sb.String()
}
