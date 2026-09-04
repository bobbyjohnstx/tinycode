package tui

// Key bindings for the TUI.
// NOTE: The foundation layer (keys.go) is being created by another agent.
// These definitions should be consolidated with the canonical versions once available.

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the TUI.
type KeyMap struct {
	Submit    key.Binding
	Newline   key.Binding
	Quit      key.Binding
	Escape    key.Binding
	ScrollUp  key.Binding
	ScrollDown key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Newline: key.NewBinding(
			key.WithKeys("shift+enter", "alt+enter"),
			key.WithHelp("shift+enter", "newline"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "ctrl+d"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "escape"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdown", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdown", "page down"),
		),
	}
}
