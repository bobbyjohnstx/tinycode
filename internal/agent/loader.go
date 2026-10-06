package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

// LoadFromDirectory loads agent definitions from .md files in dir.
// Files are parsed for YAML frontmatter (name, description, permission, etc).
// Compact variants (*.compact.md) are loaded alongside their full counterparts.
// Native agents are never overwritten. Non-native (bundled) agents may be replaced
// by user/project definitions.
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
		if existing := r.agents[agentName]; existing != nil && existing.Native {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			slog.Error("reading agent file", "path", filepath.Join(dir, entry.Name()), "err", err)
			continue
		}

		frontmatter, body := parseFrontmatter(string(data))
		info := &Info{
			Name:    agentName,
			Mode:    ModeSubagent,
			Options: make(map[string]any),
			Native:  false,
			Prompt:  strings.TrimSpace(body),
		}
		applyFrontmatter(info, frontmatter)

		agentPerm := extractPermissionRules(frontmatter)
		info.Permission = permission.Merge(defaultPerms, agentPerm, userPerms)

		r.agents[agentName] = info
	}

	return nil
}

// LoadUserAgents loads agents from user config and project directories.
// Order: user (~/.config/tinycode/agents and .../agent for compat), then project
// (.tinycode/agent). Later loads overwrite earlier non-native agents, so project wins.
func (r *Registry) LoadUserAgents(configDir, projectDir string, defaultPerms, userPerms permission.Ruleset) {
	for _, dir := range []string{
		filepath.Join(configDir, "agents"),
		filepath.Join(configDir, "agent"),
	} {
		if err := r.LoadFromDirectory(dir, defaultPerms, userPerms); err != nil {
			slog.Error("loading user agents", "dir", dir, "err", err)
		}
	}

	if projectDir != "" {
		projectAgentDir := filepath.Join(projectDir, ".tinycode", "agent")
		if err := r.LoadFromDirectory(projectAgentDir, defaultPerms, userPerms); err != nil {
			slog.Error("loading project agents", "dir", projectAgentDir, "err", err)
		}
	}
}
