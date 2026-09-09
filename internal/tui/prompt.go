package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var oscHexFragment = regexp.MustCompile(`^[0-9a-fA-F]{1,4}(/[0-9a-fA-F]{1,4}){1,2}\\?$`)

// PromptInput is a multi-line text input with a metadata bar,
// autocomplete popover, and prompt history navigation.
type PromptInput struct {
	textarea     textarea.Model
	autocomplete Autocomplete
	history      PromptHistory
	agent        string
	model        string
	provider     string
	width        int
	keys         KeyMap
	guardEnabled bool
	startTime    time.Time
}

// NewPromptInput creates a PromptInput with the given width.
func NewPromptInput(width int) PromptInput {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetWidth(width - 4)
	ta.SetHeight(3)
	ta.Focus()
	ta.CharLimit = 0

	// Strip all default textarea chrome — the prompt accent bar handles the visual frame.
	noBorder := lipgloss.NewStyle()
	ta.FocusedStyle.Base = noBorder
	ta.FocusedStyle.CursorLine = noBorder
	ta.FocusedStyle.CursorLineNumber = noBorder
	ta.FocusedStyle.EndOfBuffer = noBorder
	ta.FocusedStyle.Prompt = noBorder
	ta.BlurredStyle.Base = noBorder
	ta.BlurredStyle.CursorLine = noBorder
	ta.BlurredStyle.CursorLineNumber = noBorder
	ta.BlurredStyle.EndOfBuffer = noBorder
	ta.BlurredStyle.Prompt = noBorder

	return PromptInput{
		textarea:     ta,
		autocomplete: NewAutocomplete(),
		history:      NewPromptHistory(),
		agent:        "build",
		width:        width,
		keys:         DefaultKeyMap(),
	}
}

// EnableStartupGuard arms the startup guard. The actual timer starts
// when ResetStartupGuard is called (typically on first render).
func (p *PromptInput) EnableStartupGuard() {
	p.guardEnabled = true
	p.startTime = time.Now()
}

// ResetStartupGuard restarts the guard timer if the guard was armed.
func (p *PromptInput) ResetStartupGuard() {
	if p.guardEnabled {
		p.startTime = time.Now()
	}
}

// SetSize updates the prompt width.
func (p *PromptInput) SetSize(width int) {
	p.width = width
	p.textarea.SetWidth(width - 4)
	p.autocomplete.SetWidth(width)
}

// SetCommands sets the available slash commands for autocomplete.
func (p *PromptInput) SetCommands(cmds []AutocompleteItem) {
	p.autocomplete.SetCommands(cmds)
	p.autocomplete.SetWidth(p.width)
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
		// Discard terminal escape sequences that leak through bubbletea's input parser.
		if s := keyMsg.String(); isTerminalEscape(s) {
			return p, nil
		}
		// Discard rune-only input during startup grace period — terminal
		// responses (OSC color, CSI cursor reports) arrive as printable
		// characters indistinguishable from typing.
		if keyMsg.Type == tea.KeyRunes && p.guardEnabled && time.Since(p.startTime) < 2*time.Second {
			return p, nil
		}
		// When the autocomplete popover is visible, it gets priority
		// for navigation keys so the user can browse and select commands.
		if p.autocomplete.IsVisible() {
			switch keyMsg.String() {
			case "up", "down", "esc":
				var acCmd tea.Cmd
				p.autocomplete, acCmd, _ = p.autocomplete.Update(keyMsg)
				return p, acCmd
			case "tab":
				selected := p.autocomplete.Selected()
				if selected != "" {
					p.textarea.SetValue("/" + selected + " ")
				}
				p.autocomplete, _, _ = p.autocomplete.Update(keyMsg)
				return p, nil
			case "enter":
				selected := p.autocomplete.Selected()
				if selected != "" {
					current := strings.TrimPrefix(strings.TrimSpace(p.textarea.Value()), "/")
					if current != selected && !strings.HasPrefix(current, selected+" ") {
						// Still completing — fill in the command name
						p.textarea.SetValue("/" + selected + " ")
						p.autocomplete, _, _ = p.autocomplete.Update(keyMsg)
						return p, nil
					}
					// User already typed the full command — dismiss and fall through to submit
					p.autocomplete, _, _ = p.autocomplete.Update(keyMsg)
				}
				// Fall through to normal enter handling.
			}
		}

		switch keyMsg.String() {
		case "enter":
			// Submit on Enter (plain, no modifier).
			content := p.textarea.Value()
			if content != "" {
				p.history.Add(content)
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

		case "up":
			// Navigate history when autocomplete is not visible.
			if prev, ok := p.history.Previous(p.textarea.Value()); ok {
				p.textarea.SetValue(prev)
				return p, nil
			}
			return p, nil

		case "down":
			// Navigate history forward.
			if next, ok := p.history.Next(p.textarea.Value()); ok {
				p.textarea.SetValue(next)
			} else {
				p.textarea.SetValue(p.history.StashValue())
			}
			return p, nil
		}
	}

	var cmd tea.Cmd
	p.textarea, cmd = p.textarea.Update(msg)

	// Update autocomplete based on current input text.
	p.autocomplete.UpdateInput(p.textarea.Value())

	return p, cmd
}

// View implements tea.Model.
func (p PromptInput) View() string {
	innerWidth := p.width - 4 // account for accent border + padding

	taView := p.textarea.View()
	meta := p.renderMetadata()
	inner := lipgloss.JoinVertical(lipgloss.Left, taView, meta)

	box := stylePromptAccent.Width(innerWidth).Render(inner)
	top := stylePromptBorder.Width(p.width - 2).Render(box)

	if p.autocomplete.IsVisible() {
		acView := p.autocomplete.View()
		return lipgloss.JoinVertical(lipgloss.Left, acView, top)
	}

	return top
}

// renderMetadata renders the status line below the textarea.
func (p PromptInput) renderMetadata() string {
	agentPrefix := "  " + p.agent
	modelProvider := truncatedModelProvider(p.model, p.provider, p.width-len(agentPrefix)-5)
	label := agentPrefix
	if modelProvider != "" {
		label = fmt.Sprintf("%s · %s", agentPrefix, modelProvider)
	}
	return styleMetadata.Width(p.width).Render(label)
}

// isTerminalEscape returns true if the string looks like a terminal escape
// response that leaked through bubbletea's input parser (raw ESC/C1 bytes
// or OSC color response fragments like "rgb:" or "11;").
func isTerminalEscape(s string) bool {
	if strings.ContainsAny(s, "\x1b\x9c") {
		return true
	}
	if strings.Contains(s, "rgb:") || strings.HasPrefix(s, "11;") || strings.HasPrefix(s, "10;") {
		return true
	}
	if oscHexFragment.MatchString(s) {
		return true
	}
	return false
}
