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

func TestPermission_BuildAllowsOrdinaryShell(t *testing.T) {
	// Default build agent allows ordinary shell without --dangerously-skip-permissions.
	h := testutil.NewTestHarness(t)

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "shell",
			Arguments: `{"command":"echo ordinary-shell-ok"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "command ran"})

	result := h.Run(
		"run",
		"--format", "json",
		"-m", h.MockModelName(),
		"run echo ordinary-shell-ok",
	)

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !result.HasEvent("tool_end") {
		t.Error("expected tool_end for ordinary shell under build agent without skip-perms")
	}
}

func TestPermission_ResidualAsk_WebfetchRejected(t *testing.T) {
	// Residual Ask classes (webfetch, .env*, destructive/secret-shell, external_directory)
	// are auto-rejected in headless mode without skip-perms or -i / --permissions json.
	h := testutil.NewTestHarness(t)

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "webfetch",
			Arguments: `{"url":"https://example.com"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "fetch blocked"})

	result := h.Run(
		"run",
		"--format", "json",
		"-m", h.MockModelName(),
		"fetch example.com",
	)

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	for _, e := range result.EventsOfType("tool_end") {
		isError, _ := e.Data["isError"].(bool)
		if isError {
			return
		}
	}
	t.Error("expected webfetch tool_end with isError when residual Ask is auto-rejected")
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
