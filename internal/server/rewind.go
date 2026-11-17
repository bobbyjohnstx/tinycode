package server

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

// readOnlyPrefixes lists shell commands that never mutate the filesystem.
// A command whose first token matches one of these is classified as read-only.
var readOnlyPrefixes = []string{
	"ls", "cat", "head", "tail", "grep", "egrep", "fgrep", "rg",
	"wc", "diff", "file", "stat", "readlink", "which", "whereis",
	"type", "echo", "printf", "true", "false", "test", "[",
	"pwd", "whoami", "hostname", "uname", "date", "env", "printenv",
	"id", "uptime", "ps", "top", "htop", "free", "df", "du",
	"man", "help", "info",
	"git status", "git log", "git diff", "git show", "git branch",
	"git remote", "git tag", "git rev-parse", "git describe",
	"git ls-files", "git ls-tree", "git cat-file", "git blame",
	"go doc", "go version", "go env", "go list",
}

// redirectRe matches output redirects (> or >>) that turn a read-only
// command into a write.
var redirectRe = regexp.MustCompile(`(?:^|[^>&])(>>?)\s+[^\s]`)

// isReadOnly returns true when the command is known to be side-effect-free.
func isReadOnly(command string) bool {
	// A redirect turns any command into a write.
	if redirectRe.MatchString(command) {
		return false
	}

	trimmed := strings.TrimSpace(command)
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(trimmed, prefix+" ") || trimmed == prefix {
			return true
		}
	}

	// find without -exec/-delete is read-only
	if strings.HasPrefix(trimmed, "find ") &&
		!strings.Contains(trimmed, "-exec") &&
		!strings.Contains(trimmed, "-delete") {
		return true
	}

	return false
}

// shellMutationInfo describes a shell mutation detected in a deleted message.
type shellMutationInfo struct {
	Paths   []string `json:"paths,omitempty"`
	Unknown bool     `json:"unknown,omitempty"`
}

// extractUnrestoredPaths scans messages for bash tool calls and returns
// paths that may have been written by shell mutations. Commands that are
// clearly read-only (grep, cat, ls, etc.) are excluded. Unclassifiable
// commands that are not read-only produce an "unknown" entry.
func extractUnrestoredPaths(messages []session.Message) []string {
	seen := make(map[string]bool)
	hasUnknown := false

	for i := range messages {
		msg := &messages[i]
		for _, part := range msg.Parts {
			if part.Type != session.PartToolCall || part.ToolName != "bash" {
				continue
			}

			command := extractCommand(part.ToolArgs)
			if command == "" {
				continue
			}

			if isReadOnly(command) {
				continue
			}

			isMut, paths := tool.ClassifyMutation(command)
			if isMut {
				for _, p := range paths {
					if p == "unknown" {
						hasUnknown = true
					} else if !seen[p] {
						seen[p] = true
					}
				}
			} else {
				// Not classified as mutation, not read-only → unknown
				hasUnknown = true
			}
		}
	}

	var result []string
	for p := range seen {
		result = append(result, p)
	}
	if hasUnknown {
		result = append(result, "unknown shell changes")
	}
	return result
}

// extractCommand pulls the "command" field from bash tool call JSON args.
func extractCommand(argsJSON string) string {
	if argsJSON == "" {
		return ""
	}
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return args.Command
}
