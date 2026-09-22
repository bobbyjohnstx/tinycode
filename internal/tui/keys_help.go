package tui

import "github.com/charmbracelet/bubbles/key"

// keybindingPaletteItems returns informational PaletteItems grouped by section,
// extracted from the KeyMap help metadata. Items have empty Value — selecting
// them is a no-op.
func keybindingPaletteItems(keys KeyMap) []PaletteItem {
	separator := func(label string) PaletteItem {
		return PaletteItem{Label: "── " + label + " ──", Description: "", Value: ""}
	}
	entry := func(b key.Binding) PaletteItem {
		h := b.Help()
		return PaletteItem{Label: h.Key, Description: h.Desc, Value: ""}
	}

	return []PaletteItem{
		separator("Prompt"),
		entry(keys.Submit),
		entry(keys.Newline),
		entry(keys.CycleAgentNext),
		{Label: "@", Description: "file completion", Value: ""},
		{Label: "/", Description: "commands", Value: ""},

		separator("Leader (ctrl+x)"),
		entry(keys.ToggleSidebar),
		entry(keys.AgentList),
		entry(keys.ModelList),
		entry(keys.SessionList),
		entry(keys.NewSession),
		entry(keys.ExportSession),
		entry(keys.Undo),
		entry(keys.Redo),

		separator("Global"),
		entry(keys.CommandPalette),
		entry(keys.ClearOrQuit),
		entry(keys.Quit),
		entry(keys.Interrupt),
		entry(keys.ScrollUp),
		entry(keys.ScrollDown),
	}
}
