package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/procenv"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
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

// mutationPatterns detect shell commands that write to files via redirection,
// tee, in-place edit, etc. This is a best-effort heuristic for permission
// gating and rewind tracking — NOT a security boundary. Determined attackers
// or unusual shell constructs can bypass these patterns.
var mutationPatterns = []struct {
	re       *regexp.Regexp
	pathFunc func(cmd string) []string // extract destination paths; nil → "unknown"
}{
	// > or >> redirecting to a file.
	// The \s+ after >>? ensures >&N and 2>&1 (no space) are not matched.
	{
		re: regexp.MustCompile(`(>>?)\s+([^\s;&|]+)`),
		pathFunc: func(cmd string) []string {
			re := regexp.MustCompile(`(>>?)\s+([^\s;&|]+)`)
			matches := re.FindAllStringSubmatch(cmd, -1)
			var paths []string
			for _, m := range matches {
				dest := m[2]
				if dest == "/dev/null" {
					continue
				}
				if strings.Contains(dest, "$(") || strings.Contains(dest, "`") {
					paths = append(paths, "unknown")
				} else {
					paths = append(paths, dest)
				}
			}
			return paths
		},
	},
	// tee with a file argument
	{
		re: regexp.MustCompile(`\btee\s+(?:-[a-zA-Z]*\s+)*([^\s;&|]+)`),
		pathFunc: func(cmd string) []string {
			re := regexp.MustCompile(`\btee\s+(?:-[a-zA-Z]*\s+)*([^\s;&|]+)`)
			m := re.FindStringSubmatch(cmd)
			if m == nil {
				return nil
			}
			dest := m[1]
			if strings.HasPrefix(dest, "-") {
				return nil
			}
			if strings.Contains(dest, "$(") || strings.Contains(dest, "`") {
				return []string{"unknown"}
			}
			return []string{dest}
		},
	},
	// find with -delete or -exec rm
	{
		re: regexp.MustCompile(`\bfind\b.*(?:-delete\b|-exec\s+rm\b)`),
		pathFunc: func(cmd string) []string {
			return []string{"unknown"}
		},
	},
	// sed -i (in-place edit)
	{
		re: regexp.MustCompile(`\bsed\s+(?:-[a-zA-Z]*)*i`),
		pathFunc: func(cmd string) []string {
			// Extract the last non-flag argument as the file
			re := regexp.MustCompile(`\bsed\s+(?:-[^\s]*\s+)*'[^']*'\s+(\S+)`)
			m := re.FindStringSubmatch(cmd)
			if m != nil {
				dest := m[1]
				if strings.Contains(dest, "$(") || strings.Contains(dest, "`") {
					return []string{"unknown"}
				}
				return []string{dest}
			}
			return []string{"unknown"}
		},
	},
	// sort -o (output to file)
	{
		re: regexp.MustCompile(`\bsort\s+.*-o\s+([^\s;&|]+)`),
		pathFunc: func(cmd string) []string {
			re := regexp.MustCompile(`\bsort\s+.*-o\s+([^\s;&|]+)`)
			m := re.FindStringSubmatch(cmd)
			if m == nil {
				return []string{"unknown"}
			}
			dest := m[1]
			if strings.Contains(dest, "$(") || strings.Contains(dest, "`") {
				return []string{"unknown"}
			}
			return []string{dest}
		},
	},
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
	Command       string `json:"command"`
	Timeout       *int   `json:"timeout,omitempty"`
	Description   string `json:"description,omitempty"`
	Permission    string `json:"permission,omitempty"`
	Justification string `json:"justification,omitempty"`
	Background    bool   `json:"background,omitempty"`
	Workdir       string `json:"workdir,omitempty"`
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
				"permission": map[string]any{
					"type":        "string",
					"enum":        []string{"default", "wider"},
					"description": "Permission level: default (normal checks) or wider (escalation with justification)",
				},
				"justification": map[string]any{
					"type":        "string",
					"description": "One-sentence justification required when permission is wider",
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "Run in background and return a monitor ID immediately. Output delivered at next turn via monitor drain.",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Set workdir instead of cd when the command should run in another directory.",
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

	if args.Permission == "wider" && args.Justification == "" {
		return &ExecuteResult{Output: "justification is required when permission is wider", IsError: true}, nil
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

	// Resolve workdir: defaults to tc.Directory, validates existence and type.
	workdir := tc.Directory
	if args.Workdir != "" {
		w := args.Workdir
		if !filepath.IsAbs(w) {
			w = filepath.Join(tc.Directory, w)
		}
		info, statErr := os.Stat(w)
		if statErr != nil {
			return &ExecuteResult{Output: fmt.Sprintf("workdir %q does not exist: %v", args.Workdir, statErr), IsError: true}, nil
		}
		if !info.IsDir() {
			return &ExecuteResult{Output: fmt.Sprintf("workdir %q is not a directory", args.Workdir), IsError: true}, nil
		}
		workdir = w
	}

	if blocked := gateSecretAccess(ctx, tc, args.Command, args.Justification); blocked != nil {
		return blocked, nil
	}

	if IsDestructive(args.Command) {
		if tc.Perms != nil {
			meta := map[string]any{"command": args.Command}
			if args.Permission == "wider" && args.Justification != "" {
				meta["justification"] = args.Justification
				meta["escalation"] = true
			}
			_, askErr := tc.Perms.Ask(ctx, permission.AskInput{
				SessionID:  tc.SessionID,
				Permission: "destructive-shell",
				Patterns:   []string{args.Command},
				Metadata:   meta,
				Ruleset:    tc.Ruleset,
				Tool:       askToolRef(tc),
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

	// Mutation check: best-effort heuristic for file-writing commands.
	// Records destination paths on the result metadata for rewind tracking (#971).
	// No separate permission ask — the registry-level shell permission already covers it.
	var mutationPaths []string
	if isMut, mPaths := ClassifyMutation(args.Command); isMut {
		mutationPaths = mPaths
	}

	// Background mode: delegate to MonitorManager immediately.
	if args.Background {
		return executeShellBackground(ctx, tc, args, workdir, timeout)
	}

	// Foreground mode with timeout-to-background promotion.
	// Use a cancel-only context so the process is not killed by the timeout;
	// the timer below handles the timeout and promotes the process instead.
	cmdCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", args.Command)
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Dir = workdir
	env := environToMap(procenv.Child(nil))
	if tc != nil && tc.ShellEnvHook != nil {
		if merged := tc.ShellEnvHook(tc.SessionID, tc.Directory, env); merged != nil {
			env = merged
		}
	}
	cmd.Env = flattenEnv(env)

	stdout := NewLimitedWriter(MaxOutputSize)
	stderr := NewLimitedWriter(MaxOutputSize)

	// Also tee output into a ring buffer for potential monitor handoff.
	rb := newRingBuffer(ringBufferSize)
	rbStdout := &ringBufferLineWriter{rb: rb}
	rbStderr := &ringBufferLineWriter{rb: rb, prefix: "STDERR: "}
	cmd.Stdout = io.MultiWriter(stdout, rbStdout)
	cmd.Stderr = io.MultiWriter(stderr, rbStderr)

	stdinPipe, pipeErr := cmd.StdinPipe()
	if pipeErr != nil {
		cancel()
		return &ExecuteResult{Output: fmt.Sprintf("stdin pipe: %v", pipeErr), IsError: true}, nil
	}

	slog.Info("shell exec", "command", args.Command, "dir", workdir, "timeout", timeout)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		cancel()
		return &ExecuteResult{Output: fmt.Sprintf("start command: %v", err), IsError: true}, nil
	}

	done := make(chan struct{})
	safego.Go(func() {
		_ = cmd.Wait()
		rbStdout.Flush()
		rbStderr.Flush()
		close(done)
	})

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// attachMutation stamps mutation metadata onto a result for rewind tracking.
	attachMutation := func(r *ExecuteResult) *ExecuteResult {
		if len(mutationPaths) > 0 {
			if r.Metadata == nil {
				r.Metadata = make(map[string]any)
			}
			r.Metadata["mutation_paths"] = mutationPaths
		}
		return r
	}

	select {
	case <-done:
		// Process finished within timeout — normal path.
		cancel()
		elapsed := time.Since(start)
		result, err := formatShellOutput(args.Command, cmd, stdout, stderr, elapsed)
		if err != nil {
			return result, err
		}
		return attachMutation(result), nil

	case <-timer.C:
		// Timeout — try to promote to background monitor.
		elapsed := time.Since(start)
		result, err := promoteToBackground(tc, cmd, cancel, stdinPipe, rb, done, args.Command, stdout, stderr, timeout, elapsed)
		if err != nil {
			return result, err
		}
		return attachMutation(result), nil

	case <-ctx.Done():
		// Outer context cancelled (e.g. deadline from bodyTimeout).
		elapsed := time.Since(start)
		if ctx.Err() == context.DeadlineExceeded {
			result, err := promoteToBackground(tc, cmd, cancel, stdinPipe, rb, done, args.Command, stdout, stderr, timeout, elapsed)
			if err != nil {
				return result, err
			}
			return attachMutation(result), nil
		}
		cancel()
		<-done
		return &ExecuteResult{
			Output:  fmt.Sprintf("Command cancelled after %v", elapsed),
			IsError: true,
		}, nil
	}
}

// executeShellBackground starts a command via MonitorManager and returns the
// monitor ID immediately.
func executeShellBackground(ctx context.Context, tc *Context, args shellArgs, workdir string, timeout time.Duration) (*ExecuteResult, error) {
	if tc.MonitorManager == nil {
		return &ExecuteResult{Output: "Monitor manager not available for background execution", IsError: true}, nil
	}

	desc := args.Description
	if desc == "" {
		desc = args.Command
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
	}

	monCtx := context.WithoutCancel(ctx)
	monID, err := tc.MonitorManager.Start(monCtx, args.Command, desc, workdir, timeout)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to start background command: %v", err), IsError: true}, nil
	}

	return &ExecuteResult{
		Output: fmt.Sprintf("Command started in background (ID: %s)\nOutput will be delivered at the next turn boundary.", monID),
	}, nil
}

// promoteToBackground attempts to transfer a timed-out foreground command to
// MonitorManager. If promotion fails, it kills the process and returns the
// timeout error.
func promoteToBackground(tc *Context, cmd *exec.Cmd, cancel context.CancelFunc, stdinPipe io.WriteCloser, rb *ringBuffer, done chan struct{}, command string, stdout, stderr *LimitedWriter, timeout, elapsed time.Duration) (*ExecuteResult, error) {
	partial := collectPartialOutput(stdout, stderr)

	if tc.MonitorManager != nil {
		monID, err := tc.MonitorManager.Adopt(cmd, cancel, stdinPipe, rb, done, command)
		if err == nil {
			slog.Info("shell promoted to background", "command", command, "timeout", timeout, "monitorID", monID)
			msg := fmt.Sprintf("Command promoted to background after %v (ID: %s)\nOutput will be delivered at the next turn boundary.", timeout, monID)
			if partial != "" {
				msg += "\nPartial output:\n" + partial
			}
			return &ExecuteResult{Output: msg}, nil
		}
		slog.Warn("shell promote failed, killing process", "command", command, "error", err)
	}

	// Promotion failed — fall back to killing the process.
	cancel()
	<-done
	slog.Warn("shell timeout", "command", command, "timeout", timeout, "elapsed", elapsed)
	return &ExecuteResult{
		Output:  fmt.Sprintf("Command timed out after %v\n%s", timeout, partial),
		IsError: true,
	}, nil
}

// formatShellOutput builds the result for a completed foreground shell command.
func formatShellOutput(command string, cmd *exec.Cmd, stdout, stderr *LimitedWriter, elapsed time.Duration) (*ExecuteResult, error) {
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

	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		exitErr := fmt.Sprintf("exit status %d", cmd.ProcessState.ExitCode())
		slog.Warn("shell error", "command", command, "elapsed", elapsed, "error", exitErr, "stderrLen", stderr.Len())
		exitMsg := fmt.Sprintf("Command exited with error: %s\n%s", exitErr, output.String())
		return &ExecuteResult{Output: exitMsg, IsError: true}, nil
	}

	slog.Info("shell done", "command", command, "elapsed", elapsed, "stdoutLen", stdout.Len(), "stderrLen", stderr.Len())
	return &ExecuteResult{Output: output.String()}, nil
}

// collectPartialOutput formats whatever stdout/stderr has been captured so far.
func collectPartialOutput(stdout, stderr *LimitedWriter) string {
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
	return output.String()
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

// ClassifyMutation checks whether a shell command writes to files via
// redirection, tee, in-place edit, etc. Returns the detected destination
// paths (or "unknown" when the path is not statically parseable).
//
// This is a best-effort heuristic — NOT a security boundary. Unusual shell
// constructs, variable expansion, and creative quoting can bypass detection.
func ClassifyMutation(command string) (isMutation bool, paths []string) {
	// Skip >&N and 2>&1 patterns before checking mutation patterns —
	// pre-filter the command to avoid false positives from fd redirects.
	for _, mp := range mutationPatterns {
		if !mp.re.MatchString(command) {
			continue
		}
		if mp.pathFunc != nil {
			extracted := mp.pathFunc(command)
			if len(extracted) > 0 {
				isMutation = true
				paths = append(paths, extracted...)
			}
		} else {
			isMutation = true
			paths = append(paths, "unknown")
		}
	}
	return isMutation, paths
}

// IsMutation returns true if the command appears to write to files.
// Best-effort heuristic — not a security boundary.
func IsMutation(command string) bool {
	mut, _ := ClassifyMutation(command)
	return mut
}

// gateSecretAccess asks before a command that names a secret file.
// A nil result means the command may run. With no permission service the
// command stays blocked, because nothing can approve it.
func gateSecretAccess(ctx context.Context, tc *Context, command, justification string) *ExecuteResult {
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
	meta := map[string]any{"command": command}
	if justification != "" {
		meta["justification"] = justification
		meta["escalation"] = true
	}
	_, askErr := tc.Perms.Ask(ctx, permission.AskInput{
		SessionID:  tc.SessionID,
		Permission: "secret-shell",
		Patterns:   []string{command},
		Metadata:   meta,
		Ruleset:    tc.Ruleset,
		Tool:       askToolRef(tc),
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
