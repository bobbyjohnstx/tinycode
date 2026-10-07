package tui

// HistoryLoadedMsg delivers persisted history entries on startup.
type HistoryLoadedMsg struct {
	Entries []string
}

// maxHistorySize is the maximum number of entries in the ring buffer.
const maxHistorySize = 100

// PromptHistory is a ring buffer of prompt entries with stash support.
// Add deduplicates consecutive entries. Previous/Next navigate the buffer.
// Stash saves the current in-progress input and swaps it on toggle.
type PromptHistory struct {
	entries []string
	cursor  int    // -1 means "not navigating" (at the live input)
	stash   string // saved in-progress input while navigating
}

// NewPromptHistory creates an empty PromptHistory.
func NewPromptHistory() PromptHistory {
	return PromptHistory{
		cursor: -1,
	}
}

// Add appends an entry to the history. Consecutive duplicates are skipped.
func (h *PromptHistory) Add(entry string) {
	if entry == "" {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > maxHistorySize {
		h.entries = h.entries[len(h.entries)-maxHistorySize:]
	}
	h.cursor = -1
}

// Len returns the number of history entries.
func (h *PromptHistory) Len() int {
	return len(h.entries)
}

// Entries returns a copy of the history entries in chronological order
// (oldest first). Used by the history browser popover.
func (h *PromptHistory) Entries() []string {
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out
}

// Stash saves the current input text and resets the cursor for navigation.
func (h *PromptHistory) Stash(current string) {
	h.stash = current
	h.cursor = -1
}

// StashValue returns the stashed text.
func (h *PromptHistory) StashValue() string {
	return h.stash
}

// Previous moves backward in history, returning the entry and true,
// or "" and false if at the beginning. On first call, stashes the current input.
func (h *PromptHistory) Previous(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.cursor == -1 {
		// First navigation: stash the live input.
		h.stash = current
		h.cursor = len(h.entries) - 1
	} else if h.cursor > 0 {
		h.cursor--
	} else {
		// Already at oldest entry, wrap to stash.
		h.cursor = -1
		return h.stash, true
	}
	return h.entries[h.cursor], true
}

// Next moves forward in history, returning the entry and true,
// or the stashed value when past the newest entry.
func (h *PromptHistory) Next(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.cursor == -1 {
		// Not navigating, wrap to oldest.
		h.stash = current
		h.cursor = 0
		return h.entries[h.cursor], true
	}
	if h.cursor < len(h.entries)-1 {
		h.cursor++
		return h.entries[h.cursor], true
	}
	// Past newest, return stash.
	h.cursor = -1
	return h.stash, true
}

// Reset clears the navigation state without clearing entries.
func (h *PromptHistory) Reset() {
	h.cursor = -1
	h.stash = ""
}
