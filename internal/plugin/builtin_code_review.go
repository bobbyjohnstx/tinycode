package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type codeReviewPlugin struct{}

// NewCodeReviewPlugin creates a builtin plugin that provides a git diff tool
// formatted as a markdown code review block.
func NewCodeReviewPlugin() BuiltinPlugin {
	return &codeReviewPlugin{}
}

func (p *codeReviewPlugin) ID() string { return "code-review" }

func (p *codeReviewPlugin) Tools() []BuiltinTool {
	return []BuiltinTool{
		{
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
			Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var args struct {
					Ref          string `json:"ref"`
					Path         string `json:"path"`
					Staged       bool   `json:"staged"`
					ContextLines *int   `json:"context_lines"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}

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

				cmd := exec.CommandContext(ctx, "git", diffArgs...)
				if dir := DirectoryFromContext(ctx); dir != "" {
					cmd.Dir = dir
				}
				output, err := cmd.Output()
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						return "", fmt.Errorf("git diff failed: %s", string(exitErr.Stderr))
					}
					return "", fmt.Errorf("git diff failed: %w", err)
				}

				diff := string(output)
				if diff == "" {
					return "No changes found.", nil
				}

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
				return sb.String(), nil
			},
		},
	}
}

func (p *codeReviewPlugin) Hooks() BuiltinHooks { return BuiltinHooks{} }
