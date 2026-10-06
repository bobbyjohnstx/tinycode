package vcs

import (
	"os/exec"
	"strconv"
	"strings"
)

// Info holds VCS repository metadata.
type Info struct {
	Type   string `json:"type"`
	Branch string `json:"branch"`
	Remote string `json:"remote"`
}

// Change represents a single file change in the working tree.
type Change struct {
	Status string `json:"status"`
	File   string `json:"file"`
}

// Status holds the working tree status.
type Status struct {
	Clean   bool     `json:"clean"`
	Changes []Change `json:"changes"`
}

// GitInfo returns repository metadata for the git repo at dir.
func GitInfo(dir string) (*Info, error) {
	branch, err := gitCommand(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}

	remote, _ := gitCommand(dir, "config", "--get", "remote.origin.url")

	return &Info{
		Type:   "git",
		Branch: branch,
		Remote: remote,
	}, nil
}

// GitStatus returns the working tree status for the git repo at dir.
func GitStatus(dir string) (*Status, error) {
	out, err := gitCommandRaw(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var changes []Change
	for _, line := range strings.Split(out, "\n") {
		if c, ok := parsePorcelainLine(line); ok {
			changes = append(changes, c)
		}
	}

	return &Status{
		Clean:   len(changes) == 0,
		Changes: changes,
	}, nil
}

// parsePorcelainLine parses one git status --porcelain line into a Change.
// Rename/copy lines (R/C) use the destination path. Quoted paths are unquoted.
func parsePorcelainLine(line string) (Change, bool) {
	if len(line) < 4 {
		return Change{}, false
	}
	status := strings.TrimSpace(line[:2])
	pathPart := line[3:]

	// Rename/copy: "R  old -> new" or "R  \"old\" -> \"new\""
	if len(status) > 0 && (status[0] == 'R' || status[0] == 'C') {
		if dest, ok := renameDestination(pathPart); ok {
			return Change{Status: status, File: dest}, true
		}
	}

	file, ok := unquotePath(pathPart)
	if !ok || file == "" {
		return Change{}, false
	}
	return Change{Status: status, File: file}, true
}

func renameDestination(pathPart string) (string, bool) {
	// Split on " -> " outside of quotes when possible; porcelain always uses " -> ".
	const sep = " -> "
	idx := strings.LastIndex(pathPart, sep)
	if idx < 0 {
		return "", false
	}
	return unquotePath(pathPart[idx+len(sep):])
}

// unquotePath handles git's C-style quoted paths (spaces, special chars).
func unquotePath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if !strings.HasPrefix(s, `"`) {
		return s, true
	}
	// Use strconv.Unquote for C-style escapes git emits.
	unquoted, err := strconv.Unquote(s)
	if err != nil {
		// Fallback: strip surrounding quotes only.
		if len(s) >= 2 && strings.HasSuffix(s, `"`) {
			return s[1 : len(s)-1], true
		}
		return s, true
	}
	return unquoted, true
}

// GitDiff returns the staged+unstaged diff for the git repo at dir
// (equivalent to git diff HEAD, matching the TUI /diff command).
func GitDiff(dir string) (string, error) {
	return gitCommand(dir, "diff", "HEAD")
}

// GitDiffNumstat returns the files touched by the staged+unstaged diff and the
// summed added and deleted line counts. It does not load the patch.
func GitDiffNumstat(dir string) (files []string, additions, deletions int, err error) {
	out, err := gitCommand(dir, "diff", "HEAD", "--numstat")
	if err != nil {
		return nil, 0, 0, err
	}
	files, additions, deletions = parseNumstat(out)
	return files, additions, deletions, nil
}

func parseNumstat(out string) (files []string, additions, deletions int) {
	files = []string{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		if n, err := strconv.Atoi(parts[0]); err == nil {
			additions += n
		}
		if n, err := strconv.Atoi(parts[1]); err == nil {
			deletions += n
		}
		files = append(files, parts[2])
	}
	return files, additions, deletions
}

func gitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCommandRaw(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}
