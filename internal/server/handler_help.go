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
		{Name: "@ File References", Description: "Type @ in the prompt to reference files by path. Autocomplete helps find files in your project."},
		{Name: "Slash Commands", Description: "Type / to see available commands like /review, /ask, /swarm, and custom skills."},
		{Name: "Agent System", Description: "Switch between specialized agents (architect, debugger, executor) for different tasks."},
		{Name: "Subagent Swarm", Description: "Use /swarm to dispatch parallel subagents that work on independent tasks simultaneously."},
		{Name: "Session Management", Description: "Create, archive, fork, and switch between sessions. Each session maintains its own conversation history."},
		{Name: "File Tree", Description: "Browse and navigate project files in the sidebar. Click to reference files in your prompt."},
		{Name: "VCS Integration", Description: "View git status, diffs, and changes directly in the interface."},
		{Name: "MCP Servers", Description: "Connect external tool servers via the Model Context Protocol for extended capabilities."},
	}

	respondJSON(w, http.StatusOK, HelpResponse{
		Keybindings: keybindings,
		Commands:    commands,
		Features:    features,
	})
}
