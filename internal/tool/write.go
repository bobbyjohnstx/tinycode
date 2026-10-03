package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func WriteTool() *Def {
	return &Def{
		ID:          "write",
		Description: "Write content to a file. Creates the file if it doesn't exist, or overwrites if it does.",
		Permission:  "edit",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			"required": []string{"file_path", "content"},
		},
		Execute: executeWrite,
	}
}

func executeWrite(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args writeArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	path := resolvePath(args.FilePath, tc.Directory)

	// Check if file exists but hasn't been read or edited.
	var warnUnread bool
	if _, statErr := os.Stat(path); statErr == nil {
		if !tc.ReadFiles.Has(path) {
			warnUnread = true
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error creating directory: %v", err), IsError: true}, nil
	}

	existing, err := os.ReadFile(path)
	if err == nil {
		content := preserveLineEndings(string(existing), args.Content)
		args.Content = content
	}

	if err := os.WriteFile(path, []byte(args.Content), 0644); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}

	if tc.Bus != nil {
		tc.Bus.Publish("file.modified", map[string]any{
			"sessionID": tc.SessionID,
			"path":      path,
			"operation": "write",
		})
	}

	output := fmt.Sprintf("Successfully wrote %d bytes to %s", len(args.Content), path)
	if warnUnread {
		output = "WARNING: You have not read this file. The write may be based on incorrect assumptions about the file's contents.\n\n" + output
	}

	return &ExecuteResult{Output: output}, nil
}

func preserveLineEndings(existing, newContent string) string {
	hasCRLF := len(existing) > 0 && containsCRLF(existing)
	if hasCRLF && !containsCRLF(newContent) {
		return toCRLF(newContent)
	}
	return newContent
}

func containsCRLF(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\r' && s[i+1] == '\n' {
			return true
		}
	}
	return false
}

func toCRLF(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			result = append(result, '\r')
		}
		result = append(result, s[i])
	}
	return string(result)
}
