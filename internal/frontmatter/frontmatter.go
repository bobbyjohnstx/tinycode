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
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// If the value is empty, check for YAML-style list items on subsequent lines.
		if value == "" {
			var items []string
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if !strings.HasPrefix(next, "- ") {
					break
				}
				items = append(items, unquote(strings.TrimSpace(next[2:])))
				i++
			}
			if len(items) > 0 {
				result[key] = items
				continue
			}
		}

		result[key] = value
	}

	return result, body
}

// unquote strips matching surrounding double or single quotes from s.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
