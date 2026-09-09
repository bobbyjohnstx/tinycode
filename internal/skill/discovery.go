package skill

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/frontmatter"
)

// Skill represents a discovered skill definition.
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Params      []string `json:"params,omitempty"`
	Source      string   `json:"source"`
}

// Discover scans all skill locations and returns the merged list.
// It checks:
//  1. User skills from configDir/skills/*/SKILL.md
//  2. Project skills from projectDir/.tinycode/skills/*/SKILL.md
func Discover(configDir, projectDir string) []Skill {
	seen := make(map[string]struct{})
	var skills []Skill

	userSkillDir := filepath.Join(configDir, "skills")
	skills = appendSkillsFromDir(skills, seen, userSkillDir, "user")

	if projectDir != "" {
		projectSkillDir := filepath.Join(projectDir, ".tinycode", "skills")
		skills = appendSkillsFromDir(skills, seen, projectSkillDir, "project")
	}

	return skills
}

func appendSkillsFromDir(skills []Skill, seen map[string]struct{}, dir, source string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return skills
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

		s := Skill{
			ID:     name,
			Name:   name,
			Source: source,
		}
		if desc, ok := fm["description"].(string); ok {
			s.Description = desc
		}
		if params, ok := fm["params"].(string); ok {
			s.Params = parseParamsList(params)
		}

		skills = append(skills, s)
	}

	return skills
}

// parseParamsList parses a YAML-style list string like "[name, language]".
func parseParamsList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
