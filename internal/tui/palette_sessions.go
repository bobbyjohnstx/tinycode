package tui

import (
	"fmt"
	"time"
)

// buildSessionPaletteItems returns PaletteItems for sessions, skipping the active one.
func buildSessionPaletteItems(sessions []SessionInfo, activeID string) []PaletteItem {
	var items []PaletteItem
	for _, s := range sessions {
		if s.ID == activeID {
			continue
		}
		title := s.Title
		if title == "" {
			title = s.ID
		}
		desc := timeAgo(s.UpdatedAt)
		items = append(items, PaletteItem{
			Label:       title,
			Description: desc,
			Category:    "session",
			Value:       "session:" + s.ID,
		})
	}
	return items
}

// timeAgo returns a human-readable relative time string from a unix timestamp.
func timeAgo(unixSec int64) string {
	if unixSec == 0 {
		return ""
	}
	d := time.Since(time.Unix(unixSec, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
