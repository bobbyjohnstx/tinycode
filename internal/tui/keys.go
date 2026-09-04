package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all TUI keybindings.
type KeyMap struct {
	// Global
	CommandPalette key.Binding
	Quit           key.Binding
	ClearOrQuit    key.Binding

	// Prompt
	Submit     key.Binding
	Newline    key.Binding
	AltNewline key.Binding
	Interrupt  key.Binding

	// Navigation
	CycleAgentNext key.Binding
	CycleAgentPrev key.Binding
	CycleModelNext key.Binding
	CycleModelPrev key.Binding
	ScrollUp       key.Binding
	ScrollDown     key.Binding

	// Leader key sequences (ctrl+x prefix)
	LeaderKey     key.Binding
	ToggleSidebar key.Binding // ctrl+x b
	SessionList   key.Binding // ctrl+x o
	NewSession    key.Binding // ctrl+x n
	ModelList     key.Binding // ctrl+x m
	AgentList     key.Binding // ctrl+x a
	Undo          key.Binding // ctrl+x u
	Redo          key.Binding // ctrl+x r
}

// DefaultKeyMap returns the default key bindings matching the design doc.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		CommandPalette: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "command palette"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "quit"),
		),
		ClearOrQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "clear/quit"),
		),

		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Newline: key.NewBinding(
			key.WithKeys("shift+enter"),
			key.WithHelp("shift+enter", "newline"),
		),
		AltNewline: key.NewBinding(
			key.WithKeys("alt+enter"),
			key.WithHelp("alt+enter", "newline"),
		),
		Interrupt: key.NewBinding(
			key.WithKeys("escape"),
			key.WithHelp("esc", "interrupt"),
		),

		CycleAgentNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next agent"),
		),
		CycleAgentPrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev agent"),
		),
		CycleModelNext: key.NewBinding(
			key.WithKeys("f2"),
			key.WithHelp("f2", "next model"),
		),
		CycleModelPrev: key.NewBinding(
			key.WithKeys("shift+f2"),
			key.WithHelp("shift+f2", "prev model"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdown", "scroll down"),
		),

		LeaderKey: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "leader"),
		),
		ToggleSidebar: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("ctrl+x b", "toggle sidebar"),
		),
		SessionList: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("ctrl+x o", "session list"),
		),
		NewSession: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("ctrl+x n", "new session"),
		),
		ModelList: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("ctrl+x m", "model list"),
		),
		AgentList: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("ctrl+x a", "agent list"),
		),
		Undo: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("ctrl+x u", "undo"),
		),
		Redo: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("ctrl+x r", "redo"),
		),
	}
}
