package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
)

// LoadFromDirectory loads agent definitions from .md files in dir.
// Files are parsed for YAML frontmatter (name, description, permission, etc).
// Compact variants (*.compact.md) are loaded alongside their full counterparts.
// Agents already in the registry are not overwritten.
func (r *Registry) LoadFromDirectory(dir string, defaultPerms, userPerms permission.Ruleset) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		agentName := strings.TrimSuffix(entry.Name(), ".md")
		if r.agents[agentName] != nil {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		frontmatter, body := parseFrontmatter(string(data))
		info := &Info{
			Name:    agentName,
			Mode:    ModeAll,
			Options: make(map[string]any),
			Native:  false,
			Prompt:  strings.TrimSpace(body),
		}

		if desc, ok := frontmatter["description"].(string); ok {
			info.Description = desc
		}
		if mode, ok := frontmatter["mode"].(string); ok {
			info.Mode = Mode(mode)
		}
		if hidden, ok := frontmatter["hidden"].(bool); ok {
			info.Hidden = hidden
		}
		if color, ok := frontmatter["color"].(string); ok {
			info.Color = color
		}
		if steps, ok := frontmatter["steps"].(int); ok {
			info.Steps = &steps
		}

		agentPerm := extractPermissionRules(frontmatter)
		info.Permission = permission.Merge(defaultPerms, agentPerm, userPerms)

		r.agents[agentName] = info
	}

	return nil
}

// LoadUserAgents loads agents from user config and project directories.
// It scans ~/.config/tinycode/agents/ and .tinycode/agent/ directories.
func (r *Registry) LoadUserAgents(configDir, projectDir string, defaultPerms, userPerms permission.Ruleset) {
	userAgentDir := filepath.Join(configDir, "agents")
	r.LoadFromDirectory(userAgentDir, defaultPerms, userPerms)

	if projectDir != "" {
		projectAgentDir := filepath.Join(projectDir, ".tinycode", "agent")
		r.LoadFromDirectory(projectAgentDir, defaultPerms, userPerms)
	}
}
