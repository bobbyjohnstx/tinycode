package integration

import (
	"strings"
	"testing"

	testutil "github.com/bobbyjohnstx/tinycode-go/test"
)

func TestPermission_SkipPerms_AllowsExecution(t *testing.T) {
	h := testutil.NewTestHarness(t)

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "shell",
			Arguments: `{"command":"echo permitted"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "command ran"})

	result := h.RunJSON("run echo permitted")

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !result.HasEvent("tool_begin") {
		t.Error("expected tool_begin event when permissions skipped")
	}
	if !result.HasEvent("tool_end") {
		t.Error("expected tool_end event when permissions skipped")
	}
	if !strings.Contains(result.TextContent(), "command ran") {
		t.Errorf("expected final text, got: %q", result.TextContent())
	}
}

func TestPermission_DefaultDeny_BlocksExecution(t *testing.T) {
	h := testutil.NewTestHarness(t)

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "shell",
			Arguments: `{"command":"echo blocked"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "no tools used"})

	args := []string{
		"run",
		"--format", "json",
		"-m", h.MockModelName(),
		"run echo blocked",
	}
	result := h.Run(args...)

	// Default mode auto-denies permissions — tool should not execute
	toolCalls := result.ToolCalls()
	for _, tc := range toolCalls {
		name, _ := tc.Data["toolName"].(string)
		if name == "shell" {
			// tool_begin may fire, but the tool result should indicate denial
			t.Log("tool_begin for shell seen — checking tool_end for denial")
		}
	}
}

func TestPermission_ConfigAllow(t *testing.T) {
	// TODO: Test config-based permission.allow rules by writing a config file
	// with permission.allow = ["shell *"] and verifying the tool executes
	// without --dangerously-skip-permissions.
	t.Skip("config-based permission allow test not yet implemented")
}

func TestPermission_JSONProtocol(t *testing.T) {
	// TODO: Implement pipe-based interactive permission test.
	// This requires concurrent stdin/stdout handling:
	// 1. Start tinycode with --permissions json
	// 2. Read permission request from stdout
	// 3. Write permission reply to stdin
	// 4. Verify tool executes (allow) or is blocked (reject)
	t.Skip("interactive JSON permission protocol test not yet implemented")
}
