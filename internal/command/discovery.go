package command

import (
	"os"
	"path/filepath"
	"strings"
)

// Command represents a discoverable slash command.
type Command struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Source      string   `json:"source"`
	Template    string   `json:"template,omitempty"`
	Subtask     bool     `json:"subtask,omitempty"`
	Hints       []string `json:"hints"`
}

// Discover scans all command/skill locations and returns a merged list.
// It includes:
//  1. Built-in commands (init, review, ask, swarm)
//  2. Default skills from the embedded agent defaults directory (agent names as commands)
//  3. User skills from configDir/skills/*/SKILL.md
//  4. Project skills from projectDir/.tinycode/skills/*/SKILL.md
func Discover(configDir, projectDir string, agentNames []string) []Command {
	seen := make(map[string]struct{})
	var commands []Command

	for _, cmd := range builtinCommands() {
		seen[cmd.Name] = struct{}{}
		commands = append(commands, cmd)
	}

	for _, name := range agentNames {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		commands = append(commands, Command{
			Name:        name,
			Description: "Switch to " + name + " agent",
			Source:      "builtin",
			Template:    "",
			Hints:       []string{},
		})
	}

	userSkillDir := filepath.Join(configDir, "skills")
	commands = appendSkillCommands(commands, seen, userSkillDir, "skill")

	if projectDir != "" {
		projectSkillDir := filepath.Join(projectDir, ".tinycode", "skills")
		commands = appendSkillCommands(commands, seen, projectSkillDir, "skill")
	}

	return commands
}

func builtinCommands() []Command {
	return []Command{
		{
			Name:        "init",
			Description: "Guided project setup",
			Source:      "builtin",
			Template:    "Initialize this project for AI-assisted development.",
			Hints:       []string{},
		},
		{
			Name:        "review",
			Description: "Review changes -- /review [commit|branch|pr]",
			Source:      "builtin",
			Template:    "Review the recent changes in this project.",
			Subtask:     true,
			Hints:       []string{"$1"},
		},
		{
			Name:        "ask",
			Description: "Ask an agent -- /ask <agent> <prompt>",
			Source:      "builtin",
			Template:    "$2",
			Subtask:     true,
			Hints:       []string{"$1", "$2"},
		},
		{
			Name:        "swarm",
			Description: "Dispatch parallel subagents for multi-task work",
			Source:      "builtin",
			Template:    "Break down this task and dispatch subagents.",
			Subtask:     true,
			Hints:       []string{"$1"},
		},
	}
}

func appendSkillCommands(commands []Command, seen map[string]struct{}, dir, source string) []Command {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return commands
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		fm, _ := parseFrontmatter(string(data))
		name := entry.Name()
		if n, ok := fm["name"].(string); ok && n != "" {
			name = n
		}

		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}

		cmd := Command{
			Name:   name,
			Source: source,
			Hints:  []string{},
		}
		if desc, ok := fm["description"].(string); ok {
			cmd.Description = desc
		}

		commands = append(commands, cmd)
	}

	return commands
}

// parseFrontmatter extracts YAML-like frontmatter from markdown content.
func parseFrontmatter(content string) (map[string]any, string) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content
	}

	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return nil, content
	}

	fm := content[4 : 4+end]
	body := content[4+end+4:]

	result := make(map[string]any)
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		result[key] = value
	}

	return result, body
}
