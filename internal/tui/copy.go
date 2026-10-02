package tui

import (
	"regexp"
	"strings"
)

// CodeBlock represents a fenced code block extracted from markdown text.
type CodeBlock struct {
	Language string
	Code     string
	Preview  string // first non-empty line, truncated
}

var fencedBlockRe = regexp.MustCompile("(?m)^```(\\w*)\\s*\n((?:.*\n)*?)^```\\s*$")

// extractCodeBlocks finds all fenced code blocks in text.
func extractCodeBlocks(text string) []CodeBlock {
	matches := fencedBlockRe.FindAllStringSubmatch(text, -1)
	blocks := make([]CodeBlock, 0, len(matches))
	for _, m := range matches {
		lang := m[1]
		code := strings.TrimRight(m[2], "\n")
		preview := firstNonEmptyLine(code)
		blocks = append(blocks, CodeBlock{
			Language: lang,
			Code:     code,
			Preview:  preview,
		})
	}
	return blocks
}

// firstNonEmptyLine returns the first non-blank line, truncated to 60 chars.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if len(trimmed) > 60 {
				return trimmed[:57] + "..."
			}
			return trimmed
		}
	}
	return ""
}

// nthAssistantText returns the concatenated text parts from the Nth assistant
// message (1-indexed, counting backwards). N=1 is the last assistant message.
func nthAssistantText(messages []MessageView, n int) string {
	if n < 1 {
		n = 1
	}
	count := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Info.Role != "assistant" {
			continue
		}
		var sb strings.Builder
		for _, p := range messages[i].Parts {
			if p.Type == "text" && p.Text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(p.Text)
			}
		}
		if sb.Len() == 0 {
			continue
		}
		count++
		if count == n {
			return sb.String()
		}
	}
	return ""
}
