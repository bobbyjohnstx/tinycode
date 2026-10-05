package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
)

// SystemPromptInput contains all inputs needed to build the system prompt.
type SystemPromptInput struct {
	AgentPrompt        string
	Instructions       string
	Directory          string
	ToolDefs           []llm.Tool
	GitBranch          string
	Platform           string
	AppendSystemPrompt string
}

// BuildSystemPrompt assembles a system prompt from the agent persona, discovered
// instruction files (CLAUDE.md, AGENTS.md walking up from Directory), an LSP
// hint when those tools are present, environment info, and any extra
// instructions from config. Files that cannot be read are silently skipped.
// Tool schemas are not copied into the prompt; the request carries them.
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

	if section := buildToolSection(input.ToolDefs); section != "" {
		sections = append(sections, section)
	}

	if env := buildEnvironmentSection(input); env != "" {
		sections = append(sections, env)
	}

	if input.Instructions != "" {
		sections = append(sections, input.Instructions)
	}

	if input.AppendSystemPrompt != "" {
		sections = append(sections, input.AppendSystemPrompt)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// buildToolSection returns the LSP usage hint when those tools are available.
// Tool names and descriptions are omitted: the request already carries the
// JSON schemas.
func buildToolSection(tools []llm.Tool) string {
	for _, tool := range tools {
		if strings.HasPrefix(tool.Function.Name, "lsp_") {
			return "Use `lsp_diagnostics` to check files for errors after writing or editing code. Use `lsp_hover` and `lsp_definition` to understand APIs before coding against them."
		}
	}
	return ""
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
