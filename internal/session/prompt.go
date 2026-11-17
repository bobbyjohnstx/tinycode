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

// maxImportDepth is the maximum nesting depth for @path imports in instruction
// files. A chain of 5 imports (depth 0 → 1 → 2 → 3 → try 4) stops at the 5th
// file and reports a skip.
const maxImportDepth = 4

// instructionBlock holds a discovered instruction file's display path and content.
type instructionBlock struct {
	Path    string
	Content string
}

// SystemPromptInput contains all inputs needed to build the system prompt.
type SystemPromptInput struct {
	AgentPrompt        string
	AgentName          string
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

	if input.AgentName == "plan" {
		sections = append(sections, "You are in plan mode. Do not modify files outside plans/ and drafts/. If the user asks to implement, tell them to switch agents.")
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
// display paths relative to dir when possible. Each file's content is
// expanded for @path imports before being added to the block list.
func readInstructionBlocks(dir string, files []string) []instructionBlock {
	homeDir, _ := os.UserHomeDir()
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
		visited := map[string]bool{path: true}
		trimmed = expandImports(trimmed, path, dir, homeDir, 0, visited)
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

// expandImports processes @path directives in instruction file content, inlining
// the referenced files. Paths are resolved relative to the directory of filePath.
// Lines inside fenced code blocks or inline backticks are left untouched.
// Absolute paths are only allowed inside homeDir or projectDir.
func expandImports(content, filePath, projectDir, homeDir string, depth int, visited map[string]bool) string {
	dir := filepath.Dir(filePath)
	lines := strings.Split(content, "\n")
	var result []string
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track fenced code blocks (``` toggles state).
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			result = append(result, line)
			continue
		}
		if inFence {
			result = append(result, line)
			continue
		}

		// A line is an import directive when the entire trimmed line is @<path>.
		if !strings.HasPrefix(trimmed, "@") || len(trimmed) < 2 {
			result = append(result, line)
			continue
		}

		// Skip @path inside inline backticks.
		atIdx := strings.Index(line, "@")
		if isInsideBackticks(line, atIdx) {
			result = append(result, line)
			continue
		}

		importPath := trimmed[1:]

		// Depth limit: skip further includes.
		if depth >= maxImportDepth {
			result = append(result, fmt.Sprintf("[import skipped (depth limit): %s]", importPath))
			continue
		}

		// Resolve relative to containing file's directory.
		var resolved string
		if filepath.IsAbs(importPath) {
			resolved = filepath.Clean(importPath)
			// Safety: absolute paths must be inside home or project directory.
			if homeDir == "" || (!strings.HasPrefix(resolved, homeDir+string(filepath.Separator)) &&
				resolved != homeDir &&
				!strings.HasPrefix(resolved, projectDir+string(filepath.Separator)) &&
				resolved != projectDir) {
				result = append(result, line)
				continue
			}
		} else {
			resolved = filepath.Clean(filepath.Join(dir, importPath))
		}

		// Cycle detection.
		if visited[resolved] {
			result = append(result, fmt.Sprintf("[import cycle: %s]", importPath))
			continue
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			result = append(result, line) // leave directive as text
			continue
		}

		visited[resolved] = true
		included := strings.TrimSpace(string(data))
		expanded := expandImports(included, resolved, projectDir, homeDir, depth+1, visited)
		result = append(result, expanded)
	}

	return strings.Join(result, "\n")
}

// isInsideBackticks returns true if the character at pos in line falls between
// an odd number of preceding backtick characters (i.e. inside inline code).
func isInsideBackticks(line string, pos int) bool {
	count := 0
	for i := 0; i < pos && i < len(line); i++ {
		if line[i] == '`' {
			count++
		}
	}
	return count%2 == 1
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
// CLAUDE.local.md and AGENTS.local.md are loaded after their shared counterparts
// when present (both load — local does not replace the shared file). These local
// variants belong in .gitignore.
// Results are returned outermost first so the innermost (closest to the project)
// file wins when appended last.
func discoverInstructionFiles(dir string) []string {
	var files []string
	current := dir
	for {
		// Collection order is innermost-first; the final reversal flips it to
		// outermost-first. To ensure each .local.md appears AFTER its shared
		// file post-reversal, we add .local.md BEFORE the shared file here.

		// CLAUDE.local.md (before CLAUDE.md → after CLAUDE.md post-reversal)
		claudeLocalPath := filepath.Join(current, "CLAUDE.local.md")
		if _, err := os.Stat(claudeLocalPath); err == nil {
			files = append(files, claudeLocalPath)
		}
		// CLAUDE.md
		claudePath := filepath.Join(current, "CLAUDE.md")
		if _, err := os.Stat(claudePath); err == nil {
			files = append(files, claudePath)
		}

		// AGENTS.local.md (before AGENTS.md → after AGENTS.md post-reversal)
		agentsLocalPath := filepath.Join(current, "AGENTS.local.md")
		if _, err := os.Stat(agentsLocalPath); err == nil {
			files = append(files, agentsLocalPath)
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
