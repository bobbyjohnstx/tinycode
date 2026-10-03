package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

const fileMutexCleanupThreshold = 1000

type fileMutexMap struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

var fileMutexes = fileMutexMap{m: make(map[string]*sync.Mutex)}

func (fm *fileMutexMap) Get(path string) *sync.Mutex {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if mu, ok := fm.m[path]; ok {
		return mu
	}

	if len(fm.m) > fileMutexCleanupThreshold {
		fm.cleanup()
	}

	mu := &sync.Mutex{}
	fm.m[path] = mu
	return mu
}

func (fm *fileMutexMap) cleanup() {
	for path, mu := range fm.m {
		if mu.TryLock() {
			delete(fm.m, path)
			mu.Unlock()
		}
	}
}

type editArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func EditTool() *Def {
	return &Def{
		ID:          "edit",
		Description: "Perform exact string replacement in a file. The old_string must match exactly once in the file unless replace_all is true.",
		Permission:  "edit",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to modify",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "The text to replace",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "The replacement text",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences",
				},
			},
			"required": []string{"file_path", "old_string", "new_string"},
		},
		Execute: executeEdit,
	}
}

func executeEdit(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args editArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if args.OldString == args.NewString {
		return &ExecuteResult{Output: "old_string and new_string are identical", IsError: true}, nil
	}

	path := resolvePath(args.FilePath, tc.Directory)

	mu := getFileMutex(path)
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ExecuteResult{Output: fmt.Sprintf("File not found: %s", path), IsError: true}, nil
		}
		return &ExecuteResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	content := string(data)

	newContent, strategy, count, ok := cascadeReplace(content, args.OldString, args.NewString, args.ReplaceAll)
	if !ok {
		if count > 1 {
			return &ExecuteResult{
				Output: fmt.Sprintf("old_string appears %d times in %s. Use replace_all or provide more context to make the match unique.", count, path),
				IsError: true,
			}, nil
		}
		fuzzyMatch := fuzzyFind(content, args.OldString)
		msg := fmt.Sprintf("old_string not found in %s", path)
		if fuzzyMatch != "" {
			msg += fmt.Sprintf("\n\nDid you mean:\n%s", fuzzyMatch)
		}
		return &ExecuteResult{Output: msg, IsError: true}, nil
	}
	_ = strategy

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}

	if tc.Bus != nil {
		tc.Bus.Publish("file.modified", map[string]any{
			"sessionID": tc.SessionID,
			"path":      path,
			"operation": "edit",
		})
	}

	tc.ReadFiles.Mark(path)

	return &ExecuteResult{
		Output: fmt.Sprintf("Replaced in %s", path),
	}, nil
}

func getFileMutex(path string) *sync.Mutex {
	return fileMutexes.Get(path)
}

// ClearFileMutexes removes all entries from the file mutex map.
// Call at session boundaries to prevent unbounded growth.
func ClearFileMutexes() {
	fileMutexes.mu.Lock()
	defer fileMutexes.mu.Unlock()
	fileMutexes.m = make(map[string]*sync.Mutex)
}

func fuzzyFind(content, needle string) string {
	needleLines := strings.Split(strings.TrimSpace(needle), "\n")
	if len(needleLines) == 0 {
		return ""
	}

	contentLines := strings.Split(content, "\n")
	first := strings.TrimSpace(needleLines[0])
	if first == "" {
		return ""
	}

	for i, line := range contentLines {
		if strings.Contains(strings.TrimSpace(line), first) {
			start := i
			end := i + len(needleLines)
			if end > len(contentLines) {
				end = len(contentLines)
			}
			var sb strings.Builder
			for j := start; j < end; j++ {
				sb.WriteString(fmt.Sprintf("%d\t%s\n", j+1, contentLines[j]))
			}
			return sb.String()
		}
	}

	return ""
}
