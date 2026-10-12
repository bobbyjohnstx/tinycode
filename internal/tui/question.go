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
}

// QuestionPrompt shows a question and collects an answer.
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
}

// NewQuestionPrompt creates a QuestionPrompt.
func NewQuestionPrompt() QuestionPrompt {
	ti := textinput.New()
	ti.Placeholder = "Type your answer"
	ti.Prompt = ""
	return QuestionPrompt{input: ti}
}

// Show displays the question. Free-text questions focus the input.
func (p *QuestionPrompt) Show(msg QuestionAskedMsg) tea.Cmd {
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

// Hide closes the question prompt.
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
		return p.dismiss("cancelled")
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
		return p.dismiss("cancelled")
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
