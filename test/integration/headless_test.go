package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	testutil "github.com/bobbyjohnstx/tinycode-go/test"
)

func TestBasicPrompt(t *testing.T) {
	h := testutil.NewTestHarness(t)
	h.MockServer.AddResponse(testutil.MockResponse{Content: "Hello from tinycode"})

	result := h.RunJSON("Say hello")

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.TextContent(), "Hello from tinycode") {
		t.Errorf("expected mock response in output, got: %q", result.TextContent())
	}
	if !result.HasEvent("text") {
		t.Error("expected text event")
	}
}

func TestToolExecution_Shell(t *testing.T) {
	h := testutil.NewTestHarness(t)

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "shell",
			Arguments: `{"command":"echo hello-from-shell"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "The command printed hello-from-shell"})

	result := h.RunJSON("Run echo hello-from-shell")

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	toolCalls := result.ToolCalls()
	if len(toolCalls) == 0 {
		t.Fatal("expected at least one tool_begin event")
	}
	toolName, _ := toolCalls[0].Data["toolName"].(string)
	if toolName != "shell" {
		t.Errorf("expected tool 'shell', got %q", toolName)
	}
	if !result.HasEvent("tool_end") {
		t.Error("expected tool_end event")
	}
}

func TestToolExecution_Read(t *testing.T) {
	h := testutil.NewTestHarness(t)

	testFile := filepath.Join(h.WorkDir, "test-file.txt")
	os.WriteFile(testFile, []byte("file content here"), 0644)

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "read",
			Arguments: `{"file_path":"test-file.txt"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "The file contains: file content here"})

	result := h.RunJSON("Read test-file.txt")

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	toolCalls := result.ToolCalls()
	if len(toolCalls) == 0 {
		t.Fatal("expected read tool call")
	}
	toolName, _ := toolCalls[0].Data["toolName"].(string)
	if toolName != "read" {
		t.Errorf("expected tool 'read', got %q", toolName)
	}
}

func TestMultiTurn(t *testing.T) {
	h := testutil.NewTestHarness(t)

	h.MockServer.AddResponse(testutil.MockResponse{Content: "First response"})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "Second response"})

	result := h.RunJSONMultiTurn([]string{"first prompt", "second prompt"})

	text := result.TextContent()
	if !strings.Contains(text, "First response") {
		t.Errorf("missing first response in output: %q", text)
	}
	if !strings.Contains(text, "Second response") {
		t.Errorf("missing second response in output: %q", text)
	}
}

func TestMaxIterations(t *testing.T) {
	h := testutil.NewTestHarness(t)

	for i := 0; i < 10; i++ {
		h.MockServer.AddResponse(testutil.MockResponse{
			ToolCalls: []testutil.MockToolCall{{
				Name:      "shell",
				Arguments: `{"command":"echo iteration"}`,
			}},
		})
	}
	h.MockServer.AddResponse(testutil.MockResponse{Content: "final"})

	result := h.RunJSON("keep going", "--max-iterations", "3")

	steps := result.EventsOfType("step_start")
	if len(steps) > 3 {
		t.Errorf("expected max 3 steps, got %d", len(steps))
	}
}

func TestToolExecution_Write(t *testing.T) {
	h := testutil.NewTestHarness(t)

	outFile := filepath.Join(h.WorkDir, "output.txt")

	h.MockServer.AddResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{{
			Name:      "write",
			Arguments: `{"file_path":"output.txt","content":"written by test"}`,
		}},
	})
	h.MockServer.AddResponse(testutil.MockResponse{Content: "File written"})

	result := h.RunJSON("Write output.txt")

	if result.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(content) != "written by test" {
		t.Errorf("file content = %q, want %q", string(content), "written by test")
	}
}

func TestRequestCount(t *testing.T) {
	h := testutil.NewTestHarness(t)
	h.MockServer.AddResponse(testutil.MockResponse{Content: "ok"})

	h.RunJSON("hello")

	if h.MockServer.RequestCount() == 0 {
		t.Error("expected at least one request to mock server")
	}
}
