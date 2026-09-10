package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxGlobResults = 1000

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func GlobTool() *Def {
	return &Def{
		ID:          "glob",
		Description: "Find files matching a glob pattern. Returns matching file paths relative to the search directory.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The glob pattern to match (e.g., **/*.go, src/**/*.ts)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The directory to search in",
				},
			},
			"required": []string{"pattern"},
		},
		Execute: executeGlob,
	}
}

func executeGlob(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args globArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	searchPath := tc.Directory
	if args.Path != "" {
		searchPath = resolvePath(args.Path, tc.Directory)
	}

	var matches []string
	pattern := args.Pattern

	isDoubleGlob := strings.Contains(pattern, "**")

	if isDoubleGlob {
		var err error
		matches, err = walkDoubleGlob(ctx, searchPath, pattern)
		if err != nil && len(matches) == 0 {
			return &ExecuteResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}, nil
		}
	} else {
		fullPattern := filepath.Join(searchPath, pattern)
		globMatches, err := filepath.Glob(fullPattern)
		if err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Invalid glob pattern: %v", err), IsError: true}, nil
		}
		for _, m := range globMatches {
			rel, err := filepath.Rel(searchPath, m)
			if err != nil {
				continue
			}
			matches = append(matches, rel)
		}
	}

	// Sort by modification time, newest first.
	sort.Slice(matches, func(i, j int) bool {
		iInfo, iErr := os.Stat(filepath.Join(searchPath, matches[i]))
		jInfo, jErr := os.Stat(filepath.Join(searchPath, matches[j]))
		if iErr != nil || jErr != nil {
			return matches[i] < matches[j]
		}
		return iInfo.ModTime().After(jInfo.ModTime())
	})

	if len(matches) == 0 {
		return &ExecuteResult{Output: "No files matched the pattern."}, nil
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m)
		sb.WriteString("\n")
	}

	if len(matches) >= maxGlobResults {
		sb.WriteString(fmt.Sprintf("\n... (results capped at %d)\n", maxGlobResults))
	}

	return &ExecuteResult{Output: sb.String()}, nil
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".tinycode": true,
}

func walkDoubleGlob(ctx context.Context, searchPath, pattern string) ([]string, error) {
	parts := strings.SplitN(pattern, "**", 2)
	prefix := parts[0]
	suffix := ""
	if len(parts) > 1 {
		suffix = strings.TrimPrefix(parts[1], "/")
	}

	root := filepath.Join(searchPath, prefix)
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if suffix != "" {
			relFromRoot, _ := filepath.Rel(root, path)
			if !matchSuffix(suffix, relFromRoot) {
				return nil
			}
		}
		relPath, err := filepath.Rel(searchPath, path)
		if err != nil {
			return nil
		}
		matches = append(matches, relPath)
		if len(matches) > maxGlobResults {
			return fmt.Errorf("too many results")
		}
		return nil
	})
	return matches, err
}

// matchSuffix tries filepath.Match(suffix, subpath) against progressively
// shorter subpaths of rel so that "src/*.go" matches "foo/src/main.go".
// Paths are normalized to forward slashes for consistent matching across platforms.
func matchSuffix(suffix, rel string) bool {
	rel = filepath.ToSlash(rel)
	for rel != "" {
		if matched, _ := filepath.Match(suffix, rel); matched {
			return true
		}
		i := strings.IndexByte(rel, '/')
		if i < 0 {
			break
		}
		rel = rel[i+1:]
	}
	return false
}
