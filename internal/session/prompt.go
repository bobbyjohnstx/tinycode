package session

import (
	"fmt"
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
	GitBranch    string
	Platform     string
}

// BuildSystemPrompt assembles a system prompt from the agent persona, discovered
// instruction files (CLAUDE.md, AGENTS.md walking up from Directory), tool
// definitions, environment info, and any extra instructions from config. Files
// that cannot be read are silently skipped.
func BuildSystemPrompt(input SystemPromptInput) string {
	var sections []string

	if input.AgentPrompt != "" {
		sections = append(sections, input.AgentPrompt)
	}

	if input.Directory != "" {
		instrFiles := discoverInstructionFiles(input.Directory)
		for _, path := range instrFiles {
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

	if len(input.ToolDefs) > 0 {
		sections = append(sections, buildToolSection(input.ToolDefs))
	}

	if env := buildEnvironmentSection(input); env != "" {
		sections = append(sections, env)
	}

	if input.Instructions != "" {
		sections = append(sections, input.Instructions)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// buildToolSection creates a section listing available tools and their descriptions.
func buildToolSection(tools []llm.Tool) string {
	var sb strings.Builder
	sb.WriteString("# Available Tools\n")
	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**", tool.Function.Name))
		if tool.Function.Description != "" {
			sb.WriteString(fmt.Sprintf(": %s", tool.Function.Description))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// buildEnvironmentSection creates a section with environment context.
func buildEnvironmentSection(input SystemPromptInput) string {
	var parts []string
	if input.Directory != "" {
		parts = append(parts, fmt.Sprintf("Working directory: %s", input.Directory))
	}
	if input.Platform != "" {
		parts = append(parts, fmt.Sprintf("Platform: %s", input.Platform))
	}
	if input.GitBranch != "" {
		parts = append(parts, fmt.Sprintf("Git branch: %s", input.GitBranch))
	}
	if len(parts) == 0 {
		return ""
	}
	return "# Environment\n" + strings.Join(parts, "\n")
}

// discoverInstructionFiles walks up from dir to the filesystem root, collecting
// paths to CLAUDE.md, AGENTS.md, and .tinycode/CLAUDE.md files. Results are
// returned outermost first so the innermost (closest to the project) file wins
// when appended last.
func discoverInstructionFiles(dir string) []string {
	var files []string
	current := dir
	for {
		for _, candidate := range []string{
			filepath.Join(current, "CLAUDE.md"),
			filepath.Join(current, "AGENTS.md"),
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

// discoverClaudeMD is an alias for backward compatibility. Callers should prefer
// discoverInstructionFiles.
func discoverClaudeMD(dir string) []string {
	return discoverInstructionFiles(dir)
}
