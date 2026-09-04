package llm

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	fenceStartRe = regexp.MustCompile(`(?i)^` + "```" + `(?:json)?\s*\n?`)
	fenceEndRe   = regexp.MustCompile(`\n?` + "```" + `\s*$`)
	trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)
)

// RepairToolCallJSON attempts to fix malformed JSON from LLM tool calls.
// Strips markdown fences and trailing commas. Returns nil if repair fails.
func RepairToolCallJSON(raw string) *string {
	s := strings.TrimSpace(raw)
	s = fenceStartRe.ReplaceAllString(s, "")
	s = fenceEndRe.ReplaceAllString(s, "")
	s = trailingCommaRe.ReplaceAllString(s, "$1")
	s = strings.TrimSpace(s)

	if json.Valid([]byte(s)) {
		return &s
	}
	return nil
}
