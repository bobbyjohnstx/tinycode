package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type TestHarness struct {
	MockServer *MockLLMServer
	BinPath    string
	WorkDir    string
	ConfigDir  string
	t          *testing.T
}

func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()

	binPath := findTinycodeBin(t)
	workDir := t.TempDir()
	configDir := t.TempDir()

	mock := NewMockLLMServer(t)
	t.Cleanup(mock.Close)

	writeTestConfig(t, configDir, mock.URL())

	return &TestHarness{
		MockServer: mock,
		BinPath:    binPath,
		WorkDir:    workDir,
		ConfigDir:  configDir,
		t:          t,
	}
}

type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Events   []NDJSONEvent
	Duration time.Duration
}

type NDJSONEvent struct {
	Type string         `json:"type"`
	Data map[string]any `json:"-"`
	Raw  string         `json:"-"`
}

func (r *RunResult) TextContent() string {
	var parts []string
	for _, e := range r.Events {
		if e.Type == "text" {
			if text, ok := e.Data["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

func (r *RunResult) ToolCalls() []NDJSONEvent {
	var out []NDJSONEvent
	for _, e := range r.Events {
		if e.Type == "tool_begin" {
			out = append(out, e)
		}
	}
	return out
}

func (r *RunResult) HasEvent(typ string) bool {
	for _, e := range r.Events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func (r *RunResult) EventsOfType(typ string) []NDJSONEvent {
	var out []NDJSONEvent
	for _, e := range r.Events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func (h *TestHarness) Run(args ...string) *RunResult {
	h.t.Helper()
	return h.runCmd(nil, args...)
}

func (h *TestHarness) RunWithStdin(stdin string, args ...string) *RunResult {
	h.t.Helper()
	return h.runCmd(&stdin, args...)
}

func (h *TestHarness) RunJSON(prompt string, extraArgs ...string) *RunResult {
	h.t.Helper()
	args := []string{
		"run",
		"--format", "json",
		"-m", h.MockModelName(),
		"--dangerously-skip-permissions",
	}
	args = append(args, extraArgs...)
	args = append(args, prompt)
	return h.runCmd(nil, args...)
}

func (h *TestHarness) RunJSONMultiTurn(prompts []string) *RunResult {
	h.t.Helper()

	if len(prompts) == 0 {
		h.t.Fatal("RunJSONMultiTurn requires at least one prompt")
	}

	// Pass the first prompt as a CLI arg to avoid the dual-scanner issue
	// in runRun() (two bufio.Scanners on stdin when initial prompt comes
	// from stdin). Remaining prompts go via stdin JSON.
	var stdinLines []string
	for _, p := range prompts[1:] {
		line, _ := json.Marshal(map[string]string{"type": "prompt", "text": p})
		stdinLines = append(stdinLines, string(line))
	}
	exitLine, _ := json.Marshal(map[string]string{"type": "exit"})
	stdinLines = append(stdinLines, string(exitLine))
	stdin := strings.Join(stdinLines, "\n") + "\n"

	args := []string{
		"run",
		"--format", "json",
		"--multi-turn",
		"-m", h.MockModelName(),
		"--dangerously-skip-permissions",
		prompts[0],
	}
	return h.runCmd(&stdin, args...)
}

func (h *TestHarness) MockModelName() string {
	return "vllm/test-model"
}

func (h *TestHarness) Env() []string {
	return []string{
		"TINYCODE_CONFIG_DIR=" + h.ConfigDir,
		"TINYCODE_DATA_DIR=" + h.ConfigDir,
		"TINYCODE_DB=:memory:",
		"TINYCODE_DISABLE_MOUSE=1",
		"TINYCODE_VLLM_HOST=" + h.MockServer.URL(),
		"OLLAMA_HOST=http://127.0.0.1:1",
		"HOME=" + h.WorkDir,
		"PATH=" + os.Getenv("PATH"),
	}
}

func (h *TestHarness) runCmd(stdin *string, args ...string) *RunResult {
	h.t.Helper()

	cmd := exec.Command(h.BinPath, args...)
	cmd.Dir = h.WorkDir
	cmd.Env = h.Env()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			h.t.Logf("run error (not exit): %v", err)
			exitCode = -1
		}
	}

	result := &RunResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	result.Events = ParseNDJSON(stdout.String())

	return result
}

func ParseNDJSON(s string) []NDJSONEvent {
	var events []NDJSONEvent
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		evt := NDJSONEvent{Raw: line, Data: raw}
		if t, ok := raw["type"].(string); ok {
			evt.Type = t
		}
		events = append(events, evt)
	}
	return events
}

func findTinycodeBin(t *testing.T) string {
	t.Helper()

	candidates := []string{
		"dist/tinycode",
		"../dist/tinycode",
		"../../dist/tinycode",
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	path, err := exec.LookPath("tinycode")
	if err == nil {
		return path
	}

	t.Skip("tinycode binary not found — run 'make build' first")
	return ""
}

func writeTestConfig(t *testing.T, configDir, mockURL string) {
	t.Helper()

	config := map[string]any{
		"disabled_providers": []string{"ollama", "openrouter", "lmstudio", "maas"},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
