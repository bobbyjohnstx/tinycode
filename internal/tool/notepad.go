package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	maxNotepadEntries = 50
	maxNotepadValue   = 10 * 1024 // 10KB per entry
)

// NotepadTool returns a tool that provides session-scoped key-value scratch
// storage. Notes survive conversation compaction because they are stored
// outside the message list.
func NotepadTool() *Def {
	return &Def{
		ID:          "notepad",
		Description: "Read, write, list, or delete session scratch notes. Notes survive compaction. After compaction, call notepad with action 'list' to recover your working notes.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":  map[string]any{"type": "string", "enum": []string{"read", "write", "list", "delete"}, "description": "Operation to perform"},
				"key":     map[string]any{"type": "string", "description": "Note key (required for read/write/delete)"},
				"content": map[string]any{"type": "string", "description": "Note content (required for write)"},
			},
			"required": []string{"action"},
		},
		Execute: executeNotepad,
	}
}

func executeNotepad(_ context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args struct {
		Action  string `json:"action"`
		Key     string `json:"key"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if tc.Notepad == nil {
		return &ExecuteResult{Output: "Notepad storage not initialized", IsError: true}, nil
	}

	switch args.Action {
	case "write":
		if args.Key == "" {
			return &ExecuteResult{Output: "key is required for write", IsError: true}, nil
		}
		if args.Content == "" {
			return &ExecuteResult{Output: "content is required for write", IsError: true}, nil
		}
		if len(args.Content) > maxNotepadValue {
			return &ExecuteResult{
				Output:  fmt.Sprintf("content exceeds maximum size of %d bytes", maxNotepadValue),
				IsError: true,
			}, nil
		}
		// Allow updating existing keys without counting against the limit.
		if _, exists := (*tc.Notepad)[args.Key]; !exists && len(*tc.Notepad) >= maxNotepadEntries {
			return &ExecuteResult{
				Output:  fmt.Sprintf("notepad full: maximum of %d entries reached", maxNotepadEntries),
				IsError: true,
			}, nil
		}
		(*tc.Notepad)[args.Key] = args.Content
		return &ExecuteResult{Output: fmt.Sprintf("Saved note %q (%d bytes)", args.Key, len(args.Content))}, nil

	case "read":
		if args.Key == "" {
			return &ExecuteResult{Output: "key is required for read", IsError: true}, nil
		}
		content, ok := (*tc.Notepad)[args.Key]
		if !ok {
			return &ExecuteResult{Output: fmt.Sprintf("key %q not found", args.Key)}, nil
		}
		return &ExecuteResult{Output: content}, nil

	case "list":
		if len(*tc.Notepad) == 0 {
			return &ExecuteResult{Output: "No notes stored."}, nil
		}
		keys := make([]string, 0, len(*tc.Notepad))
		for k := range *tc.Notepad {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var sb strings.Builder
		fmt.Fprintf(&sb, "%d notes:\n", len(keys))
		for _, k := range keys {
			preview := firstLine((*tc.Notepad)[k])
			fmt.Fprintf(&sb, "  %s: %s\n", k, preview)
		}
		return &ExecuteResult{Output: sb.String()}, nil

	case "delete":
		if args.Key == "" {
			return &ExecuteResult{Output: "key is required for delete", IsError: true}, nil
		}
		if _, ok := (*tc.Notepad)[args.Key]; !ok {
			return &ExecuteResult{Output: fmt.Sprintf("key %q not found", args.Key)}, nil
		}
		delete(*tc.Notepad, args.Key)
		return &ExecuteResult{Output: fmt.Sprintf("Deleted note %q", args.Key)}, nil

	default:
		return &ExecuteResult{
			Output:  fmt.Sprintf("invalid action %q: must be read, write, list, or delete", args.Action),
			IsError: true,
		}, nil
	}
}

// firstLine returns the first line of s, truncated to 80 characters.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 80 {
		return line[:77] + "..."
	}
	return line
}
