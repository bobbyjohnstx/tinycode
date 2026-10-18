package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

const (
	defaultSemgrepConfig = "p/java-security"
	semgrepCmdTimeout    = 25 * time.Second
	maxScanOutputBytes   = 32 * 1024
)

type semgrepResult struct {
	Results []semgrepFinding `json:"results"`
}

type semgrepFinding struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Start   struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"start"`
	Extra struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
	} `json:"extra"`
}

func scanTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "scan",
		Description: "Run Semgrep static analysis on a file or directory. Requires the semgrep CLI on PATH. Default ruleset is p/java-security.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory path to scan",
				},
				"config": map[string]any{
					"type":        "string",
					"description": "Semgrep config/ruleset (default: p/java-security)",
				},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
			var input struct {
				Path   string `json:"path"`
				Config string `json:"config"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if strings.TrimSpace(input.Path) == "" {
				return "", fmt.Errorf("path is required")
			}
			config := input.Config
			if config == "" {
				config = defaultSemgrepConfig
			}
			return runSemgrep(ctx, input.Path, config, tc.Directory)
		},
	}
}

func runSemgrep(ctx context.Context, path, config, cwd string) (string, error) {
	if _, err := exec.LookPath("semgrep"); err != nil {
		return "", fmt.Errorf("semgrep not found. Install with: pip install semgrep or brew install semgrep")
	}

	runCtx, cancel := context.WithTimeout(ctx, semgrepCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "semgrep",
		"--config="+config,
		"--json",
		"--error",
		path,
	)
	if cwd != "" && !filepath.IsAbs(path) {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outBytes := stdout.Bytes()
	parsed, parseErr := parseSemgrepJSON(outBytes)
	if parseErr == nil {
		return formatFindings(parsed, config), nil
	}

	if err == nil {
		// Zero findings, empty/minimal JSON still OK.
		if len(bytes.TrimSpace(outBytes)) == 0 {
			return fmt.Sprintf("No Semgrep findings (config: %s).", config), nil
		}
		return "", fmt.Errorf("semgrep returned unparseable JSON: %v", parseErr)
	}

	snippet := strings.TrimSpace(stderr.String())
	if snippet == "" {
		snippet = strings.TrimSpace(stdout.String())
	}
	if len(snippet) > 500 {
		snippet = snippet[:500] + "…"
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("semgrep timed out after %s: %s", semgrepCmdTimeout, snippet)
	}
	return "", fmt.Errorf("semgrep failed: %s", snippet)
}

func parseSemgrepJSON(data []byte) (*semgrepResult, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("empty output")
	}
	// Semgrep may emit leading log lines; find the JSON object.
	if idx := bytes.IndexByte(data, '{'); idx > 0 {
		data = data[idx:]
	}
	var result semgrepResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func formatFindings(result *semgrepResult, config string) string {
	if result == nil || len(result.Results) == 0 {
		return fmt.Sprintf("No Semgrep findings (config: %s).", config)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Semgrep findings: %d (config: %s)\n\n", len(result.Results), config))

	omitted := 0
	for i, f := range result.Results {
		sev := f.Extra.Severity
		if sev == "" {
			sev = "UNKNOWN"
		}
		line := fmt.Sprintf("- [%s] %s at %s:%d\n  %s\n",
			sev, f.CheckID, f.Path, f.Start.Line, f.Extra.Message)
		if sb.Len()+len(line) > maxScanOutputBytes {
			omitted = len(result.Results) - i
			break
		}
		sb.WriteString(line)
	}
	if omitted > 0 {
		sb.WriteString(fmt.Sprintf("\n… %d finding(s) omitted (output truncated to %d KiB)\n", omitted, maxScanOutputBytes/1024))
	}
	return sb.String()
}

func main() {
	plugin.Run(plugin.Plugin{
		ID:    "semgrep",
		Tools: []plugin.ToolDef{scanTool()},
	})
}
