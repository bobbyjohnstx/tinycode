package integration

import (
	"strings"
	"testing"

	testutil "github.com/bobbyjohnstx/tinycode/test"
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
			Arguments: `{"command":"printf PERM_DENY_MARKER"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "no tools used"})

	args := []string{
		"run",
		"--format", "json",
		"-m", h.MockModelName(),
		"run blocked shell",
	}
	result := h.Run(args...)

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	for _, e := range result.Events {
		if e.Type != "text" {
			continue
		}
		text, _ := e.Data["text"].(string)
		if strings.Contains(text, "PERM_DENY_MARKER") {
			t.Error("shell output should not appear in assistant text when permissions are auto-denied")
		}
	}
}

func TestPermission_ConfigAllow(t *testing.T) {
	h := testutil.NewTestHarness(t)
	h.WriteConfig(map[string]any{
		"permission": map[string]any{
			"allow": []string{"shell *"},
		},
	})

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "shell",
			Arguments: `{"command":"echo permitted"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "command ran"})

	result := h.Run(
		"run",
		"--format", "json",
		"-m", h.MockModelName(),
		"run echo permitted",
	)

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !result.HasEvent("tool_end") {
		t.Error("expected tool_end when shell is allowed via config")
	}
	if !strings.Contains(result.TextContent(), "command ran") {
		t.Errorf("expected final text, got: %q", result.TextContent())
	}
}

func TestPermission_JSONProtocol(t *testing.T) {
	h := testutil.NewTestHarness(t)
	h.WriteConfig(map[string]any{
		"permission": map[string]any{
			"allow": []string{"shell *"},
		},
	})

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "shell",
			Arguments: `{"command":"echo json-permitted"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "command ran"})

	result := h.Run(
		"run",
		"--format", "json",
		"--permissions", "json",
		"-m", h.MockModelName(),
		"run echo json-permitted",
	)

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !result.HasEvent("tool_end") {
		t.Error("expected tool_end in JSON permission mode")
	}
	if !strings.Contains(result.TextContent(), "command ran") {
		t.Errorf("expected final text, got: %q", result.TextContent())
	}
}
