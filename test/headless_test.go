package test

import (
	"os"
	"strings"
	"testing"
)

func TestHeadless_SinglePromptTextResponse(t *testing.T) {
	h := NewTestHarness(t)
	h.MockServer.AddResponse(MockResponse{Content: "Hello from mock LLM"})

	result := h.RunJSON("Say hello")

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.TextContent(), "Hello from mock LLM") {
		t.Errorf("expected mock response in text events, got: %q", result.TextContent())
	}
	if !result.HasEvent("text") {
		t.Error("expected text event")
	}
}

func TestHeadless_StepEvents(t *testing.T) {
	h := NewTestHarness(t)
	h.MockServer.AddResponse(MockResponse{Content: "Response text"})

	result := h.RunJSON("Test step events")

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !result.HasEvent("step_start") {
		t.Error("expected step_start event")
	}
	if !result.HasEvent("step_finish") {
		t.Error("expected step_finish event")
	}
}

func TestHeadless_ToolCallRead(t *testing.T) {
	h := NewTestHarness(t)

	testFile := h.WorkDir + "/test-file.txt"
	if err := writeFile(testFile, "file contents here"); err != nil {
		t.Fatal(err)
	}

	h.MockServer.AddResponse(MockResponse{
		ToolCalls: []MockToolCall{
			{Name: "read", Arguments: `{"file_path":"` + testFile + `"}`},
		},
	})
	h.MockServer.AddResponse(MockResponse{Content: "I read the file"})

	result := h.RunJSON("Read the test file")

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	toolCalls := result.ToolCalls()
	if len(toolCalls) == 0 {
		t.Fatal("expected at least one tool_begin event")
	}

	if name, ok := toolCalls[0].Data["toolName"].(string); !ok || name != "read" {
		t.Errorf("expected tool name 'read', got %q", name)
	}

	if !result.HasEvent("tool_end") {
		t.Error("expected tool_end event")
	}
}

func TestHeadless_ToolCallShell(t *testing.T) {
	h := NewTestHarness(t)

	h.MockServer.AddResponse(MockResponse{
		ToolCalls: []MockToolCall{
			{Name: "shell", Arguments: `{"command":"echo test-output-12345"}`},
		},
	})
	h.MockServer.AddResponse(MockResponse{Content: "Shell command executed"})

	result := h.RunJSON("Run a shell command")

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	toolCalls := result.ToolCalls()
	if len(toolCalls) == 0 {
		t.Fatal("expected tool_begin event for shell")
	}
}

func TestHeadless_MaxIterations(t *testing.T) {
	h := NewTestHarness(t)

	for i := 0; i < 10; i++ {
		h.MockServer.AddResponse(MockResponse{
			ToolCalls: []MockToolCall{
				{Name: "shell", Arguments: `{"command":"echo iteration"}`},
			},
		})
	}

	result := h.RunJSON("Loop forever", "--max-iterations", "2")

	starts := result.EventsOfType("step_start")
	if len(starts) > 3 {
		t.Errorf("expected <=3 step_start events with --max-iterations 2, got %d", len(starts))
	}
}

func TestHeadless_LLMError500(t *testing.T) {
	h := NewTestHarness(t)
	h.MockServer.AddResponse(MockResponse{StatusCode: 500})

	result := h.RunJSON("Trigger error")

	if result.Stderr == "" && result.ExitCode == 0 {
		t.Error("expected error output or non-zero exit on LLM 500")
	}
}

func TestHeadless_RequestContainsSystemPrompt(t *testing.T) {
	h := NewTestHarness(t)
	h.MockServer.AddResponse(MockResponse{Content: "OK"})

	h.RunJSON("Hello")

	reqs := h.MockServer.Requests()
	if len(reqs) == 0 {
		t.Fatal("no requests captured")
	}

	if len(reqs[0].Messages) < 2 {
		t.Fatal("expected at least system + user messages")
	}
}

func TestHeadless_RequestContainsUserPrompt(t *testing.T) {
	h := NewTestHarness(t)
	h.MockServer.AddResponse(MockResponse{Content: "OK"})

	h.RunJSON("My specific test prompt")

	reqs := h.MockServer.Requests()
	if len(reqs) == 0 {
		t.Fatal("no requests captured")
	}

	found := false
	for _, msg := range reqs[0].Messages {
		if strings.Contains(string(msg), "My specific test prompt") {
			found = true
			break
		}
	}
	if !found {
		t.Error("user prompt not found in captured request messages")
	}
}

func TestHeadless_MockServerRecordsMultipleRequests(t *testing.T) {
	h := NewTestHarness(t)
	h.MockServer.AddResponse(MockResponse{
		ToolCalls: []MockToolCall{
			{Name: "shell", Arguments: `{"command":"echo hi"}`},
		},
	})
	h.MockServer.AddResponse(MockResponse{Content: "Done"})

	h.RunJSON("Call a tool")

	count := h.MockServer.RequestCount()
	if count < 2 {
		t.Errorf("expected at least 2 requests (initial + after tool), got %d", count)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
