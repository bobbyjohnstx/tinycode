package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QuestionAskedMsg is a pending question from the question tool.
type QuestionAskedMsg struct {
	ID        string
	SessionID string
	Question  string
	Options   []string
}

// QuestionDismissedMsg is the user's answer (or cancel) for a question prompt.
type QuestionDismissedMsg struct {
	ID        string
	SessionID string
	Answer    string
	Rejected  bool
}

// QuestionPrompt shows a question and collects an answer.
// Requests that arrive while one is visible are queued and shown after it.
type QuestionPrompt struct {
	id        string
	sessionID string
	question  string
	options   []string
	selected  int
	input     textinput.Model
	visible   bool
	width     int
	height    int
	queue     []QuestionAskedMsg
}

// NewQuestionPrompt creates a QuestionPrompt.
func NewQuestionPrompt() QuestionPrompt {
	ti := textinput.New()
	ti.Placeholder = "Type your answer"
	ti.Prompt = ""
	return QuestionPrompt{input: ti}
}

// Show displays the question. A different request that arrives while one is
// already visible is queued. Free-text questions focus the input.
func (p *QuestionPrompt) Show(msg QuestionAskedMsg) tea.Cmd {
	if p.visible {
		if msg.ID == p.id || p.queued(msg.ID) {
			return nil
		}
		p.queue = append(p.queue, msg)
		return nil
	}
	return p.apply(msg)
}

// ShowNext displays the next queued question. It reports whether one was shown.
func (p *QuestionPrompt) ShowNext() bool {
	if len(p.queue) == 0 {
		p.visible = false
		return false
	}
	next := p.queue[0]
	p.queue = p.queue[1:]
	_ = p.apply(next)
	return true
}

// FocusCmd returns a focus command for free-text questions, or nil.
func (p *QuestionPrompt) FocusCmd() tea.Cmd {
	if !p.visible || len(p.options) > 0 {
		return nil
	}
	return p.input.Focus()
}

// VisibleID returns the ID of the currently shown question, or "".
func (p QuestionPrompt) VisibleID() string {
	if !p.visible {
		return ""
	}
	return p.id
}

func (p *QuestionPrompt) apply(msg QuestionAskedMsg) tea.Cmd {
	p.id = msg.ID
	p.sessionID = msg.SessionID
	p.question = msg.Question
	p.options = append([]string(nil), msg.Options...)
	p.selected = 0
	p.visible = true
	p.input.Reset()
	if len(p.options) > 0 {
		p.input.Blur()
		return nil
	}
	return p.input.Focus()
}

func (p *QuestionPrompt) queued(id string) bool {
	for _, q := range p.queue {
		if q.ID == id {
			return true
		}
	}
	return false
}

// removeQueued drops a queued question by ID (SSE reject for a non-visible item).
func (p *QuestionPrompt) removeQueued(id string) {
	if len(p.queue) == 0 {
		return
	}
	kept := p.queue[:0]
	for _, q := range p.queue {
		if q.ID == id {
			continue
		}
		kept = append(kept, q)
	}
	p.queue = kept
}

// Hide closes the question prompt without discarding queued requests.
func (p *QuestionPrompt) Hide() {
	p.visible = false
	p.input.Blur()
}

// IsVisible reports whether the prompt is shown.
func (p QuestionPrompt) IsVisible() bool {
	return p.visible
}

// SetSize updates the prompt dimensions.
func (p *QuestionPrompt) SetSize(width, height int) {
	p.width = width
	p.height = height
	inputWidth := width - 10
	if inputWidth < 20 {
		inputWidth = 20
	}
	p.input.Width = inputWidth
}

// Update handles key events for the question prompt.
func (p QuestionPrompt) Update(msg tea.Msg) (QuestionPrompt, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	if len(p.options) > 0 {
		return p.updateOptions(keyMsg)
	}
	return p.updateText(msg, keyMsg)
}

func (p QuestionPrompt) updateOptions(keyMsg tea.KeyMsg) (QuestionPrompt, tea.Cmd) {
	switch keyMsg.String() {
	case "up", "left", "k", "h":
		if p.selected > 0 {
			p.selected--
		}
	case "down", "right", "j", "l":
		if p.selected < len(p.options)-1 {
			p.selected++
		}
	case "enter":
		if p.selected >= 0 && p.selected < len(p.options) {
			return p.dismiss(p.options[p.selected])
		}
	case "esc":
		return p.dismissRejected()
	}
	return p, nil
}

func (p QuestionPrompt) updateText(msg tea.Msg, keyMsg tea.KeyMsg) (QuestionPrompt, tea.Cmd) {
	switch keyMsg.String() {
	case "enter":
		answer := strings.TrimSpace(p.input.Value())
		if answer == "" {
			return p, nil
		}
		return p.dismiss(answer)
	case "esc":
		return p.dismissRejected()
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

func (p QuestionPrompt) dismiss(answer string) (QuestionPrompt, tea.Cmd) {
	id := p.id
	sessionID := p.sessionID
	p.visible = false
	p.input.Blur()
	return p, func() tea.Msg {
		return QuestionDismissedMsg{ID: id, SessionID: sessionID, Answer: answer}
	}
}

func (p QuestionPrompt) dismissRejected() (QuestionPrompt, tea.Cmd) {
	id := p.id
	sessionID := p.sessionID
	p.visible = false
	p.input.Blur()
	return p, func() tea.Msg {
		return QuestionDismissedMsg{ID: id, SessionID: sessionID, Rejected: true}
	}
}

// View renders the question prompt.
func (p QuestionPrompt) View() string {
	if !p.visible {
		return ""
	}

	dialogWidth := p.width - 4
	if dialogWidth < 50 {
		dialogWidth = 50
	}
	if dialogWidth > 100 {
		dialogWidth = 100
	}

	accent := lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}
	accentStyle := lipgloss.NewStyle().Foreground(accent)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})

	var sb strings.Builder
	sb.WriteString(accentStyle.Render("?") + " Question\n")
	sb.WriteString("  " + p.question + "\n\n")

	if len(p.options) > 0 {
		for i, opt := range p.options {
			if i == p.selected {
				sb.WriteString(accentStyle.Render("  > "+opt) + "\n")
				continue
			}
			sb.WriteString(dimStyle.Render("    "+opt) + "\n")
		}
		sb.WriteString("\n" + dimStyle.Render("↑↓ select  enter answer  esc cancel"))
	} else {
		sb.WriteString("  " + p.input.View() + "\n")
		sb.WriteString("\n" + dimStyle.Render("enter answer  esc cancel"))
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(accent).
		Padding(1, 2)

	return lipgloss.Place(
		p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		borderStyle.Width(dialogWidth).Render(sb.String()),
	)
}
