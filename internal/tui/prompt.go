package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PromptInput is a multi-line text input with a metadata bar.
type PromptInput struct {
	textarea textarea.Model
	agent    string
	model    string
	provider string
	width    int
	keys     KeyMap
}

// NewPromptInput creates a PromptInput with the given width.
func NewPromptInput(width int) PromptInput {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.ShowLineNumbers = false
	ta.SetWidth(width - 2) // account for border
	ta.SetHeight(3)
	ta.Focus()
	ta.CharLimit = 0 // no limit

	return PromptInput{
		textarea: ta,
		agent:    "build",
		width:    width,
		keys:     DefaultKeyMap(),
	}
}

// SetSize updates the prompt width.
func (p *PromptInput) SetSize(width int) {
	p.width = width
	p.textarea.SetWidth(width - 2)
}

// SetMetadata updates the agent/model display below the textarea.
func (p *PromptInput) SetMetadata(agent, model, provider string) {
	p.agent = agent
	p.model = model
	p.provider = provider
}

// Value returns the current text content.
func (p *PromptInput) Value() string {
	return p.textarea.Value()
}

// Reset clears the textarea content.
func (p *PromptInput) Reset() {
	p.textarea.Reset()
}

// Focus gives keyboard focus to the textarea.
func (p *PromptInput) Focus() {
	p.textarea.Focus()
}

// Blur removes keyboard focus from the textarea.
func (p *PromptInput) Blur() {
	p.textarea.Blur()
}

// Init implements tea.Model.
func (p PromptInput) Init() tea.Cmd {
	return textarea.Blink
}

// Update implements tea.Model.
func (p PromptInput) Update(msg tea.Msg) (PromptInput, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			// Submit on Enter (plain, no modifier).
			content := p.textarea.Value()
			if content != "" {
				p.textarea.Reset()
				return p, func() tea.Msg {
					return PromptSubmittedMsg{Content: content}
				}
			}
			return p, nil

		case "shift+enter", "alt+enter":
			// Insert newline.
			p.textarea, _ = p.textarea.Update(msg)
			return p, nil
		}
	}

	var cmd tea.Cmd
	p.textarea, cmd = p.textarea.Update(msg)
	return p, cmd
}

// View implements tea.Model.
func (p PromptInput) View() string {
	ta := stylePromptBorder.Width(p.width - 2).Render(p.textarea.View())
	meta := p.renderMetadata()
	return lipgloss.JoinVertical(lipgloss.Left, ta, meta)
}

// renderMetadata renders the status line below the textarea.
func (p PromptInput) renderMetadata() string {
	label := p.agent
	if p.model != "" {
		label = fmt.Sprintf("%s · %s", p.agent, p.model)
	}
	if p.provider != "" {
		label = fmt.Sprintf("%s %s", label, p.provider)
	}
	return styleMetadata.Width(p.width).Render("  " + label)
}
