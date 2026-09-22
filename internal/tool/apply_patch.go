package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type applyPatchArgs struct {
	Patch string `json:"patch"`
}

func ApplyPatchTool() *Def {
	return &Def{
		ID:          "apply_patch",
		Description: "Apply a unified diff patch to one or more files. All changes are applied atomically — if any hunk fails, no files are modified.",
		Permission:  "edit",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "The unified diff content",
				},
			},
			"required": []string{"patch"},
		},
		Execute: executeApplyPatch,
	}
}

type diffLine struct {
	op   byte   // ' ', '+', '-'
	text string // line content without the op prefix
}

type hunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []diffLine
}

type filePatch struct {
	oldPath string // "a/..." or "/dev/null"
	newPath string // "b/..." or "/dev/null"
	hunks   []hunk
}

func executeApplyPatch(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args applyPatchArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if strings.TrimSpace(args.Patch) == "" {
		return &ExecuteResult{Output: "patch is empty", IsError: true}, nil
	}

	patches, err := parsePatch(args.Patch)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to parse patch: %v", err), IsError: true}, nil
	}

	if len(patches) == 0 {
		return &ExecuteResult{Output: "No file patches found in diff", IsError: true}, nil
	}

	// Phase 1: Read all files and compute new contents in memory
	type fileResult struct {
		path       string
		newContent string
		isNew      bool
		isDelete   bool
	}
	results := make([]fileResult, 0, len(patches))

	for _, fp := range patches {
		targetPath := patchTargetPath(fp, tc.Directory)
		isNew := fp.oldPath == "/dev/null"
		isDelete := fp.newPath == "/dev/null"

		var original string
		if !isNew {
			mu := getFileMutex(targetPath)
			mu.Lock()
			defer mu.Unlock()

			data, readErr := os.ReadFile(targetPath)
			if readErr != nil {
				if os.IsNotExist(readErr) {
					return &ExecuteResult{Output: fmt.Sprintf("File not found: %s", targetPath), IsError: true}, nil
				}
				return &ExecuteResult{Output: fmt.Sprintf("Error reading file %s: %v", targetPath, readErr), IsError: true}, nil
			}
			original = string(data)
		}

		if isDelete {
			results = append(results, fileResult{path: targetPath, isDelete: true})
			continue
		}

		newContent, applyErr := applyHunks(original, fp.hunks)
		if applyErr != nil {
			return &ExecuteResult{
				Output:  fmt.Sprintf("Failed to apply patch to %s: %v", targetPath, applyErr),
				IsError: true,
			}, nil
		}

		results = append(results, fileResult{path: targetPath, newContent: newContent, isNew: isNew})
	}

	// Phase 2: All hunks applied successfully — write all files
	for _, r := range results {
		if r.isDelete {
			if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
				return &ExecuteResult{Output: fmt.Sprintf("Error deleting file %s: %v", r.path, err), IsError: true}, nil
			}
			if tc.Bus != nil {
				tc.Bus.Publish("file.modified", map[string]any{
					"sessionID": tc.SessionID,
					"path":      r.path,
					"operation": "delete",
				})
			}
			continue
		}

		if r.isNew {
			dir := filepath.Dir(r.path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return &ExecuteResult{Output: fmt.Sprintf("Error creating directory for %s: %v", r.path, err), IsError: true}, nil
			}
		}

		if err := os.WriteFile(r.path, []byte(r.newContent), 0644); err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Error writing file %s: %v", r.path, err), IsError: true}, nil
		}

		if tc.Bus != nil {
			op := "edit"
			if r.isNew {
				op = "create"
			}
			tc.Bus.Publish("file.modified", map[string]any{
				"sessionID": tc.SessionID,
				"path":      r.path,
				"operation": op,
			})
		}
	}

	var summary strings.Builder
	for i, r := range results {
		if i > 0 {
			summary.WriteString("\n")
		}
		if r.isDelete {
			summary.WriteString(fmt.Sprintf("Deleted %s", r.path))
		} else if r.isNew {
			summary.WriteString(fmt.Sprintf("Created %s", r.path))
		} else {
			summary.WriteString(fmt.Sprintf("Modified %s", r.path))
		}
	}
	return &ExecuteResult{Output: summary.String()}, nil
}

// patchTargetPath determines the absolute filesystem path for a filePatch.
func patchTargetPath(fp filePatch, dir string) string {
	var raw string
	if fp.newPath != "/dev/null" {
		raw = fp.newPath
	} else {
		raw = fp.oldPath
	}
	// Strip a/ or b/ prefix from unified diff paths
	raw = stripDiffPrefix(raw)
	return resolvePath(raw, dir)
}

// stripDiffPrefix removes the leading "a/" or "b/" from a unified diff path.
func stripDiffPrefix(p string) string {
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// parsePatch parses unified diff text into a slice of filePatch.
func parsePatch(text string) ([]filePatch, error) {
	text = strings.TrimRight(text, "\n")
	lines := strings.Split(text, "\n")
	var patches []filePatch
	i := 0

	for i < len(lines) {
		// Skip until we find a --- line
		if !strings.HasPrefix(lines[i], "--- ") {
			i++
			continue
		}

		oldPath := strings.TrimPrefix(lines[i], "--- ")
		i++
		if i >= len(lines) || !strings.HasPrefix(lines[i], "+++ ") {
			return nil, fmt.Errorf("expected +++ line after --- at line %d", i)
		}
		newPath := strings.TrimPrefix(lines[i], "+++ ")
		i++

		fp := filePatch{oldPath: oldPath, newPath: newPath}

		// Parse hunks for this file
		for i < len(lines) && strings.HasPrefix(lines[i], "@@") {
			h, nextI, err := parseHunk(lines, i)
			if err != nil {
				return nil, err
			}
			fp.hunks = append(fp.hunks, h)
			i = nextI
		}

		if len(fp.hunks) == 0 {
			return nil, fmt.Errorf("no hunks found for %s", newPath)
		}

		patches = append(patches, fp)
	}

	return patches, nil
}

// parseHunk parses a single hunk starting at lines[start] which must be a @@ line.
func parseHunk(lines []string, start int) (hunk, int, error) {
	header := lines[start]
	h, err := parseHunkHeader(header)
	if err != nil {
		return hunk{}, 0, fmt.Errorf("line %d: %w", start+1, err)
	}

	i := start + 1
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "--- ") {
			break
		}
		if len(line) == 0 {
			// Empty line in diff = context line with empty content
			h.lines = append(h.lines, diffLine{op: ' ', text: ""})
			i++
			continue
		}
		op := line[0]
		switch op {
		case ' ', '+', '-':
			h.lines = append(h.lines, diffLine{op: op, text: line[1:]})
		case '\\':
			// "\ No newline at end of file" — skip
			i++
			continue
		default:
			// Treat as context line (some diffs omit the leading space)
			h.lines = append(h.lines, diffLine{op: ' ', text: line})
		}
		i++
	}

	return h, i, nil
}

// parseHunkHeader parses a "@@ -old,count +new,count @@" header.
func parseHunkHeader(header string) (hunk, error) {
	// Format: @@ -start[,count] +start[,count] @@[ optional section heading]
	if !strings.HasPrefix(header, "@@") {
		return hunk{}, fmt.Errorf("invalid hunk header: %s", header)
	}

	// Find the range between the first @@ and the closing @@
	rest := strings.TrimPrefix(header, "@@")
	closingIdx := strings.Index(rest, "@@")
	if closingIdx < 0 {
		return hunk{}, fmt.Errorf("invalid hunk header (no closing @@): %s", header)
	}
	rangePart := strings.TrimSpace(rest[:closingIdx])

	parts := strings.Fields(rangePart)
	if len(parts) < 2 {
		return hunk{}, fmt.Errorf("invalid hunk header range: %s", header)
	}

	oldStart, oldCount, err := parseRange(parts[0], '-')
	if err != nil {
		return hunk{}, fmt.Errorf("invalid old range in %s: %w", header, err)
	}

	newStart, newCount, err := parseRange(parts[1], '+')
	if err != nil {
		return hunk{}, fmt.Errorf("invalid new range in %s: %w", header, err)
	}

	return hunk{
		oldStart: oldStart,
		oldCount: oldCount,
		newStart: newStart,
		newCount: newCount,
	}, nil
}

// parseRange parses "-start,count" or "+start,count" (count defaults to 1 if omitted).
func parseRange(s string, prefix byte) (int, int, error) {
	if len(s) == 0 || s[0] != prefix {
		return 0, 0, fmt.Errorf("expected %c prefix in %q", prefix, s)
	}
	s = s[1:]
	parts := strings.SplitN(s, ",", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start: %w", err)
	}
	count := 1
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid count: %w", err)
		}
	}
	return start, count, nil
}

// applyHunks applies hunks to the original content and returns the new content.
// Hunks must be in order. Returns an error if context lines don't match.
func applyHunks(original string, hunks []hunk) (string, error) {
	lines := splitLines(original)
	offset := 0 // accumulated line offset from previous hunks

	for i, h := range hunks {
		// oldStart is 1-based; convert to 0-based index, adjusted by offset
		startIdx := h.oldStart - 1 + offset
		if h.oldStart == 0 && h.oldCount == 0 {
			// Special case: adding to empty file
			startIdx = 0
		}

		// Verify context and removal lines match
		lineIdx := startIdx
		for _, dl := range h.lines {
			if dl.op == '+' {
				continue
			}
			// Context or removal line — must match original
			if lineIdx < 0 || lineIdx >= len(lines) {
				return "", fmt.Errorf("hunk %d: line %d out of range (file has %d lines)", i+1, lineIdx+1, len(lines))
			}
			if lines[lineIdx] != dl.text {
				return "", fmt.Errorf("hunk %d: context mismatch at line %d: expected %q, got %q", i+1, lineIdx+1, dl.text, lines[lineIdx])
			}
			lineIdx++
		}

		// Build the replacement lines for this hunk
		var newLines []string
		oldLineCount := 0
		for _, dl := range h.lines {
			switch dl.op {
			case ' ':
				newLines = append(newLines, dl.text)
				oldLineCount++
			case '+':
				newLines = append(newLines, dl.text)
			case '-':
				oldLineCount++
			}
		}

		// Splice: remove old lines, insert new lines
		result := make([]string, 0, len(lines)-oldLineCount+len(newLines))
		result = append(result, lines[:startIdx]...)
		result = append(result, newLines...)
		result = append(result, lines[startIdx+oldLineCount:]...)
		lines = result

		offset += len(newLines) - oldLineCount
	}

	return joinLines(lines), nil
}

// splitLines splits content into lines, preserving the semantic that a trailing
// newline means the last element is NOT an empty string (matching diff behavior).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	// Remove trailing newline for splitting so we don't get an extra empty element
	trimmed := strings.TrimSuffix(s, "\n")
	return strings.Split(trimmed, "\n")
}

// joinLines joins lines back with newlines, adding a trailing newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
