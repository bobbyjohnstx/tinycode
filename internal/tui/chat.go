package tui

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/render"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// ChatView displays a scrollable list of messages using a viewport.
type ChatView struct {
	viewport     viewport.Model
	messages     []MessageView
	renderer     *render.MarkdownRenderer
	thoughtLines map[int]string // content line number → part ID
	width        int
	height       int
	stickyBottom bool

	// Subagent group state
	subagentExpanded map[string]bool           // label → expanded
	subagentStatus   map[string]SubagentStatus // label → completion data
	subagentLines    map[int]string            // content line number → subagent label

	// Mouse drag selection state
	dragging  bool
	dragStart [2]int // [line, col] in content coordinates
	dragEnd   [2]int
}

// NewChatView creates a ChatView with the given dimensions.
func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.SetContent("")

	return ChatView{
		viewport:         vp,
		renderer:         render.NewMarkdownRenderer(width - 4),
		width:            width,
		height:           height,
		stickyBottom:     true,
		subagentExpanded: make(map[string]bool),
		subagentStatus:   make(map[string]SubagentStatus),
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

	case ToggleThoughtMsg:
		c.toggleThought(msg.PartID)
		c.rebuildContent()
		return c, nil

	case ToggleSubagentMsg:
		c.toggleSubagent(msg.Label)
		c.rebuildContent()
		return c, nil

	case SubagentCompletedMsg:
		c.subagentStatus[msg.Label] = SubagentStatus{
			Label:        msg.Label,
			Agent:        msg.Agent,
			InputTokens:  msg.InputTokens,
			OutputTokens: msg.OutputTokens,
			Done:         true,
		}
		c.rebuildContent()
		return c, nil

	case tea.KeyMsg:
		if msg.String() == "T" {
			c.toggleThought("")
			c.rebuildContent()
			return c, nil
		}

	case tea.MouseMsg:
		slog.Debug("chat mouse event", "button", msg.Button, "action", msg.Action, "x", msg.X, "y", msg.Y, "dragging", c.dragging)
		contentLine := c.viewport.YOffset + msg.Y
		switch {
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			if partID, ok := c.thoughtLines[contentLine]; ok {
				c.toggleThought(partID)
				c.rebuildContent()
				return c, nil
			}
			if label, ok := c.subagentLines[contentLine]; ok {
				c.toggleSubagent(label)
				c.rebuildContent()
				return c, nil
			}
			c.dragging = true
			c.dragStart = [2]int{contentLine, msg.X}
			c.dragEnd = c.dragStart
			slog.Info("drag started", "start", c.dragStart)
		case msg.Action == tea.MouseActionMotion && c.dragging:
			c.dragEnd = [2]int{contentLine, msg.X}
		case msg.Action == tea.MouseActionRelease && c.dragging:
			c.dragging = false
			c.dragEnd = [2]int{contentLine, msg.X}
			selected := c.extractSelection()
			slog.Info("drag released", "start", c.dragStart, "end", c.dragEnd, "selectedLen", len(selected))
			if selected != "" {
				return c, copyToClipboard(selected)
			}
		}

	case MessagesLoadedMsg:
		return c.handleMessagesLoaded(msg)
	}

	// Delegate viewport key/mouse handling.
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)

	if c.viewport.AtBottom() {
		c.stickyBottom = true
	} else {
		c.stickyBottom = false
	}

	return c, cmd
}

// HasMessages reports whether any messages have been loaded.
func (c ChatView) HasMessages() bool {
	return len(c.messages) > 0
}

// Messages returns the current message list.
func (c ChatView) Messages() []MessageView {
	return c.messages
}

// View implements tea.Model.
func (c ChatView) View() string {
	view := c.viewport.View()
	if !c.dragging {
		return view
	}
	return c.applySelectionHighlight(view)
}

// applySelectionHighlight overlays reverse-video on the dragged region.
func (c ChatView) applySelectionHighlight(view string) string {
	lines := strings.Split(view, "\n")

	startLine := c.dragStart[0] - c.viewport.YOffset
	startCol := c.dragStart[1]
	endLine := c.dragEnd[0] - c.viewport.YOffset
	endCol := c.dragEnd[1]

	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, endLine = endLine, startLine
		startCol, endCol = endCol, startCol
	}

	for i := range lines {
		if i < startLine || i > endLine {
			continue
		}
		sc := 0
		ec := len(stripAnsi(lines[i]))
		if i == startLine {
			sc = startCol
		}
		if i == endLine {
			ec = endCol
		}
		lines[i] = highlightLineRange(lines[i], sc, ec)
	}

	return strings.Join(lines, "\n")
}

// highlightLineRange applies reverse video (\x1b[7m) to the visual column
// range [startCol, endCol) on a line that may contain ANSI escape sequences.
func highlightLineRange(line string, startCol, endCol int) string {
	if startCol >= endCol {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + 16)
	visualCol := 0
	highlighted := false
	runes := []rune(line)

	for i := 0; i < len(runes); {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) && !((runes[j] >= 'A' && runes[j] <= 'Z') || (runes[j] >= 'a' && runes[j] <= 'z')) {
				j++
			}
			if j < len(runes) {
				j++
			}
			sb.WriteString(string(runes[i:j]))
			i = j
			continue
		}

		if visualCol == startCol && !highlighted {
			sb.WriteString("\x1b[7m")
			highlighted = true
		}
		if visualCol == endCol && highlighted {
			sb.WriteString("\x1b[27m")
			highlighted = false
		}

		sb.WriteRune(runes[i])
		visualCol++
		i++
	}

	if highlighted {
		sb.WriteString("\x1b[27m")
	}

	return sb.String()
}

// applyDelta appends streaming text to the matching part.
func (c *ChatView) applyDelta(msg MessagePartDeltaMsg) {
	for i := range c.messages {
		if c.messages[i].Info.ID != msg.MessageID {
			continue
		}
		for j := range c.messages[i].Parts {
			if c.messages[i].Parts[j].ID != msg.PartID {
				continue
			}
			if msg.Field == "text" {
				c.messages[i].Parts[j].Text += msg.Delta
				c.messages[i].Parts[j].Streaming = true
			}
			return
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
				// Preserve UI-only state that lives outside the server data model.
				msg.Part.ThoughtExpanded = c.messages[i].Parts[j].ThoughtExpanded
				msg.Part.Collapsed = c.messages[i].Parts[j].Collapsed
				c.messages[i].Parts[j] = msg.Part
				return
			}
		}
		c.messages[i].Parts = append(c.messages[i].Parts, msg.Part)
		return
	}
}

// toggleThought toggles the expanded state of reasoning parts.
// If partID is empty, toggles all reasoning parts.
func (c *ChatView) toggleThought(partID string) {
	for i := range c.messages {
		for j := range c.messages[i].Parts {
			p := &c.messages[i].Parts[j]
			if p.Type != "reasoning" {
				continue
			}
			if partID == "" || p.ID == partID {
				p.ThoughtExpanded = !p.ThoughtExpanded
			}
		}
	}
}

// toggleSubagent toggles the expanded state of a subagent group.
// If label is empty, toggles all subagent groups.
func (c *ChatView) toggleSubagent(label string) {
	if label == "" {
		anyExpanded := false
		for _, exp := range c.subagentExpanded {
			if exp {
				anyExpanded = true
				break
			}
		}
		// Collect all labels from messages
		for _, msg := range c.messages {
			for _, p := range msg.Parts {
				if p.SubagentLabel != "" {
					c.subagentExpanded[p.SubagentLabel] = !anyExpanded
				}
			}
		}
		return
	}
	c.subagentExpanded[label] = !c.subagentExpanded[label]
}

// thoughtHit records a thought label's line offset within rendered output.
type thoughtHit struct {
	lineOffset int
	partID     string
}

// extractSelection returns the text between dragStart and dragEnd positions.
func (c *ChatView) extractSelection() string {
	content := c.viewport.View()
	lines := strings.Split(content, "\n")

	startLine := c.dragStart[0] - c.viewport.YOffset
	startCol := c.dragStart[1]
	endLine := c.dragEnd[0] - c.viewport.YOffset
	endCol := c.dragEnd[1]

	// Normalize direction
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, endLine = endLine, startLine
		startCol, endCol = endCol, startCol
	}

	// Same position = click, not drag
	if startLine == endLine && startCol == endCol {
		return ""
	}

	// Clamp to visible content
	if startLine < 0 {
		startLine = 0
		startCol = 0
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
		endCol = len(lines[endLine])
	}

	var sb strings.Builder
	for i := startLine; i <= endLine; i++ {
		if i >= len(lines) {
			break
		}
		line := stripAnsi(lines[i])
		sc := 0
		ec := len(line)
		if i == startLine {
			sc = startCol
		}
		if i == endLine {
			ec = endCol
		}
		if sc > len(line) {
			sc = len(line)
		}
		if ec > len(line) {
			ec = len(line)
		}
		if sc > ec {
			sc = ec
		}
		sb.WriteString(strings.TrimRight(line[sc:ec], " "))
		if i < endLine {
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// handleMessagesLoaded converts raw message maps into MessageViews.
func (c ChatView) handleMessagesLoaded(msg MessagesLoadedMsg) (ChatView, tea.Cmd) {
	if msg.Err == nil {
		views := make([]MessageView, 0, len(msg.Messages))
		for _, m := range msg.Messages {
			mv := parseMessageView(m)
			mv.Parts = parseLoadedParts(m)
			views = append(views, mv)
		}
		c.messages = views
		c.rebuildContent()
	}
	return c, nil
}

func parseLoadedParts(m map[string]any) []PartView {
	parts, ok := m["parts"].([]any)
	if !ok {
		return nil
	}
	views := make([]PartView, 0, len(parts))
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		views = append(views, parsePartView(map[string]any{"part": pm}))
	}
	return views
}

// rebuildContent renders all messages into the viewport.
func (c *ChatView) rebuildContent() {
	var sb strings.Builder
	c.thoughtLines = make(map[int]string)
	c.subagentLines = make(map[int]string)
	lineNum := 0

	for i, msg := range c.messages {
		if i > 0 {
			sb.WriteString("\n")
			lineNum++
		}
		var tHits []thoughtHit
		var sHits []subagentHit
		opts := &renderOpts{
			thoughtHits:      &tHits,
			subagentHits:     &sHits,
			subagentExpanded: c.subagentExpanded,
			subagentStatus:   c.subagentStatus,
		}
		rendered := renderMessageWithOpts(msg, c.width, c.renderer, opts)
		for _, h := range tHits {
			c.thoughtLines[lineNum+h.lineOffset] = h.partID
		}
		for _, h := range sHits {
			c.subagentLines[lineNum+h.lineOffset] = h.label
		}
		sb.WriteString(rendered)
		lineNum += strings.Count(rendered, "\n")
	}

	c.viewport.SetContent(sb.String())
	if c.stickyBottom {
		c.viewport.GotoBottom()
	}
}
