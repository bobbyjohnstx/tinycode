package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/procenv"
)

const (
	defaultShellTimeout = 120 * time.Second
	maxShellTimeout     = 600 * time.Second
)

var secretFilePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.env\b`),
	regexp.MustCompile(`\.env\.\w+`),
	regexp.MustCompile(`\bcredentials\b`),
	regexp.MustCompile(`\.key\b`),
	regexp.MustCompile(`\.pem\b`),
}

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
	regexp.MustCompile(`(?i)\b(?:reboot|shutdown|poweroff|halt)\b`),
	regexp.MustCompile(`(?i)\b(?:oc|kubectl)\s+(?:apply|create|delete|replace|patch|scale|rollout|annotate|label|expose|set|exec|rsh|debug|cp|edit|drain|cordon|uncordon|taint)\b`),
	regexp.MustCompile(`(?i)\b(?:oc|kubectl)\s+adm\s+(?:drain|cordon|uncordon|reboot|upgrade|policy|prune|release|migrate)\b`),
	regexp.MustCompile(`(?i)\b(?:oc|kubectl)\s+config\s+(?:set|use-context|delete|rename|unset)\b`),
	regexp.MustCompile(`(?i)\bhelm\s+(?:install|upgrade|uninstall|rollback|delete)\b`),
	regexp.MustCompile(`(?i)\bsystemctl\s+(?:--\S+\s+)*(?:start|stop|restart|reload|try-restart|reload-or-restart|enable|disable|mask|unmask|kill|isolate|edit|daemon-reload|set-property)\b`),
	regexp.MustCompile(`(?i)\bservice\s+\S+\s+(?:start|stop|restart|reload)\b`),
	regexp.MustCompile(`(?i)\b(?:useradd|userdel|usermod|groupadd|groupdel|groupmod|passwd|chpasswd|visudo|chage)\b`),
	regexp.MustCompile(`(?i)\b(?:hostnamectl|timedatectl|localectl)\s+set-`),
	regexp.MustCompile(`(?i)\b(?:setenforce|setsebool)\b`),
	regexp.MustCompile(`(?i)\bfirewall-cmd\b.*(?:--add-|--remove-|--permanent|--reload|--panic-)`),
	regexp.MustCompile(`(?i)\biptables\b.*(?:-[ADIFXPZ]\b|--flush|--delete)`),
	regexp.MustCompile(`(?i)\bnft\s+(?:add|delete|flush|insert|replace)\b`),
	regexp.MustCompile(`(?i)\bnmcli\s+(?:connection|con|device|dev)\s+(?:up|down|modify|mod|delete|del|add|reapply|connect|disconnect)\b`),
	regexp.MustCompile(`(?i)\bip\s+(?:link|addr|address|route|rule)\s+(?:add|del|delete|replace|set|change|flush)\b`),
}

type shellArgs struct {
	Command     string `json:"command"`
	Timeout     *int   `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
}

func ShellTool() *Def {
	return &Def{
		ID:          "bash",
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

	if blocked := gateSecretAccess(ctx, tc, args.Command); blocked != nil {
		return blocked, nil
	}

	if IsDestructive(args.Command) {
		if tc.Perms != nil {
			askErr := tc.Perms.Ask(ctx, permission.AskInput{
				SessionID:  tc.SessionID,
				Permission: "destructive-shell",
				Patterns:   []string{args.Command},
				Metadata:   map[string]any{"command": args.Command},
				Ruleset:    tc.Ruleset,
			})
			if askErr != nil {
				return &ExecuteResult{Output: askErr.Error(), IsError: true}, nil
			}
		} else {
			return &ExecuteResult{
				Output:  fmt.Sprintf("Potentially destructive command detected: %s\nUse with caution.", args.Command),
				IsError: true,
			}, nil
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", args.Command)
	cmd.Dir = tc.Directory
	env := environToMap(procenv.Child(nil))
	if tc != nil && tc.ShellEnvHook != nil {
		if merged := tc.ShellEnvHook(tc.SessionID, tc.Directory, env); merged != nil {
			env = merged
		}
	}
	cmd.Env = flattenEnv(env)

	stdout := NewLimitedWriter(MaxOutputSize)
	stderr := NewLimitedWriter(MaxOutputSize)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	slog.Info("shell exec", "command", args.Command, "dir", tc.Directory, "timeout", timeout)
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	var output strings.Builder
	if stdout.Len() > 0 {
		output.Write(stdout.Bytes())
		if stdout.Overflow {
			output.WriteString("\n[output truncated at 10MB]")
		}
	}
	if stderr.Len() > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.Write(stderr.Bytes())
		if stderr.Overflow {
			output.WriteString("\n[stderr truncated at 10MB]")
		}
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			slog.Warn("shell timeout", "command", args.Command, "timeout", timeout, "elapsed", elapsed)
			return &ExecuteResult{
				Output:  fmt.Sprintf("Command timed out after %v\n%s", timeout, output.String()),
				IsError: true,
			}, nil
		}
		slog.Warn("shell error", "command", args.Command, "elapsed", elapsed, "error", err, "stderrLen", stderr.Len())
		exitMsg := fmt.Sprintf("Command exited with error: %v\n%s", err, output.String())
		return &ExecuteResult{Output: exitMsg, IsError: true}, nil
	}

	slog.Info("shell done", "command", args.Command, "elapsed", elapsed, "stdoutLen", stdout.Len(), "stderrLen", stderr.Len())
	return &ExecuteResult{Output: output.String()}, nil
}

// IsDestructive returns true if the command matches known destructive patterns
// (e.g. rm -rf, git push --force, DROP TABLE).
func IsDestructive(command string) bool {
	for _, p := range destructivePatterns {
		if p.MatchString(command) {
			return true
		}
	}
	return false
}

// gateSecretAccess asks before a command that names a secret file.
// A nil result means the command may run. With no permission service the
// command stays blocked, because nothing can approve it.
func gateSecretAccess(ctx context.Context, tc *Context, command string) *ExecuteResult {
	warning := CheckSecretAccess(command)
	if warning == "" {
		return nil
	}
	if tc == nil || tc.Perms == nil {
		slog.Warn("secret file access blocked", "command", command, "warning", warning)
		return &ExecuteResult{
			Output:  fmt.Sprintf("Access to secret file blocked: %s", warning),
			IsError: true,
		}
	}
	askErr := tc.Perms.Ask(ctx, permission.AskInput{
		SessionID:  tc.SessionID,
		Permission: "secret-shell",
		Patterns:   []string{command},
		Metadata:   map[string]any{"command": command},
		Ruleset:    tc.Ruleset,
	})
	if askErr != nil {
		return &ExecuteResult{Output: askErr.Error(), IsError: true}
	}
	return nil
}

// CheckSecretAccess returns a warning message if the command references
// files that commonly contain secrets.
func CheckSecretAccess(command string) string {
	for _, p := range secretFilePatterns {
		if p.MatchString(command) {
			return fmt.Sprintf("Command may access sensitive file matching pattern %q", p.String())
		}
	}
	return ""
}

func environToMap(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, e := range environ {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		m[k] = v
	}
	return m
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
