package tui

// clientCommandDef defines a client-side slash command.
// This is the single canonical list — both the palette and autocomplete
// derive their entries from it to prevent drift.
type clientCommandDef struct {
	Name        string
	Description string
	InPalette   bool // false = autocomplete-only (e.g. exit, undo)
}

// clientCommandDefs is the canonical list of client-side commands.
// Server-side commands are appended at runtime.
var clientCommandDefs = []clientCommandDef{
	{Name: "archive", Description: "Archive current session", InPalette: true},
	{Name: "auto-approve", Description: "Toggle auto-approve for session", InPalette: true},
	{Name: "branch", Description: "Branch conversation to try a different approach", InPalette: true},
	{Name: "btw", Description: "Side question without polluting context", InPalette: true},
	{Name: "changes", Description: "Show session-scoped diff of modified files", InPalette: true},
	{Name: "compact", Description: "Compact context (summarize session)", InPalette: true},
	{Name: "connect", Description: "Select provider and model", InPalette: true},
	{Name: "context", Description: "Show context window usage breakdown", InPalette: true},
	{Name: "copy", Description: "Copy response to clipboard (/copy N for Nth)", InPalette: true},
	{Name: "diagnostics", Description: "Show diagnostics for bug reports", InPalette: true},
	{Name: "diff", Description: "Show uncommitted changes", InPalette: true},
	{Name: "editor", Description: "Open prompt or file in $EDITOR (/editor @file)", InPalette: true},
	{Name: "effort", Description: "Set reasoning depth (low/medium/high/max)", InPalette: true},
	{Name: "exit", Description: "Exit the app", InPalette: false},
	{Name: "export", Description: "Export session as Markdown", InPalette: true},
	{Name: "export-html", Description: "Export session as HTML", InPalette: true},
	{Name: "goal", Description: "Autonomous execution until condition met", InPalette: true},
	{Name: "help", Description: "Show keybindings and commands", InPalette: true},
	{Name: "hooks", Description: "Show configured hooks (plugin and shell)", InPalette: true},
	{Name: "mcp", Description: "Manage MCP servers", InPalette: true},
	{Name: "paste-image", Description: "Paste image from clipboard", InPalette: true},
	{Name: "privacy", Description: "Show what data is stored and where", InPalette: false},
	{Name: "redo", Description: "Restore previously reverted changes", InPalette: false},
	{Name: "rename", Description: "Rename current session", InPalette: true},
	{Name: "rewind", Description: "Rewind conversation to a previous turn", InPalette: true},
	{Name: "scoped-models", Description: "Toggle model scoping (favorites)", InPalette: true},
	{Name: "shell", Description: "Open interactive shell session", InPalette: true},
	{Name: "theme", Description: "Change color theme", InPalette: true},
	{Name: "thinking", Description: "Set reasoning level (off/low/medium/high/max)", InPalette: true},
	{Name: "undo", Description: "Revert last AI file changes", InPalette: false},
}
