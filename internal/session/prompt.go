package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
)

// maxInstructionBytes caps the combined size of discovered instruction files
// and AppendSystemPrompt text. When exceeded, outermost files are dropped first.
const maxInstructionBytes = 32768

// instructionBlock holds a discovered instruction file's display path and content.
type instructionBlock struct {
	Path    string
	Content string
}

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
		blocks := readInstructionBlocks(input.Directory, instrFiles)
		instrSection := formatInstructionBlocks(blocks, len(input.AppendSystemPrompt))
		if instrSection != "" {
			sections = append(sections, instrSection)
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

// readInstructionBlocks reads the given files and returns blocks with
// display paths relative to dir when possible.
func readInstructionBlocks(dir string, files []string) []instructionBlock {
	var blocks []instructionBlock
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" {
			continue
		}
		displayPath := path
		if rel, err := filepath.Rel(dir, path); err == nil {
			displayPath = rel
		}
		blocks = append(blocks, instructionBlock{
			Path:    displayPath,
			Content: trimmed,
		})
	}
	return blocks
}

// formatInstructionBlocks assembles instruction blocks into a single prompt
// section with [path] headers. Combined file text plus appendSize is capped
// at maxInstructionBytes; outermost blocks are dropped first when over the cap.
func formatInstructionBlocks(blocks []instructionBlock, appendSize int) string {
	if len(blocks) == 0 {
		return ""
	}

	total := appendSize
	for _, b := range blocks {
		total += len(b.Content)
	}

	omitted := 0
	for total > maxInstructionBytes && len(blocks) > 1 {
		total -= len(blocks[0].Content)
		blocks = blocks[1:]
		omitted++
	}

	var sb strings.Builder
	if omitted > 0 {
		fmt.Fprintf(&sb, "[%d instruction files omitted due to size limit]\n\n", omitted)
	}
	for i, b := range blocks {
		fmt.Fprintf(&sb, "[%s]\n%s", b.Path, b.Content)
		if i < len(blocks)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// discoverInstructionFiles walks up from dir to the filesystem root, collecting
// paths to CLAUDE.md, AGENTS.md (or AGENTS.override.md), and .tinycode/CLAUDE.md
// files. In each directory, AGENTS.override.md replaces AGENTS.md when present.
// Results are returned outermost first so the innermost (closest to the project)
// file wins when appended last.
func discoverInstructionFiles(dir string) []string {
	var files []string
	current := dir
	for {
		// CLAUDE.md
		claudePath := filepath.Join(current, "CLAUDE.md")
		if _, err := os.Stat(claudePath); err == nil {
			files = append(files, claudePath)
		}
		// AGENTS.override.md replaces AGENTS.md for this directory only
		overridePath := filepath.Join(current, "AGENTS.override.md")
		agentsPath := filepath.Join(current, "AGENTS.md")
		if _, err := os.Stat(overridePath); err == nil {
			files = append(files, overridePath)
		} else if _, err := os.Stat(agentsPath); err == nil {
			files = append(files, agentsPath)
		}
		// .tinycode/CLAUDE.md
		tinycodePath := filepath.Join(current, ".tinycode", "CLAUDE.md")
		if _, err := os.Stat(tinycodePath); err == nil {
			files = append(files, tinycodePath)
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

// FormatInstructionInfo returns a human-readable listing of discovered
// instruction blocks with paths and byte sizes. Used by /instructions.
func FormatInstructionInfo(dir string) string {
	files := discoverInstructionFiles(dir)
	blocks := readInstructionBlocks(dir, files)

	var sb strings.Builder
	total := 0
	for _, b := range blocks {
		total += len(b.Content)
	}

	fmt.Fprintf(&sb, "INSTRUCTION BLOCKS (%d files, %d bytes, cap %d bytes)\n\n", len(blocks), total, maxInstructionBytes)

	if len(blocks) == 0 {
		sb.WriteString("  (none found)\n")
		return sb.String()
	}

	for i, b := range blocks {
		fmt.Fprintf(&sb, "  %d. [%s]  %d bytes\n", i+1, b.Path, len(b.Content))
	}

	if total > maxInstructionBytes {
		sb.WriteString("\n  Over cap: outermost files will be dropped at prompt assembly\n")
	}

	return sb.String()
}
