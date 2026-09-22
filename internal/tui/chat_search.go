package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// searchMatch records the location of a search hit within the message list.
type searchMatch struct {
	msgIdx  int
	partIdx int
	offset  int
}

var styleSearchBar = lipgloss.NewStyle().
	Background(lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#2A2A2A"}).
	Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"})

var styleSearchHint = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})

// IsSearching reports whether in-transcript search is active.
func (c ChatView) IsSearching() bool {
	return c.searchMode
}

// ActivateSearch enters search mode — opens the search bar and focuses the input.
func (c *ChatView) ActivateSearch() {
	c.searchMode = true
	c.searchInput.Reset()
	c.searchInput.Width = c.width / 2
	c.searchInput.Focus()
	c.searchMatches = nil
	c.searchCurrent = 0
	c.viewport.Height = c.height - 1
	c.stickyBottom = false
}

// DismissSearch exits search mode and restores the viewport height.
func (c *ChatView) DismissSearch() {
	c.searchMode = false
	c.searchInput.Blur()
	c.searchMatches = nil
	c.searchCurrent = 0
	c.viewport.Height = c.height
}

// handleSearchKey routes key events during search mode.
func (c ChatView) handleSearchKey(msg tea.KeyMsg) (ChatView, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+f":
		c.DismissSearch()
		return c, nil
	case "enter", "ctrl+n":
		c.nextMatch()
		return c, nil
	case "ctrl+p":
		c.prevMatch()
		return c, nil
	}

	prevQuery := c.searchInput.Value()
	var cmd tea.Cmd
	c.searchInput, cmd = c.searchInput.Update(msg)

	if c.searchInput.Value() != prevQuery {
		c.findMatches()
		if len(c.searchMatches) > 0 {
			c.searchCurrent = 0
			c.scrollToMatch()
		}
	}

	return c, cmd
}

// findMatches scans all message text for case-insensitive matches of the search query.
func (c *ChatView) findMatches() {
	c.searchMatches = nil
	query := strings.ToLower(c.searchInput.Value())
	if query == "" {
		return
	}
	for i, msg := range c.messages {
		for j, part := range msg.Parts {
			if part.Type != "text" && part.Type != "reasoning" {
				continue
			}
			text := strings.ToLower(part.Text)
			offset := 0
			for {
				idx := strings.Index(text[offset:], query)
				if idx < 0 {
					break
				}
				c.searchMatches = append(c.searchMatches, searchMatch{
					msgIdx:  i,
					partIdx: j,
					offset:  offset + idx,
				})
				offset += idx + len(query)
			}
		}
	}
}

// nextMatch advances to the next search match, wrapping around.
func (c *ChatView) nextMatch() {
	if len(c.searchMatches) == 0 {
		return
	}
	c.searchCurrent = (c.searchCurrent + 1) % len(c.searchMatches)
	c.scrollToMatch()
}

// prevMatch moves to the previous search match, wrapping around.
func (c *ChatView) prevMatch() {
	if len(c.searchMatches) == 0 {
		return
	}
	c.searchCurrent--
	if c.searchCurrent < 0 {
		c.searchCurrent = len(c.searchMatches) - 1
	}
	c.scrollToMatch()
}

// scrollToMatch scrolls the viewport to show the message containing the current match.
func (c *ChatView) scrollToMatch() {
	if len(c.searchMatches) == 0 || c.searchCurrent >= len(c.searchMatches) {
		return
	}
	match := c.searchMatches[c.searchCurrent]
	if match.msgIdx < len(c.msgLineStarts) {
		c.viewport.SetYOffset(c.msgLineStarts[match.msgIdx])
		c.stickyBottom = false
	}
}

// renderSearchBar renders the search bar displayed at the top of the chat viewport.
func (c ChatView) renderSearchBar() string {
	var sb strings.Builder
	sb.WriteString("/ ")
	sb.WriteString(c.searchInput.View())

	if c.searchInput.Value() != "" {
		if len(c.searchMatches) == 0 {
			sb.WriteString("  no matches")
		} else {
			fmt.Fprintf(&sb, "  %d/%d", c.searchCurrent+1, len(c.searchMatches))
		}
	}

	hints := styleSearchHint.Render("  ctrl+n/p=nav esc=close")
	sb.WriteString(hints)

	return styleSearchBar.Width(c.width).Render(sb.String())
}
