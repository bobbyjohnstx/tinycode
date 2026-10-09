package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
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

	if err := writeFileSync(path, []byte(args.Content), 0644); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}
	note := formatNote(ctx, tc, path)

	if tc.Bus != nil {
		tc.Bus.Publish("file.modified", map[string]any{
			"sessionID": tc.SessionID,
			"path":      path,
			"operation": "write",
		})
	}

	output := fmt.Sprintf("Successfully wrote %d bytes to %s", len(args.Content), path) + note
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

// writeFileSync writes data to a file atomically with fsync. It writes to a
// temp file in the same directory, calls Sync to flush to disk, then renames
// over the target path.
func writeFileSync(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
