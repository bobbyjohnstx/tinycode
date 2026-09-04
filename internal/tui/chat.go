package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ChatView displays a scrollable list of messages using a viewport.
type ChatView struct {
	viewport     viewport.Model
	messages     []MessageView
	width        int
	height       int
	stickyBottom bool
}

// NewChatView creates a ChatView with the given dimensions.
func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.SetContent("")

	return ChatView{
		viewport:     vp,
		width:        width,
		height:       height,
		stickyBottom: true,
	}
}

// SetSize updates the viewport dimensions.
func (c *ChatView) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.viewport.Width = width
	c.viewport.Height = height
	c.rebuildContent()
}

// Init implements tea.Model.
func (c ChatView) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (c ChatView) Update(msg tea.Msg) (ChatView, tea.Cmd) {
	switch msg := msg.(type) {
	case MessagePartDeltaMsg:
		c.applyDelta(msg)
		c.rebuildContent()
		return c, nil

	case MessageUpdatedMsg:
		c.upsertMessage(msg)
		c.rebuildContent()
		return c, nil

	case MessagePartUpdatedMsg:
		c.upsertPart(msg)
		c.rebuildContent()
		return c, nil

	case MessagesLoadedMsg:
		if msg.Err == nil {
			c.messages = msg.Messages
			c.rebuildContent()
		}
		return c, nil
	}

	// Delegate viewport key/mouse handling.
	wasAtBottom := c.viewport.AtBottom()
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)

	// Track sticky-bottom: if user scrolled up, disable auto-scroll.
	if wasAtBottom && !c.viewport.AtBottom() {
		c.stickyBottom = false
	}

	return c, cmd
}

// View implements tea.Model.
func (c ChatView) View() string {
	return c.viewport.View()
}

// applyDelta appends streaming text to the matching part.
func (c *ChatView) applyDelta(msg MessagePartDeltaMsg) {
	for i := range c.messages {
		if c.messages[i].Info.ID != msg.MessageID {
			continue
		}
		for j := range c.messages[i].Parts {
			if c.messages[i].Parts[j].ID == msg.PartID {
				if msg.Field == "text" {
					c.messages[i].Parts[j].Text += msg.Delta
				}
				return
			}
		}
	}
}

// upsertMessage inserts or updates a message by ID.
func (c *ChatView) upsertMessage(msg MessageUpdatedMsg) {
	for i := range c.messages {
		if c.messages[i].Info.ID == msg.Info.ID {
			c.messages[i].Info = msg.Info
			return
		}
	}
	c.messages = append(c.messages, MessageView{Info: msg.Info})
}

// upsertPart inserts or updates a part within its parent message.
func (c *ChatView) upsertPart(msg MessagePartUpdatedMsg) {
	for i := range c.messages {
		if c.messages[i].Info.ID != msg.Part.MessageID {
			continue
		}
		for j := range c.messages[i].Parts {
			if c.messages[i].Parts[j].ID == msg.Part.ID {
				c.messages[i].Parts[j] = msg.Part
				return
			}
		}
		c.messages[i].Parts = append(c.messages[i].Parts, msg.Part)
		return
	}
}

// rebuildContent renders all messages into the viewport.
func (c *ChatView) rebuildContent() {
	var sb strings.Builder
	for i, msg := range c.messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(renderMessage(msg, c.width))
	}

	c.viewport.SetContent(sb.String())
	if c.stickyBottom {
		c.viewport.GotoBottom()
	}
}
