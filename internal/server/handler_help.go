package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode-go/internal/command"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
)

// HelpKeybinding describes a single keyboard shortcut.
type HelpKeybinding struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// HelpCommand describes a slash command.
type HelpCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// HelpFeature describes a product feature.
type HelpFeature struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HelpResponse is the top-level response for GET /help.
type HelpResponse struct {
	Keybindings []HelpKeybinding `json:"keybindings"`
	Commands    []HelpCommand    `json:"commands"`
	Features    []HelpFeature    `json:"features"`
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	// Keybindings: web UI defaults (these match the SPA's command palette keybinds).
	keybindings := []HelpKeybinding{
		{Key: "mod+shift+p", Description: "Open command palette", Category: "General"},
		{Key: "mod+n", Description: "New session", Category: "Session"},
		{Key: "mod+shift+a", Description: "Archive session", Category: "Session"},
		{Key: "mod+.", Description: "Open settings", Category: "General"},
		{Key: "mod+/", Description: "Toggle sidebar", Category: "Navigation"},
		{Key: "mod+shift+1", Description: "Focus file tree", Category: "Navigation"},
		{Key: "mod+shift+m", Description: "Select model", Category: "Model and agent"},
		{Key: "mod+shift+e", Description: "Select agent", Category: "Model and agent"},
		{Key: "mod+`", Description: "Toggle terminal", Category: "Terminal"},
		{Key: "Escape", Description: "Cancel / close dialog", Category: "General"},
		{Key: "Enter", Description: "Send message", Category: "Prompt"},
		{Key: "shift+Enter", Description: "New line in prompt", Category: "Prompt"},
		// In-transcript search.
		{Key: "ctrl+f", Description: "Open in-transcript search", Category: "Search"},
		{Key: "ctrl+n", Description: "Next search match", Category: "Search"},
		{Key: "ctrl+p (in search)", Description: "Previous search match", Category: "Search"},
		// TUI leader key sequences (ctrl+x prefix).
		{Key: "ctrl+x b", Description: "Toggle sidebar", Category: "TUI leader"},
		{Key: "ctrl+x n", Description: "New session", Category: "TUI leader"},
		{Key: "ctrl+x o", Description: "Open session list", Category: "TUI leader"},
		{Key: "ctrl+x m", Description: "Open model selector", Category: "TUI leader"},
		{Key: "ctrl+x a", Description: "Open agent list", Category: "TUI leader"},
		{Key: "ctrl+x e", Description: "Open $EDITOR to compose a prompt", Category: "TUI leader"},
		{Key: "ctrl+x d", Description: "Open diff viewer", Category: "TUI leader"},
		{Key: "ctrl+x t", Description: "Open theme picker", Category: "TUI leader"},
		{Key: "ctrl+x i", Description: "Open MCP server management", Category: "TUI leader"},
		{Key: "ctrl+x x", Description: "Export session as Markdown", Category: "TUI leader"},
		{Key: "ctrl+x y", Description: "Copy last response to clipboard", Category: "TUI leader"},
		{Key: "ctrl+x u", Description: "Undo last AI file changes", Category: "TUI leader"},
		{Key: "ctrl+x r", Description: "Redo reverted changes", Category: "TUI leader"},
	}

	// Commands: discover from config + project directory.
	var agentNames []string
	if s.deps.AgentRegistry != nil {
		for _, a := range s.deps.AgentRegistry.List("") {
			agentNames = append(agentNames, a.Name)
		}
	}
	configDir := config.ConfigDir()
	discovered := command.Discover(configDir, s.config.Directory, agentNames)

	commands := make([]HelpCommand, 0, len(discovered))
	for _, cmd := range discovered {
		commands = append(commands, HelpCommand{
			Name:        cmd.Name,
			Description: cmd.Description,
			Source:      cmd.Source,
		})
	}

	// Features: static descriptions of key product capabilities.
	features := []HelpFeature{
		{Name: "@ File References", Description: "Type @ in the prompt to reference files by path. Autocomplete with directory drill-down helps find files in your project."},
		{Name: "Slash Commands", Description: "Type / to see available commands like /review, /ask, /swarm, and custom skills."},
		{Name: "Agent System", Description: "Switch between specialized agents (architect, debugger, executor) for different tasks. Build agent delegates to executor, architect, and critic."},
		{Name: "Subagent Swarm", Description: "Use /swarm to dispatch parallel subagents that work on independent tasks simultaneously."},
		{Name: "Session Management", Description: "Create, archive, fork, and switch between sessions. Sessions are auto-titled from the first prompt."},
		{Name: "File Tree", Description: "Browse and navigate project files in the sidebar. Click to reference files in your prompt."},
		{Name: "VCS Integration", Description: "View git status, diffs, and changes directly in the interface. /diff and ctrl+x d open the diff viewer."},
		{Name: "MCP Servers", Description: "Connect external tool servers via the Model Context Protocol. Use /mcp or ctrl+x i to manage servers."},
		{Name: "Image Paste", Description: "Use /paste-image to paste clipboard images as multimodal input for vision-capable models."},
		{Name: "Extended Thinking", Description: "Use /thinking to control reasoning budget (off/low/medium/high/max) for deeper analysis."},
		{Name: "Model Scoping", Description: "Use /scoped-models to mark favorite models. When active, only favorites appear in the model selector."},
		{Name: "Undo/Redo", Description: "Use /undo and /redo (or ctrl+x u/r) to revert or restore AI file changes via snapshots."},
		{Name: "Bundled Skills", Description: "10 built-in skills (debug, verify, trace, review, plan, test, doctor, mcp-setup, remember, deepinit) available as slash commands."},
		{Name: "apply_patch Tool", Description: "Atomic multi-file edits via unified diff format. The model uses this to apply changes across multiple files in one operation."},
		{Name: "In-Transcript Search", Description: "Press ctrl+f to search the chat transcript. ctrl+n/ctrl+p to navigate matches, Esc to close."},
		{Name: "Which-Key Panel", Description: "Press ctrl+x to see a floating panel of all available leader key follow-ups grouped by category."},
		{Name: "Terminal Bell", Description: "Rings the terminal bell when a task completes or a permission prompt appears."},
		{Name: "Session Archive", Description: "Use /archive to soft-delete the current session. Archived sessions are removed from the list but recoverable."},
		{Name: "HTML Export", Description: "Use /export html to export the current session as a self-contained HTML file with syntax highlighting."},
		{Name: "Manual Compaction", Description: "Use /compact to summarize older messages and free context window space."},
		{Name: "tinycode doctor", Description: "Run tinycode doctor from the CLI for a headless health check of config, database, providers, agents, plugins, and skills."},
		{Name: "Safe Mode", Description: "Start with --safe-mode to skip plugins, MCP servers, and user-defined agents. The status bar shows an orange SAFE MODE indicator."},
		{Name: "Session Resume", Description: "Use -c/--continue to resume the most recent session, or -r/--resume to resume by ID or title substring."},
		{Name: "System Prompt Override", Description: "Use --append-system-prompt or --append-system-prompt-file to inject custom instructions into the system prompt."},
		{Name: "Token Budget", Description: "Use --max-tokens to set a cumulative token ceiling. The session aborts when total input+output tokens exceed the budget."},
		{Name: "Session Title", Description: "Use --title to set the session title from the command line."},
	}

	respondJSON(w, http.StatusOK, HelpResponse{
		Keybindings: keybindings,
		Commands:    commands,
		Features:    features,
	})
}
