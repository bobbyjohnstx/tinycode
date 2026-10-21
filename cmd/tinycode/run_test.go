package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/server"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
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
		{
			name:     "exits on exit",
			input:    "exit\n",
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "exits on quit",
			input:    "quit\n",
			wantText: "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			text, ok := readNextPrompt(context.Background(), scanner, false, nil, nil)
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
			text, ok := readNextPrompt(context.Background(), scanner, true, nil, nil)
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

	text, ok := readNextPrompt(context.Background(), scanner, false, nil, nil)
	if text != "first prompt" || !ok {
		t.Errorf("first: text=%q ok=%v, want %q %v", text, ok, "first prompt", true)
	}

	text, ok = readNextPrompt(context.Background(), scanner, false, nil, nil)
	if text != "second prompt" || !ok {
		t.Errorf("second: text=%q ok=%v, want %q %v", text, ok, "second prompt", true)
	}

	text, ok = readNextPrompt(context.Background(), scanner, false, nil, nil)
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

	text, ok := readNextPrompt(context.Background(), scanner, true, nil, nil)
	if text != "turn 1" || !ok {
		t.Errorf("turn 1: text=%q ok=%v", text, ok)
	}

	// permission_reply should be skipped
	text, ok = readNextPrompt(context.Background(), scanner, true, nil, nil)
	if text != "" || !ok {
		t.Errorf("permission_reply: text=%q ok=%v, want empty+true", text, ok)
	}

	text, ok = readNextPrompt(context.Background(), scanner, true, nil, nil)
	if text != "turn 2" || !ok {
		t.Errorf("turn 2: text=%q ok=%v", text, ok)
	}

	// exit should stop
	text, ok = readNextPrompt(context.Background(), scanner, true, nil, nil)
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
	text, ok := readNextPrompt(context.Background(), scanner, true, permReplyCh, nil)
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
	text, ok = readNextPrompt(context.Background(), scanner, true, permReplyCh, nil)
	if text != "next turn" || !ok {
		t.Errorf("prompt: text=%q ok=%v, want %q %v", text, ok, "next turn", true)
	}
}

func TestReadNextPrompt_PermissionReplyNilChannel(t *testing.T) {
	input := `{"type":"permission_reply","id":"req_1","reply":"once"}` + "\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	// With nil channel, permission_reply should be silently skipped (no panic)
	text, ok := readNextPrompt(context.Background(), scanner, true, nil, nil)
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

	streamRunOutput(b, true, nil)

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
		"sessionID":     "ses_1",
		"compactionNum": 1,
		"preMessages":   10,
		"postMessages":  3,
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

		prompt, ok := readNextPrompt(context.Background(), scanner, true, nil, nil)
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
	streamRunOutput(b, true, nil)

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
	b.Publish("session.tool.result", map[string]any{
		"sessionID": "ses_1", "toolName": "read", "toolCallID": "call_1",
		"output": "package main\n", "isError": false,
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

	for _, want := range []string{"step_start", "text", "tool_begin", "tool_call_end", "tool_end", "step_finish"} {
		if typeCounts[want] != 1 {
			t.Errorf("event type %q: count=%d, want 1; all types: %v", want, typeCounts[want], typeCounts)
		}
	}

	foundResult := false
	foundCallEnd := false
	for _, l := range lines {
		var evt map[string]any
		_ = json.Unmarshal([]byte(l), &evt)
		switch evt["type"] {
		case "tool_end":
			foundResult = true
			if evt["output"] != "package main\n" {
				t.Errorf("tool_end output = %v, want package main", evt["output"])
			}
			if evt["isError"] != false {
				t.Errorf("tool_end isError = %v, want false", evt["isError"])
			}
			if evt["toolName"] != "read" || evt["toolCallID"] != "call_1" {
				t.Errorf("tool_end ids = %v/%v", evt["toolName"], evt["toolCallID"])
			}
		case "tool_call_end":
			foundCallEnd = true
			if evt["toolArgs"] != `{"path":"main.go"}` {
				t.Errorf("tool_call_end toolArgs = %v", evt["toolArgs"])
			}
		}
	}
	if !foundResult {
		t.Error("tool_end event not found")
	}
	if !foundCallEnd {
		t.Error("tool_call_end event not found")
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

func TestStdinMux_TextModeRoutesPermissionReply(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
		_ = w.Close()
	})

	replyCh := make(chan permission.ReplyInput, 1)
	mux := newStdinMux(false, replyCh)
	done := make(chan struct{})
	go func() {
		mux.run()
		close(done)
	}()

	if _, err := fmt.Fprintf(w, "plain prompt\n{\"type\":\"permission_reply\",\"id\":\"req_1\",\"reply\":\"once\"}\nnext prompt\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case p := <-mux.prompts:
		if p != "plain prompt" {
			t.Fatalf("first prompt = %q", p)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first prompt")
	}

	// Wait for the permission reply without racing the next prompt on the
	// shared select — both channels can be ready and select is nondeterministic.
	select {
	case got := <-replyCh:
		if got.RequestID != "req_1" || got.Reply != permission.ReplyOnce {
			t.Fatalf("reply = %+v", got)
		}
	case <-time.After(time.Second):
		select {
		case p := <-mux.prompts:
			t.Fatalf("permission_reply treated as prompt %q", p)
		default:
			t.Fatal("timed out waiting for permission reply")
		}
	}

	select {
	case p := <-mux.prompts:
		if p != "next prompt" {
			t.Fatalf("second prompt = %q", p)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second prompt")
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mux did not exit on stdin EOF")
	}
}

func TestCollectRunPrompt_PositionalOnly(t *testing.T) {
	// Avoid Chdir/os.Exit paths: pass a non-directory positional prompt.
	got := collectRunPrompt([]string{"fix", "the", "bug"}, true, false)
	if got != "fix the bug" {
		t.Fatalf("got %q", got)
	}
	// multi-turn + empty positionals is allowed (no os.Exit).
	if got := collectRunPrompt(nil, true, false); got != "" {
		t.Fatalf("empty multi-turn prompt = %q", got)
	}
	if got := collectRunPrompt(nil, false, true); got != "" {
		t.Fatalf("empty permsJSON prompt = %q", got)
	}
}

func TestExitCodeForRunError(t *testing.T) {
	if got := exitCodeForRunError(context.Canceled); got != 130 {
		t.Errorf("canceled = %d, want 130", got)
	}
	if got := exitCodeForRunError(fmt.Errorf("wrap: %w", context.Canceled)); got != 130 {
		t.Errorf("wrapped canceled = %d, want 130", got)
	}
	if got := exitCodeForRunError(errors.New("boom")); got != 1 {
		t.Errorf("other = %d, want 1", got)
	}
}

func TestEmitRunSessionAndDone(t *testing.T) {
	cap := captureStdout(t)
	emitRunSession("ses_abc", true)
	emitRunDone("ses_abc", true, true)
	emitRunError("ses_abc", errors.New("fail"), true)
	out := cap.read()

	var types []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("invalid JSON %q: %v", line, err)
		}
		types = append(types, evt["type"].(string))
		switch evt["type"] {
		case "session", "done", "error":
			if evt["sessionID"] != "ses_abc" {
				t.Errorf("%s sessionID = %v", evt["type"], evt["sessionID"])
			}
		}
		if evt["type"] == "done" && evt["ok"] != true {
			t.Errorf("done.ok = %v", evt["ok"])
		}
		if evt["type"] == "error" && evt["message"] != "fail" {
			t.Errorf("error.message = %v", evt["message"])
		}
	}
	if strings.Join(types, ",") != "session,done,error" {
		t.Errorf("types = %v", types)
	}
}

func TestTruncateNDJSONOutput(t *testing.T) {
	out, trunc := truncateNDJSONOutput("hi", 10)
	if out != "hi" || trunc {
		t.Fatalf("got %q trunc=%v", out, trunc)
	}
	long := strings.Repeat("a", maxNDJSONToolOutput+10)
	out, trunc = truncateNDJSONOutput(long, maxNDJSONToolOutput)
	if !trunc || len(out) != maxNDJSONToolOutput {
		t.Fatalf("len=%d trunc=%v", len(out), trunc)
	}
}

func TestStreamRunOutput_ToolEndTruncatedFlag(t *testing.T) {
	b := bus.New()
	defer b.Close()
	cap := captureStdout(t)
	streamRunOutput(b, true, nil)

	b.Publish("session.tool.result", map[string]any{
		"toolName":   "shell",
		"toolCallID": "c1",
		"output":     strings.Repeat("x", maxNDJSONToolOutput+5),
		"isError":    false,
	})
	time.Sleep(50 * time.Millisecond)
	out := cap.read()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt["type"] == "tool_end" {
			if evt["truncated"] != true {
				t.Fatalf("truncated = %v, want true", evt["truncated"])
			}
			outStr, _ := evt["output"].(string)
			if len(outStr) != maxNDJSONToolOutput {
				t.Fatalf("output len = %d", len(outStr))
			}
			return
		}
	}
	t.Fatalf("tool_end not seen in %q", out)
}

func TestStreamRunOutput_ReasoningDelta(t *testing.T) {
	b := bus.New()
	defer b.Close()
	cap := captureStdout(t)
	streamRunOutput(b, true, nil)

	b.Publish("session.reasoning.delta", map[string]any{
		"sessionID": "ses_r",
		"text":      "thinking",
	})
	time.Sleep(50 * time.Millisecond)
	out := cap.read()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt["type"] == "reasoning" && evt["text"] == "thinking" {
			return
		}
	}
	t.Fatalf("reasoning delta not streamed in %q", out)
}

func TestWriteNDJSON_SerializesLines(t *testing.T) {
	cap := captureStdout(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			writeNDJSON(map[string]any{"type": "text", "text": fmt.Sprintf("%d", n)})
		}(i)
	}
	wg.Wait()
	out := strings.TrimSpace(cap.read())
	for _, line := range strings.Split(out, "\n") {
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("interleaved/invalid JSON line %q: %v", line, err)
		}
	}
}

func TestEmitRunDone_OkFalse(t *testing.T) {
	cap := captureStdout(t)
	emitRunDone("ses_x", true, false)
	out := strings.TrimSpace(cap.read())
	var evt map[string]any
	if err := json.Unmarshal([]byte(out), &evt); err != nil {
		t.Fatal(err)
	}
	if evt["ok"] != false {
		t.Errorf("done.ok = %v, want false", evt["ok"])
	}
}

func TestStreamRunOutput_ToolErrorTracked(t *testing.T) {
	b := bus.New()
	defer b.Close()
	tracker := &runToolErrorTracker{}
	_ = captureStdout(t)
	streamRunOutput(b, true, tracker)

	b.Publish("session.tool.result", map[string]any{
		"toolName":   "shell",
		"toolCallID": "c1",
		"output":     "denied",
		"isError":    true,
	})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tracker.seen() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("tool error not tracked")
}

func TestStreamRunOutput_ToolOKNotTracked(t *testing.T) {
	b := bus.New()
	defer b.Close()
	tracker := &runToolErrorTracker{}
	_ = captureStdout(t)
	streamRunOutput(b, true, tracker)

	b.Publish("session.tool.result", map[string]any{
		"toolName":   "shell",
		"toolCallID": "c1",
		"output":     "ok",
		"isError":    false,
	})
	time.Sleep(50 * time.Millisecond)
	if tracker.seen() {
		t.Fatal("ok tool_end should not mark tool error")
	}
}

func TestStreamRunOutput_TextToolProgressOnStderr(t *testing.T) {
	b := bus.New()
	defer b.Close()

	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = oldErr
		_ = r.Close()
	}()

	streamRunOutput(b, false, nil)
	b.Publish("session.tool.begin", map[string]any{
		"toolName": "shell", "toolCallID": "c1",
	})
	b.Publish("session.tool.result", map[string]any{
		"toolName": "shell", "toolCallID": "c1",
		"output": "ok", "isError": false,
	})
	time.Sleep(50 * time.Millisecond)
	_ = w.Close()
	os.Stderr = oldErr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	stderr := string(buf[:n])
	if !strings.Contains(stderr, "tool shell begin") {
		t.Errorf("stderr missing begin: %q", stderr)
	}
	if !strings.Contains(stderr, "tool shell end (ok)") {
		t.Errorf("stderr missing end: %q", stderr)
	}
}

func TestResolveRunSession_ListError(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	store := session.NewStore(db.DB)
	info, err := store.Create(session.CreateInput{
		ProjectID: project.IDFromDirectory(dir),
		Directory: dir,
		Title:     "test",
		Model:     &session.ModelRef{ModelID: "m", ProviderID: "p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, 1, 1, ?)`,
		"msg_corrupt", info.ID, "not-json",
	); err != nil {
		t.Fatal(err)
	}

	_, _, _, err = resolveRunSession(db, info.ID, "", false, "", dir, "build", "m", "p")
	if err == nil {
		t.Fatal("expected error listing messages")
	}
	if !strings.Contains(err.Error(), "list messages") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveResumeSessionID(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := session.NewStore(db.DB)
	info, err := store.Create(session.CreateInput{
		ProjectID: "proj_test",
		Directory: "/tmp",
		Title:     "auth refactor",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := resolveResumeSessionID(store, "proj_test", info.ID); got != info.ID {
		t.Errorf("by ID: got %q want %q", got, info.ID)
	}
	if got := resolveResumeSessionID(store, "proj_test", "auth refactor"); got != info.ID {
		t.Errorf("by title: got %q want %q", got, info.ID)
	}
	if got := resolveResumeSessionID(store, "proj_test", info.Slug); got != info.ID {
		t.Errorf("by slug: got %q want %q", got, info.ID)
	}
	if got := resolveResumeSessionID(store, "proj_test", "missing"); got != "" {
		t.Errorf("missing: got %q", got)
	}
}

func TestStdinMux_PermissionReplyDuringAsk(t *testing.T) {
	b := bus.New()
	defer b.Close()
	permSvc := permission.NewService(b)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
		_ = w.Close()
	})

	replyCh := make(chan permission.ReplyInput, 16)
	// Wire the JSON permission responder that drains replyCh into RespondToAsk.
	_ = setupRunPermissions(b, permSvc, false, false, "json")
	// Replace the channel setupRunPermissions created — feed our mux replies
	// into the same RespondToAsk path via a dedicated forwarder.
	go func() {
		for reply := range replyCh {
			for attempt := 0; attempt < 200; attempt++ {
				err := permSvc.RespondToAsk(reply)
				if err == nil || !errors.Is(err, permission.ErrNotFound) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	mux := newStdinMux(true, replyCh)
	go mux.run()

	askDone := make(chan error, 1)
	go func() {
		askDone <- permSvc.Ask(t.Context(), permission.AskInput{
			ID:         "perm_mux_1",
			SessionID:  "ses_1",
			Permission: "shell",
			Patterns:   []string{"echo hi"},
		})
	}()

	time.Sleep(30 * time.Millisecond)

	// Queue a prompt (would fill a buffer-1 channel) then the reply.
	if _, err := fmt.Fprintf(w, "{\"type\":\"prompt\",\"text\":\"hello\"}\n{\"type\":\"permission_reply\",\"id\":\"perm_mux_1\",\"reply\":\"once\"}\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-askDone:
		if err != nil {
			t.Fatalf("Ask error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask did not complete — permission reply likely blocked behind prompt send")
	}

	select {
	case p := <-mux.prompts:
		if p != "hello" {
			t.Errorf("prompt = %q", p)
		}
	case <-time.After(time.Second):
		t.Fatal("prompt not delivered")
	}
}

func TestReadNextPrompt_CancelDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mux := newStdinMux(true, nil)

	done := make(chan struct{})
	var text string
	var ok bool
	go func() {
		text, ok = readNextPrompt(ctx, nil, true, nil, mux)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		if ok || text != "" {
			t.Fatalf("got (%q, %v), want empty+false on cancel", text, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("readNextPrompt did not return promptly on cancel")
	}
}

func TestReadMuxPrompt_CancelDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mux := newStdinMux(false, nil)
	done := make(chan struct{})
	go func() {
		_, _ = readMuxPrompt(ctx, mux)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readMuxPrompt did not return on cancel")
	}
}

func TestWireRunSubagentRunner_SetsFunc(t *testing.T) {
	toolCtx := &tool.Context{}
	if toolCtx.SubagentRunner != nil {
		t.Fatal("expected nil before wire")
	}
	// server.New with minimal deps is enough to attach a non-nil runner closure.
	b := bus.New()
	defer b.Close()
	srv := server.New(server.Config{Directory: t.TempDir()}, server.Dependencies{
		Bus:          b,
		Registry:     nil,
		ToolRegistry: tool.NewRegistry(&tool.Context{Directory: t.TempDir()}),
		PermService:  permission.NewService(b),
	})
	wireRunSubagentRunner(toolCtx, srv)
	if toolCtx.SubagentRunner == nil {
		t.Fatal("expected SubagentRunner to be set")
	}
}

func TestValidateRunStdinFlags(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		multiTurn   bool
		permsJSON   bool
		formatJSON  bool
		stdinIsTTY  bool
		wantErr     string
	}{
		{name: "ok default", stdinIsTTY: true},
		{name: "ok json perms", permsJSON: true, formatJSON: true, stdinIsTTY: true},
		{name: "ok interactive tty", interactive: true, stdinIsTTY: true},
		{name: "i+multi-turn", interactive: true, multiTurn: true, stdinIsTTY: true, wantErr: "multi-turn"},
		{name: "i+perms json", interactive: true, permsJSON: true, formatJSON: true, stdinIsTTY: true, wantErr: "permissions json"},
		{name: "i+non-tty", interactive: true, stdinIsTTY: false, wantErr: "TTY"},
		{name: "perms without format json", permsJSON: true, formatJSON: false, stdinIsTTY: true, wantErr: "--format json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunStdinFlags(tt.interactive, tt.multiTurn, tt.permsJSON, tt.formatJSON, tt.stdinIsTTY)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePermissionsFlag(t *testing.T) {
	if err := validatePermissionsFlag("default"); err != nil {
		t.Errorf("default: %v", err)
	}
	if err := validatePermissionsFlag("json"); err != nil {
		t.Errorf("json: %v", err)
	}
	err := validatePermissionsFlag("always")
	if err == nil {
		t.Fatal("expected error for unknown permissions mode")
	}
	if !strings.Contains(err.Error(), "default or json") {
		t.Errorf("error = %q, want valid values listed", err)
	}
}

func TestNormalizePermissionReply(t *testing.T) {
	tests := []struct {
		in      string
		want    permission.Reply
		wantOK  bool
	}{
		{"once", permission.ReplyOnce, true},
		{"always", permission.ReplyAlways, true},
		{"reject", permission.ReplyReject, true},
		{"", permission.ReplyReject, false},
		{"yes", permission.ReplyReject, false},
		{"allow", permission.ReplyReject, false},
	}
	for _, tt := range tests {
		got, ok := normalizePermissionReply(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("normalizePermissionReply(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestJSONPermissionProtocol_EmptyReplyRejects(t *testing.T) {
	b := bus.New()
	defer b.Close()
	permSvc := permission.NewService(b)
	_ = captureStdout(t)
	permReplyCh := setupRunPermissions(b, permSvc, false, false, "json")

	askDone := make(chan error, 1)
	go func() {
		askDone <- permSvc.Ask(t.Context(), permission.AskInput{
			ID:         "perm_empty",
			SessionID:  "ses_1",
			Permission: "shell",
			Patterns:   []string{"echo"},
		})
	}()
	time.Sleep(50 * time.Millisecond)

	permReplyCh <- permission.ReplyInput{RequestID: "perm_empty", Reply: ""}

	select {
	case err := <-askDone:
		if err == nil {
			t.Fatal("expected rejection for empty reply, got nil")
		}
		if !errors.Is(err, permission.ErrRejected) {
			t.Errorf("err = %v, want ErrRejected", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask did not resolve")
	}
}

func TestJSONPermissionProtocol_RetryExhaustionForceRejects(t *testing.T) {
	b := bus.New()
	defer b.Close()
	permSvc := permission.NewService(b)
	_ = captureStdout(t)
	permReplyCh := setupRunPermissions(b, permSvc, false, false, "json")

	// Reply arrives before Ask is registered. Retries + force-reject must
	// resolve the Ask once it appears (no hang).
	permReplyCh <- permission.ReplyInput{RequestID: "late_ask", Reply: permission.ReplyOnce}

	time.Sleep(50 * time.Millisecond)
	askDone := make(chan error, 1)
	go func() {
		askDone <- permSvc.Ask(t.Context(), permission.AskInput{
			ID:         "late_ask",
			SessionID:  "ses_1",
			Permission: "shell",
			Patterns:   []string{"echo"},
		})
	}()

	select {
	case err := <-askDone:
		// once may win if Ask registers during retry; otherwise force-reject.
		if err != nil && !errors.Is(err, permission.ErrRejected) {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ask hung after reply exhaustion — force-reject failed")
	}
}

func TestJSONPermissionProtocol_EmitsMetadata(t *testing.T) {
	b := bus.New()
	defer b.Close()
	permSvc := permission.NewService(b)
	cap := captureStdout(t)
	permReplyCh := setupRunPermissions(b, permSvc, false, false, "json")

	askDone := make(chan error, 1)
	go func() {
		askDone <- permSvc.Ask(t.Context(), permission.AskInput{
			ID:         "perm_meta",
			SessionID:  "ses_1",
			Permission: "shell",
			Patterns:   []string{"ls"},
			Metadata:   map[string]any{"command": "ls"},
		})
	}()
	time.Sleep(50 * time.Millisecond)
	permReplyCh <- permission.ReplyInput{RequestID: "perm_meta", Reply: permission.ReplyReject}

	select {
	case <-askDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Ask did not resolve")
	}

	output := cap.read()
	found := false
	for _, l := range strings.Split(strings.TrimSpace(output), "\n") {
		var evt map[string]any
		if err := json.Unmarshal([]byte(l), &evt); err != nil {
			continue
		}
		if evt["type"] == "permission" && evt["id"] == "perm_meta" {
			found = true
			meta, ok := evt["metadata"].(map[string]any)
			if !ok || meta["command"] != "ls" {
				t.Errorf("metadata = %v, want command=ls", evt["metadata"])
			}
		}
	}
	if !found {
		t.Errorf("permission event not found in %q", output)
	}
}

func TestFailClosedQuestion_HeadlessRejects(t *testing.T) {
	b := bus.New()
	defer b.Close()

	setupRunQuestionReject(b)

	rejected := b.Subscribe("question.rejected")
	defer rejected.Unsubscribe()

	tc := &tool.Context{SessionID: "ses_run_q", Bus: b}
	def := tool.QuestionTool()
	done := make(chan *tool.ExecuteResult, 1)
	go func() {
		result, err := def.Execute(context.Background(), tc, json.RawMessage(`{"question":"Continue?"}`))
		if err != nil {
			t.Errorf("Execute error: %v", err)
		}
		done <- result
	}()

	select {
	case evt := <-rejected.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("question.rejected properties not a map")
		}
		if props["requestID"] == "" {
			t.Error("requestID empty")
		}
		if props["sessionID"] != "ses_run_q" {
			t.Errorf("sessionID = %v, want ses_run_q", props["sessionID"])
		}
		reason, _ := props["reason"].(string)
		if !strings.Contains(reason, "non-interactive") {
			t.Errorf("reason = %q, want mention of non-interactive", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question.rejected")
	}

	select {
	case result := <-done:
		if result == nil || !result.IsError {
			t.Fatalf("result = %+v, want IsError=true", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("question tool did not return within 2s")
	}
}


