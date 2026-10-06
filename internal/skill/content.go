package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/frontmatter"
)

// LoadContent reads a skill's markdown body with frontmatter stripped.
// For user/project/path skills (Dir set), reads Dir/SKILL.md.
// For builtins (Dir empty), reads the embedded default by ID.
func LoadContent(s Skill) (string, error) {
	var raw string
	if s.Dir != "" {
		data, err := os.ReadFile(filepath.Join(s.Dir, "SKILL.md"))
		if err != nil {
			return "", err
		}
		raw = string(data)
	} else {
		content, err := ReadDefaultSkill(s.ID)
		if err != nil {
			return "", err
		}
		raw = content
	}
	_, body := frontmatter.Parse(raw)
	return strings.TrimPrefix(body, "\n"), nil
}

// SubstituteParams replaces $N placeholders (highest index first) and
// $ARGUMENTS in skill content. Replacing from high to low avoids $1 eating
// the prefix of $10.
func SubstituteParams(content, arguments string) string {
	parts := strings.Fields(arguments)

	for i := len(parts); i >= 1; i-- {
		placeholder := fmt.Sprintf("$%d", i)
		content = strings.ReplaceAll(content, placeholder, parts[i-1])
	}

	content = strings.ReplaceAll(content, "$ARGUMENTS", arguments)
	return content
}
