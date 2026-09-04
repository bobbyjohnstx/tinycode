package vcs

import (
	"os/exec"
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
	out, err := gitCommand(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var changes []Change
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		file := strings.TrimSpace(line[3:])
		changes = append(changes, Change{
			Status: status,
			File:   file,
		})
	}

	return &Status{
		Clean:   len(changes) == 0,
		Changes: changes,
	}, nil
}

// GitDiff returns the diff output for the git repo at dir.
func GitDiff(dir string) (string, error) {
	return gitCommand(dir, "diff")
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
