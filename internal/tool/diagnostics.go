package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/procenv"
)

const diagnosticsTimeout = 10 * time.Second

type diagnosticsArgs struct {
	FilePath string `json:"file_path"`
}

func DiagnosticsTool() *Def {
	return &Def{
		ID:          "diagnostics",
		Description: "Check a file for errors after editing. Call this after completing a batch of edits to catch syntax errors, type mismatches, and lint issues before moving on. Runs go vet (Go), ruff (Python), or tsc (TypeScript/JavaScript). Use lsp_diagnostics instead if a language server is available.",
		Permission:  "",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "File to check",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: executeDiagnostics,
	}
}

func executeDiagnostics(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args diagnosticsArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	path := resolvePath(args.FilePath, tc.Directory)
	ext := strings.ToLower(filepath.Ext(path))

	cmd, dir := diagnosticCommand(ext, path)
	if cmd == nil {
		return &ExecuteResult{Output: fmt.Sprintf("No diagnostics available for %s files", ext)}, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, diagnosticsTimeout)
	defer cancel()

	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := runCmd(timeoutCtx, cmd)
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return &ExecuteResult{Output: "Diagnostics timed out after 10 seconds", IsError: true}, nil
	}

	output := strings.TrimSpace(stdout.String() + stderr.String())
	if err != nil {
		if output == "" {
			output = err.Error()
		}
		return &ExecuteResult{Output: output}, nil
	}

	if output == "" {
		return &ExecuteResult{Output: "No issues found"}, nil
	}

	return &ExecuteResult{Output: output}, nil
}

// diagnosticCommand returns the command and working directory for the given
// file extension. Returns nil if no diagnostic tool is available.
func diagnosticCommand(ext, path string) (*exec.Cmd, string) {
	dir := filepath.Dir(path)

	switch ext {
	case ".go":
		if _, err := exec.LookPath("go"); err != nil {
			return nil, ""
		}
		return exec.Command("go", "vet", "./..."), dir

	case ".py":
		if ruff, err := exec.LookPath("ruff"); err == nil {
			return exec.Command(ruff, "check", path), dir
		}
		if python, err := exec.LookPath("python3"); err == nil {
			return exec.Command(python, "-m", "py_compile", path), dir
		}
		return nil, ""

	case ".ts", ".tsx", ".js", ".jsx":
		if npx, err := exec.LookPath("npx"); err == nil {
			return exec.Command(npx, "tsc", "--noEmit"), dir
		}
		return nil, ""

	default:
		return nil, ""
	}
}

// runCmd starts and waits for the command, respecting the context for cancellation.
func runCmd(ctx context.Context, cmd *exec.Cmd) error {
	if cmd.Env == nil {
		cmd.Env = procenv.Child(nil)
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	case err := <-done:
		return err
	}
}
