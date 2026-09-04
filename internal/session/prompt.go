package session

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
)

// SystemPromptInput contains all inputs needed to build the system prompt.
type SystemPromptInput struct {
	AgentPrompt  string
	Instructions string
	Directory    string
	ToolDefs     []llm.Tool
}

// BuildSystemPrompt assembles a system prompt from the agent persona, discovered
// CLAUDE.md files (walking up from Directory), and any extra instructions from
// config. Files that cannot be read are silently skipped.
func BuildSystemPrompt(input SystemPromptInput) string {
	var sections []string

	if input.AgentPrompt != "" {
		sections = append(sections, input.AgentPrompt)
	}

	if input.Directory != "" {
		claudeFiles := discoverClaudeMD(input.Directory)
		for _, path := range claudeFiles {
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			trimmed := strings.TrimSpace(string(content))
			if trimmed != "" {
				sections = append(sections, trimmed)
			}
		}
	}

	if input.Instructions != "" {
		sections = append(sections, input.Instructions)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// discoverClaudeMD walks up from dir to the filesystem root, collecting paths
// to CLAUDE.md and .tinycode/CLAUDE.md files. Results are returned outermost
// first so the innermost (closest to the project) file wins when appended last.
func discoverClaudeMD(dir string) []string {
	var files []string
	current := dir
	for {
		for _, candidate := range []string{
			filepath.Join(current, "CLAUDE.md"),
			filepath.Join(current, ".tinycode", "CLAUDE.md"),
		} {
			if _, err := os.Stat(candidate); err == nil {
				files = append(files, candidate)
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	// Reverse so outermost appears first (same convention as config/paths.go)
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}
	return files
}
