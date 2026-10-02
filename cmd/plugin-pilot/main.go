package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func formatIssueList(issues []NormalizedIssue, owner, repo, state string) string {
	if len(issues) == 0 {
		return fmt.Sprintf("No issues found for %s/%s (%s)", owner, repo, state)
	}

	lines := make([]string, len(issues))
	for i, issue := range issues {
		labels := "none"
		if len(issue.Labels) > 0 {
			labels = strings.Join(issue.Labels, ", ")
		}
		date := issue.CreatedAt
		if idx := strings.Index(date, "T"); idx >= 0 {
			date = date[:idx]
		}
		lines[i] = fmt.Sprintf("%d. **#%d** %s — %s | Labels: %s | Created: %s",
			i+1, issue.Number, issue.Title, issue.State, labels, date)
	}

	return fmt.Sprintf("## Issues for %s/%s (%s)\n\n%s", owner, repo, state, strings.Join(lines, "\n"))
}

func buildTools(provider IssueProvider) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "pilot_issues_list",
			Description: "List issues in a repository with optional state and label filters",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":  map[string]any{"type": "string", "description": "Repository owner"},
					"repo":   map[string]any{"type": "string", "description": "Repository name"},
					"state":  map[string]any{"type": "string", "enum": []string{"open", "closed"}, "description": "Issue state filter (default: open)"},
					"labels": map[string]any{"type": "string", "description": "Comma-separated label filter"},
					"page":   map[string]any{"type": "number", "description": "Page number (default: 1)"},
				},
				"required": []string{"owner", "repo"},
			},
			Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
				var args struct {
					Owner  string `json:"owner"`
					Repo   string `json:"repo"`
					State  string `json:"state"`
					Labels string `json:"labels"`
					Page   int    `json:"page"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				state := args.State
				if state == "" {
					state = "open"
				}

				issues, err := provider.ListIssues(ListIssuesParams{
					Owner:  args.Owner,
					Repo:   args.Repo,
					State:  state,
					Labels: args.Labels,
					Page:   args.Page,
					Limit:  20,
				})
				if err != nil {
					return err.Error(), nil
				}
				return formatIssueList(issues, args.Owner, args.Repo, state), nil
			},
		},
		{
			Name:        "pilot_issue_create",
			Description: "Create a new issue in a repository",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":  map[string]any{"type": "string", "description": "Repository owner"},
					"repo":   map[string]any{"type": "string", "description": "Repository name"},
					"title":  map[string]any{"type": "string", "description": "Issue title"},
					"body":   map[string]any{"type": "string", "description": "Issue body"},
					"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Label names"},
				},
				"required": []string{"owner", "repo", "title"},
			},
			Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
				var args struct {
					Owner  string   `json:"owner"`
					Repo   string   `json:"repo"`
					Title  string   `json:"title"`
					Body   string   `json:"body"`
					Labels []string `json:"labels"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				issue, err := provider.CreateIssue(CreateIssueParams{
					Owner:  args.Owner,
					Repo:   args.Repo,
					Title:  args.Title,
					Body:   args.Body,
					Labels: args.Labels,
				})
				if err != nil {
					return err.Error(), nil
				}
				return fmt.Sprintf("Created issue #%d: %s\nURL: %s", issue.Number, issue.Title, issue.URL), nil
			},
		},
		{
			Name:        "pilot_issue_update",
			Description: "Update an existing issue in a repository",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":        map[string]any{"type": "string", "description": "Repository owner"},
					"repo":         map[string]any{"type": "string", "description": "Repository name"},
					"issue_number": map[string]any{"type": "number", "description": "Issue number"},
					"title":        map[string]any{"type": "string", "description": "New title"},
					"body":         map[string]any{"type": "string", "description": "New body"},
					"state":        map[string]any{"type": "string", "enum": []string{"open", "closed"}, "description": "New state"},
				},
				"required": []string{"owner", "repo", "issue_number"},
			},
			Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
				var args struct {
					Owner       string `json:"owner"`
					Repo        string `json:"repo"`
					IssueNumber int    `json:"issue_number"`
					Title       string `json:"title"`
					Body        string `json:"body"`
					State       string `json:"state"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				issue, err := provider.UpdateIssue(UpdateIssueParams{
					Owner:       args.Owner,
					Repo:        args.Repo,
					IssueNumber: args.IssueNumber,
					Title:       args.Title,
					Body:        args.Body,
					State:       args.State,
				})
				if err != nil {
					return err.Error(), nil
				}
				return fmt.Sprintf("Updated issue #%d: %s", issue.Number, issue.Title), nil
			},
		},
		{
			Name:        "pilot_issue_comment",
			Description: "Add a comment to an issue in a repository",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":        map[string]any{"type": "string", "description": "Repository owner"},
					"repo":         map[string]any{"type": "string", "description": "Repository name"},
					"issue_number": map[string]any{"type": "number", "description": "Issue number"},
					"body":         map[string]any{"type": "string", "description": "Comment body"},
				},
				"required": []string{"owner", "repo", "issue_number", "body"},
			},
			Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
				var args struct {
					Owner       string `json:"owner"`
					Repo        string `json:"repo"`
					IssueNumber int    `json:"issue_number"`
					Body        string `json:"body"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				_, err := provider.CommentOnIssue(CommentParams{
					Owner:       args.Owner,
					Repo:        args.Repo,
					IssueNumber: args.IssueNumber,
					Body:        args.Body,
				})
				if err != nil {
					return err.Error(), nil
				}
				return fmt.Sprintf("Comment added to issue #%d", args.IssueNumber), nil
			},
		},
	}
}

func newPlugin(provider IssueProvider) plugin.Plugin {
	return plugin.Plugin{
		ID:    "pilot",
		Tools: buildTools(provider),
	}
}

func main() {
	provider := createProvider(".")
	plugin.Run(newPlugin(provider))
}
