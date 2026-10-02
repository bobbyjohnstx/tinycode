package command

import (
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode/internal/frontmatter"
	"github.com/bobbyjohnstx/tinycode/internal/skill"
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
//  2. Agent names as commands
//  3. User skills from configDir/skills/*/SKILL.md
//  4. Project skills from projectDir/.tinycode/skills/*/SKILL.md
//  5. Bundled default skills (lowest priority, overridden by all above)
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

	// Append bundled default skills last — user/project skills override by name.
	for _, ds := range skill.DefaultSkills() {
		if _, exists := seen[ds.Name]; exists {
			continue
		}
		seen[ds.Name] = struct{}{}
		commands = append(commands, Command{
			Name:        ds.Name,
			Description: ds.Description,
			Source:      "skill",
			Hints:       []string{},
		})
	}

	return commands
}

func builtinCommands() []Command {
	return []Command{
		{
			Name:        "branch",
			Description: "Branch conversation -- /branch [name]",
			Source:      "builtin",
			Hints:       []string{"$1"},
		},
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
		{
			Name:        "auto-approve",
			Description: "Toggle auto-approve for tool permissions",
			Source:      "builtin",
			Hints:       []string{},
		},
		{
			Name:        "btw",
			Description: "Side question without polluting context -- /btw <question>",
			Source:      "builtin",
			Hints:       []string{"$1"},
		},
		{
			Name:        "goal",
			Description: "Autonomous execution until condition met -- /goal <condition>",
			Source:      "builtin",
			Hints:       []string{"$1"},
		},
		{
			Name:        "rewind",
			Description: "Rewind conversation to a previous turn",
			Source:      "builtin",
			Hints:       []string{},
		},
		{
			Name:        "hooks",
			Description: "Show configured hooks (plugin and shell)",
			Source:      "builtin",
			Hints:       []string{},
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

		fm, _ := frontmatter.Parse(string(data))
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
