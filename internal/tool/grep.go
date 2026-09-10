package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxGrepMatches   = 500
	grepContextLines = 2
)

type grepArgs struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path,omitempty"`
	Include   string `json:"include,omitempty"`
	MaxCount  *int   `json:"max_count,omitempty"`
}

func GrepTool() *Def {
	return &Def{
		ID:          "grep",
		Description: "Search for a pattern in files. Returns matching lines with file paths and line numbers.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The regex pattern to search for",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The directory or file to search in",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "File glob pattern to include (e.g., *.go)",
				},
				"max_count": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matches to return",
				},
			},
			"required": []string{"pattern"},
		},
		Execute: executeGrep,
	}
}

func executeGrep(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args grepArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid regex pattern: %v", err), IsError: true}, nil
	}

	searchPath := tc.Directory
	if args.Path != "" {
		searchPath = resolvePath(args.Path, tc.Directory)
	}

	maxMatches := maxGrepMatches
	if args.MaxCount != nil && *args.MaxCount > 0 {
		maxMatches = *args.MaxCount
	}

	results, total := walkGrepFiles(ctx, searchPath, re, args.Include, maxMatches)

	// Sort by modification time, newest first.
	sort.Slice(results, func(i, j int) bool {
		return results[i].modTime > results[j].modTime
	})

	var matches []string
	for _, r := range results {
		for _, m := range r.matches {
			if len(matches) < maxMatches {
				matches = append(matches, m)
			}
		}
	}

	if len(matches) == 0 {
		return &ExecuteResult{Output: "No matches found."}, nil
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m)
		sb.WriteString("\n")
	}

	if total > maxMatches {
		sb.WriteString(fmt.Sprintf("\n... (%d more matches not shown)\n", total-maxMatches))
	}

	return &ExecuteResult{Output: sb.String()}, nil
}

type grepFileResult struct {
	modTime int64
	matches []string
}

func walkGrepFiles(ctx context.Context, searchPath string, re *regexp.Regexp, include string, maxMatches int) ([]grepFileResult, int) {
	var results []grepFileResult
	total := 0

	_ = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
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
		if include != "" {
			matched, _ := filepath.Match(include, info.Name())
			if !matched {
				return nil
			}
		}
		if info.Size() > 1024*1024 {
			return nil
		}
		fileMatches := grepFile(path, re, searchPath)
		if len(fileMatches) > 0 {
			results = append(results, grepFileResult{
				modTime: info.ModTime().UnixNano(),
				matches: fileMatches,
			})
			total += len(fileMatches)
		}
		if total > maxMatches*2 {
			return fmt.Errorf("too many matches")
		}
		return nil
	})
	return results, total
}

func grepFile(path string, re *regexp.Regexp, baseDir string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		relPath = path
	}

	var matches []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%s:%d:%s", relPath, lineNum, line))
		}
	}

	return matches
}
