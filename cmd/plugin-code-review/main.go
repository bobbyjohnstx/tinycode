package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// codeReviewArgs is the input schema for the code_review tool.
type codeReviewArgs struct {
	Ref          string `json:"ref"`
	Path         string `json:"path"`
	Staged       bool   `json:"staged"`
	ContextLines *int   `json:"context_lines"`
}

// newPlugin returns the code-review plugin definition.
func newPlugin() plugin.Plugin {
	return plugin.Plugin{
		ID: "code-review",
		Tools: []plugin.ToolDef{{
			Name:        "code_review",
			Description: "Show git diff for code review, formatted as a markdown diff block",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ref":           map[string]any{"type": "string", "description": "Git ref to diff against (default: HEAD)"},
					"path":          map[string]any{"type": "string", "description": "Limit diff to a specific file or directory"},
					"staged":        map[string]any{"type": "boolean", "description": "Show only staged changes"},
					"context_lines": map[string]any{"type": "integer", "description": "Number of context lines in diff (default: 3)"},
				},
			},
			Execute: executeCodeReview,
		}},
	}
}

// buildDiffArgs constructs the git diff argument list from the parsed args.
func buildDiffArgs(args codeReviewArgs) []string {
	ref := args.Ref
	if ref == "" {
		ref = "HEAD"
	}

	contextLines := 3
	if args.ContextLines != nil {
		contextLines = *args.ContextLines
	}

	diffArgs := []string{"diff", fmt.Sprintf("-U%d", contextLines)}

	if args.Staged {
		diffArgs = append(diffArgs, "--cached")
	}

	diffArgs = append(diffArgs, ref)

	if args.Path != "" {
		diffArgs = append(diffArgs, "--", args.Path)
	}

	return diffArgs
}

// formatOutput formats the diff output as a markdown code review block.
func formatOutput(ref string, diff string) string {
	if diff == "" {
		return "No changes found."
	}

	// Count changed files by looking for "diff --git" lines.
	fileCount := strings.Count(diff, "diff --git")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Code Review: Changes against %s\n\n", ref))
	sb.WriteString(fmt.Sprintf("%d file(s) changed\n\n", fileCount))
	sb.WriteString("```diff\n")
	sb.WriteString(diff)
	if !strings.HasSuffix(diff, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")
	return sb.String()
}

// executeCodeReview runs git diff and returns formatted output.
func executeCodeReview(ctx context.Context, raw json.RawMessage, tc plugin.ToolContext) (string, error) {
	var args codeReviewArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	ref := args.Ref
	if ref == "" {
		ref = "HEAD"
	}

	diffArgs := buildDiffArgs(args)
	cmd := exec.CommandContext(ctx, "git", diffArgs...)
	if tc.Directory != "" {
		cmd.Dir = tc.Directory
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git diff failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git diff failed: %w", err)
	}

	return formatOutput(ref, string(output)), nil
}

func main() {
	plugin.Run(newPlugin())
}
