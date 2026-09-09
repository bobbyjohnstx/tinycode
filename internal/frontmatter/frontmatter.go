package frontmatter

import "strings"

// Parse extracts YAML-like frontmatter from markdown content.
// Returns the frontmatter key-value pairs and the remaining body.
func Parse(content string) (map[string]any, string) {
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
