package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
)

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   *int   `json:"offset,omitempty"`
	Limit    *int   `json:"limit,omitempty"`
}

func ReadTool() *Def {
	return &Def{
		ID:          "read",
		Description: "Read a file from the filesystem. Returns the file contents with line numbers.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to read",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "The line number to start reading from (0-indexed)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "The number of lines to read",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: executeRead,
	}
}

func executeRead(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args readArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	path := resolvePath(args.FilePath, tc.Directory)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			suggestion := suggestSimilar(path)
			msg := fmt.Sprintf("File not found: %s", path)
			if suggestion != "" {
				msg += fmt.Sprintf("\nDid you mean: %s?", suggestion)
			}
			return &ExecuteResult{Output: msg, IsError: true}, nil
		}
		return &ExecuteResult{Output: fmt.Sprintf("Error accessing file: %v", err), IsError: true}, nil
	}

	if info.IsDir() {
		return &ExecuteResult{Output: fmt.Sprintf("%s is a directory, not a file", path), IsError: true}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	offset := 0
	if args.Offset != nil {
		offset = *args.Offset
	}

	limit := defaultReadLimit
	if args.Limit != nil {
		limit = *args.Limit
	}

	if offset >= len(lines) {
		return &ExecuteResult{
			Output: fmt.Sprintf("Offset %d is beyond end of file (%d lines)", offset, len(lines)),
			IsError: true,
		}, nil
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	for i := offset; i < end; i++ {
		line := lines[i]
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d\t%s\n", i+1, line))
	}

	if end < len(lines) {
		sb.WriteString(fmt.Sprintf("\n... (%d more lines not shown, use offset/limit to read more)\n", len(lines)-end))
	}

	return &ExecuteResult{Output: sb.String()}, nil
}

func resolvePath(path, dir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(dir, path))
}

func suggestSimilar(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var best string
	bestScore := 0

	for _, entry := range entries {
		name := entry.Name()
		score := commonPrefixLen(strings.ToLower(base), strings.ToLower(name))
		if score > bestScore && score >= 3 {
			bestScore = score
			best = filepath.Join(dir, name)
		}
	}

	return best
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
