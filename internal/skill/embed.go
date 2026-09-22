package skill

import (
	"embed"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/frontmatter"
)

//go:embed defaults/*.md
var defaultsFS embed.FS

// DefaultSkills returns the bundled default skill definitions.
func DefaultSkills() []Skill {
	entries, err := defaultsFS.ReadDir("defaults")
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := defaultsFS.ReadFile(filepath.Join("defaults", entry.Name()))
		if err != nil {
			continue
		}

		fm, _ := frontmatter.Parse(string(data))
		id := strings.TrimSuffix(entry.Name(), ".md")
		name := id
		if n, ok := fm["name"].(string); ok && n != "" {
			name = n
		}

		s := Skill{
			ID:     id,
			Name:   name,
			Source: "builtin",
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

// ReadDefaultSkill returns the full content of a bundled default skill by ID.
func ReadDefaultSkill(id string) (string, error) {
	data, err := defaultsFS.ReadFile(filepath.Join("defaults", id+".md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
