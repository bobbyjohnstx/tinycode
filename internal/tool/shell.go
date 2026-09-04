package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	defaultShellTimeout = 120 * time.Second
	maxShellTimeout     = 600 * time.Second
)

var destructivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-[rRf]+\s+|--recursive)`),
	regexp.MustCompile(`\brm\s+-[^\s]*[rR]`),
	regexp.MustCompile(`\bgit\s+(push\s+--force|reset\s+--hard|clean\s+-[^\s]*f)`),
	regexp.MustCompile(`(?i)\bgit\s+branch\s+-D\b`),
	regexp.MustCompile(`(?i)\bdrop\s+(table|database)\b`),
	regexp.MustCompile(`(?i)\btruncate\s+table\b`),
	regexp.MustCompile(`\bkill\s+-9\b`),
	regexp.MustCompile(`\bmkfs\b`),
	regexp.MustCompile(`\bdd\s+`),
	regexp.MustCompile(`>\s*/dev/sd`),
}

type shellArgs struct {
	Command     string `json:"command"`
	Timeout     *int   `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
}

func ShellTool() *Def {
	return &Def{
		ID:          "shell",
		Description: "Execute a shell command and return its output.",
		Permission:  "shell",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds (max 600000)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Description of what this command does",
				},
			},
			"required": []string{"command"},
		},
		Execute: executeShell,
	}
}

func executeShell(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args shellArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Command == "" {
		return &ExecuteResult{Output: "Command cannot be empty", IsError: true}, nil
	}

	timeout := defaultShellTimeout
	if args.Timeout != nil {
		t := time.Duration(*args.Timeout) * time.Millisecond
		if t > maxShellTimeout {
			t = maxShellTimeout
		}
		if t > 0 {
			timeout = t
		}
	}

	destructive := isDestructive(args.Command)
	if destructive {
		return &ExecuteResult{
			Output:  fmt.Sprintf("Potentially destructive command detected: %s\nUse with caution.", args.Command),
			IsError: true,
		}, nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", args.Command)
	cmd.Dir = tc.Directory

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var output strings.Builder
	if stdout.Len() > 0 {
		output.Write(stdout.Bytes())
	}
	if stderr.Len() > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.Write(stderr.Bytes())
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return &ExecuteResult{
				Output:  fmt.Sprintf("Command timed out after %v\n%s", timeout, output.String()),
				IsError: true,
			}, nil
		}
		exitMsg := fmt.Sprintf("Command exited with error: %v\n%s", err, output.String())
		return &ExecuteResult{Output: exitMsg, IsError: true}, nil
	}

	return &ExecuteResult{Output: output.String()}, nil
}

func isDestructive(command string) bool {
	for _, p := range destructivePatterns {
		if p.MatchString(command) {
			return true
		}
	}
	return false
}
