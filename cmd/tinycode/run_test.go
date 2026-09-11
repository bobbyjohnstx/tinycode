package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

func TestReadNextPrompt_TextMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantOK   bool
	}{
		{
			name:     "returns trimmed line",
			input:    "  hello world  \n",
			wantText: "hello world",
			wantOK:   true,
		},
		{
			name:     "returns empty on EOF",
			input:    "",
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "returns empty string for blank line",
			input:    "   \n",
			wantText: "",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			text, ok := readNextPrompt(scanner, false, nil)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestReadNextPrompt_JSONMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantOK   bool
	}{
		{
			name:     "parses prompt type",
			input:    `{"type":"prompt","text":"fix the bug"}` + "\n",
			wantText: "fix the bug",
			wantOK:   true,
		},
		{
			name:     "exits on exit type",
			input:    `{"type":"exit"}` + "\n",
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "skips permission_reply type",
			input:    `{"type":"permission_reply","id":"123","allow":true}` + "\n",
			wantText: "",
			wantOK:   true,
		},
		{
			name:     "skips malformed JSON",
			input:    "not json\n",
			wantText: "",
			wantOK:   true,
		},
		{
			name:     "skips unknown type",
			input:    `{"type":"unknown","data":"stuff"}` + "\n",
			wantText: "",
			wantOK:   true,
		},
		{
			name:     "returns false on EOF",
			input:    "",
			wantText: "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			text, ok := readNextPrompt(scanner, true, nil)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestReadNextPrompt_TextMultiLine(t *testing.T) {
	input := "first prompt\nsecond prompt\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	text, ok := readNextPrompt(scanner, false, nil)
	if text != "first prompt" || !ok {
		t.Errorf("first: text=%q ok=%v, want %q %v", text, ok, "first prompt", true)
	}

	text, ok = readNextPrompt(scanner, false, nil)
	if text != "second prompt" || !ok {
		t.Errorf("second: text=%q ok=%v, want %q %v", text, ok, "second prompt", true)
	}

	text, ok = readNextPrompt(scanner, false, nil)
	if text != "" || ok {
		t.Errorf("EOF: text=%q ok=%v, want %q %v", text, ok, "", false)
	}
}

func TestReadNextPrompt_JSONMultiLine(t *testing.T) {
	input := `{"type":"prompt","text":"turn 1"}` + "\n" +
		`{"type":"permission_reply","id":"x"}` + "\n" +
		`{"type":"prompt","text":"turn 2"}` + "\n" +
		`{"type":"exit"}` + "\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	text, ok := readNextPrompt(scanner, true, nil)
	if text != "turn 1" || !ok {
		t.Errorf("turn 1: text=%q ok=%v", text, ok)
	}

	// permission_reply should be skipped
	text, ok = readNextPrompt(scanner, true, nil)
	if text != "" || !ok {
		t.Errorf("permission_reply: text=%q ok=%v, want empty+true", text, ok)
	}

	text, ok = readNextPrompt(scanner, true, nil)
	if text != "turn 2" || !ok {
		t.Errorf("turn 2: text=%q ok=%v", text, ok)
	}

	// exit should stop
	text, ok = readNextPrompt(scanner, true, nil)
	if text != "" || ok {
		t.Errorf("exit: text=%q ok=%v, want empty+false", text, ok)
	}
}

func TestReadNextPrompt_PermissionReplyRouting(t *testing.T) {
	permReplyCh := make(chan permission.ReplyInput, 4)
	input := `{"type":"permission_reply","id":"req_1","reply":"once"}` + "\n" +
		`{"type":"prompt","text":"next turn"}` + "\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	// permission_reply should be routed to the channel
	text, ok := readNextPrompt(scanner, true, permReplyCh)
	if text != "" || !ok {
		t.Errorf("permission_reply: text=%q ok=%v, want empty+true", text, ok)
	}

	select {
	case reply := <-permReplyCh:
		if reply.RequestID != "req_1" {
			t.Errorf("RequestID = %q, want %q", reply.RequestID, "req_1")
		}
		if reply.Reply != permission.ReplyOnce {
			t.Errorf("Reply = %q, want %q", reply.Reply, permission.ReplyOnce)
		}
	default:
		t.Error("expected permission reply on channel, got nothing")
	}

	// next prompt should still work
	text, ok = readNextPrompt(scanner, true, permReplyCh)
	if text != "next turn" || !ok {
		t.Errorf("prompt: text=%q ok=%v, want %q %v", text, ok, "next turn", true)
	}
}

func TestReadNextPrompt_PermissionReplyNilChannel(t *testing.T) {
	input := `{"type":"permission_reply","id":"req_1","reply":"once"}` + "\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	// With nil channel, permission_reply should be silently skipped (no panic)
	text, ok := readNextPrompt(scanner, true, nil)
	if text != "" || !ok {
		t.Errorf("permission_reply with nil ch: text=%q ok=%v, want empty+true", text, ok)
	}
}

func TestStreamRunOutput_AllEventTypes(t *testing.T) {
	b := bus.New()
	defer b.Close()

	// Capture stdout
	oldStdout := captureStdout(t)
	defer oldStdout.restore()

	streamRunOutput(b, true)

	// Publish events for each new type
	b.Publish("session.step.start", map[string]any{
		"sessionID": "ses_1",
		"stepID":    "step_1",
		"iteration": 1,
		"model":     "test-model",
	})
	b.Publish("session.step.finish", map[string]any{
		"sessionID": "ses_1",
		"stepID":    "step_1",
		"iteration": 1,
		"usage":     map[string]any{"input": 100, "output": 50},
		"error":     nil,
	})
	b.Publish("session.warning", map[string]any{
		"sessionID": "ses_1",
		"message":   "test warning",
	})
	b.Publish("session.compacted", map[string]any{
		"sessionID":    "ses_1",
		"compactionNum": 1,
		"preMessages":  10,
		"postMessages": 3,
	})
	b.Publish("session.message", map[string]any{
		"sessionID": "ses_1",
		"message": session.Message{
			SessionID: "ses_1",
			Role:      "assistant",
			Parts: []session.Part{
				{Type: session.PartText, Text: "hello"},
				{Type: session.PartReasoning, Text: "thinking about it"},
			},
		},
	})

	// Allow goroutines to process
	time.Sleep(50 * time.Millisecond)
	output := oldStdout.read()

	// Parse each line as JSON and verify
	lines := strings.Split(strings.TrimSpace(output), "\n")
	eventTypes := make(map[string]bool)
	for _, line := range lines {
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Errorf("invalid JSON line: %q: %v", line, err)
			continue
		}
		if typ, ok := evt["type"].(string); ok {
			eventTypes[typ] = true
		}
	}

	for _, want := range []string{"step_start", "step_finish", "warning", "compacted", "reasoning"} {
		if !eventTypes[want] {
			t.Errorf("missing event type %q in output; got types: %v", want, eventTypes)
		}
	}
}

// stdoutCapture helps capture stdout output in tests.
type stdoutCapture struct {
	t       *testing.T
	origOut *os.File
	r       *os.File
	w       *os.File
}

func captureStdout(t *testing.T) *stdoutCapture {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	return &stdoutCapture{t: t, origOut: orig, r: r, w: w}
}

func (c *stdoutCapture) restore() {
	os.Stdout = c.origOut
	c.w.Close()
	c.r.Close()
}

func (c *stdoutCapture) read() string {
	c.w.Close()
	os.Stdout = c.origOut
	buf := make([]byte, 64*1024)
	n, _ := c.r.Read(buf)
	return string(buf[:n])
}

func TestMultiTurnLoop_ReadySignals(t *testing.T) {
	// Simulate the multi-turn ready/prompt loop with JSON format.
	// Verify ready signals are emitted between turns and loop exits on exit message.
	input := `{"type":"prompt","text":"turn 2"}` + "\n" +
		`{"type":"exit"}` + "\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	cap := captureStdout(t)

	// Simulate what runRun's multi-turn loop does: emit ready, read prompt, repeat
	var prompts []string
	for {
		// Emit ready signal (as runRun does)
		line, _ := json.Marshal(map[string]string{"type": "ready"})
		fmt.Println(string(line))

		prompt, ok := readNextPrompt(scanner, true, nil)
		if !ok {
			break
		}
		if prompt != "" {
			prompts = append(prompts, prompt)
		}
	}

	output := cap.read()

	// Should have collected "turn 2" before exit
	if len(prompts) != 1 || prompts[0] != "turn 2" {
		t.Errorf("prompts = %v, want [turn 2]", prompts)
	}

	// Should have emitted 2 ready signals (one before turn 2, one before exit)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	readyCount := 0
	for _, l := range lines {
		var evt map[string]string
		if err := json.Unmarshal([]byte(l), &evt); err == nil && evt["type"] == "ready" {
			readyCount++
		}
	}
	if readyCount != 2 {
		t.Errorf("ready signals = %d, want 2", readyCount)
	}
}

func TestStreamRunOutput_ToolCallFlow(t *testing.T) {
	b := bus.New()
	defer b.Close()

	cap := captureStdout(t)
	streamRunOutput(b, true)

	// Simulate a complete tool call sequence as the processor emits it
	b.Publish("session.step.start", map[string]any{
		"sessionID": "ses_1", "stepID": "step_1", "iteration": 1, "model": "test-model",
	})
	b.Publish("session.text.delta", map[string]any{
		"sessionID": "ses_1", "text": "Let me read that file.",
	})
	b.Publish("session.tool.begin", map[string]any{
		"sessionID": "ses_1", "toolName": "read", "toolCallID": "call_1",
	})
	b.Publish("session.tool.end", map[string]any{
		"sessionID": "ses_1", "toolName": "read", "toolCallID": "call_1",
		"toolArgs": `{"path":"main.go"}`,
	})
	b.Publish("session.step.finish", map[string]any{
		"sessionID": "ses_1", "stepID": "step_1", "iteration": 1,
		"usage": map[string]any{"input": 100, "output": 50}, "error": "",
	})

	time.Sleep(50 * time.Millisecond)
	output := cap.read()

	// Each subscriber goroutine runs independently, so ordering across
	// different event types is non-deterministic. Verify all expected types
	// are present.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	typeCounts := make(map[string]int)
	for _, l := range lines {
		var evt map[string]any
		if err := json.Unmarshal([]byte(l), &evt); err != nil {
			t.Errorf("invalid JSON: %q", l)
			continue
		}
		if typ, ok := evt["type"].(string); ok {
			typeCounts[typ]++
		}
	}

	for _, want := range []string{"step_start", "text", "tool_begin", "tool_end", "step_finish"} {
		if typeCounts[want] != 1 {
			t.Errorf("event type %q: count=%d, want 1; all types: %v", want, typeCounts[want], typeCounts)
		}
	}

	// Verify tool_end contains the args
	foundToolArgs := false
	for _, l := range lines {
		var evt map[string]any
		json.Unmarshal([]byte(l), &evt)
		if evt["type"] == "tool_end" {
			foundToolArgs = true
			if evt["toolArgs"] != `{"path":"main.go"}` {
				t.Errorf("tool_end toolArgs = %v, want %q", evt["toolArgs"], `{"path":"main.go"}`)
			}
		}
	}
	if !foundToolArgs {
		t.Error("tool_end event not found")
	}
}

func TestJSONPermissionProtocol_EndToEnd(t *testing.T) {
	b := bus.New()
	defer b.Close()

	permSvc := permission.NewService(b)

	cap := captureStdout(t)

	// Set up JSON permission mode — returns the reply channel
	permReplyCh := setupRunPermissions(b, permSvc, false, false, "json")
	if permReplyCh == nil {
		t.Fatal("expected non-nil permReplyCh for json mode")
	}

	// Ask for permission in a goroutine (it will block until reply)
	askDone := make(chan error, 1)
	go func() {
		askDone <- permSvc.Ask(t.Context(), permission.AskInput{
			ID:         "perm_test_1",
			SessionID:  "ses_1",
			Permission: "shell",
			Patterns:   []string{"ls -la"},
		})
	}()

	// Wait for the permission event to be emitted on stdout
	time.Sleep(50 * time.Millisecond)

	// Send the reply through the channel (simulating readNextPrompt routing)
	permReplyCh <- permission.ReplyInput{
		RequestID: "perm_test_1",
		Reply:     permission.ReplyOnce,
	}

	// Wait for the Ask to resolve
	select {
	case err := <-askDone:
		if err != nil {
			t.Errorf("Ask returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask did not resolve within timeout")
	}

	output := cap.read()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Verify the permission request was emitted as NDJSON
	found := false
	for _, l := range lines {
		var evt map[string]any
		if err := json.Unmarshal([]byte(l), &evt); err != nil {
			continue
		}
		if evt["type"] == "permission" && evt["id"] == "perm_test_1" {
			found = true
			if evt["permission"] != "shell" {
				t.Errorf("permission field = %v, want 'shell'", evt["permission"])
			}
		}
	}
	if !found {
		t.Errorf("permission event not found in output: %q", output)
	}
}
