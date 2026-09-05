package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/render"
)

// ChatView displays a scrollable list of messages using a viewport.
type ChatView struct {
	viewport     viewport.Model
	messages     []MessageView
	renderer     *render.MarkdownRenderer
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
		renderer:     render.NewMarkdownRenderer(width - 4),
		width:        width,
		height:       height,
		stickyBottom: true,
	}
}

// SetSize updates the viewport dimensions and recreates the markdown renderer.
func (c *ChatView) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.viewport.Width = width
	c.viewport.Height = height
	c.renderer = render.NewMarkdownRenderer(width - 4)
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
			c.messages = nil
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
					c.messages[i].Parts[j].Streaming = true
				}
				return
			}
		}
	}
}

func (c *ChatView) upsertMessage(msg MessageUpdatedMsg) {
	for i := range c.messages {
		if c.messages[i].Info.ID == msg.Message.Info.ID {
			c.messages[i].Info = msg.Message.Info
			return
		}
	}
	c.messages = append(c.messages, msg.Message)
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
		sb.WriteString(renderMessage(msg, c.width, c.renderer))
	}

	c.viewport.SetContent(sb.String())
	if c.stickyBottom {
		c.viewport.GotoBottom()
	}
}
