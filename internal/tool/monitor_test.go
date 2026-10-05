package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRingBuffer_AddAndDrain(t *testing.T) {
	rb := newRingBuffer(5)
	rb.Add("line1")
	rb.Add("line2")
	rb.Add("line3")

	if rb.Len() != 3 {
		t.Errorf("expected 3 lines, got %d", rb.Len())
	}

	result := rb.Drain()
	if result != "line1\nline2\nline3" {
		t.Errorf("unexpected drain result: %q", result)
	}

	if rb.Len() != 0 {
		t.Errorf("expected 0 lines after drain, got %d", rb.Len())
	}
}

func TestRingBuffer_OverflowKeepsLastN(t *testing.T) {
	rb := newRingBuffer(3)
	for i := 0; i < 10; i++ {
		rb.Add(strings.Repeat("x", i+1))
	}

	if rb.Len() != 3 {
		t.Errorf("expected 3 lines, got %d", rb.Len())
	}

	result := rb.Drain()
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines after drain, got %d", len(lines))
	}
	// Last 3 lines should be the ones with length 8, 9, 10
	if lines[0] != strings.Repeat("x", 8) {
		t.Errorf("expected line of 8 x's, got %q", lines[0])
	}
	if lines[2] != strings.Repeat("x", 10) {
		t.Errorf("expected line of 10 x's, got %q", lines[2])
	}
}

func TestRingBuffer_100Lines(t *testing.T) {
	rb := newRingBuffer(ringBufferSize)
	for i := 0; i < 200; i++ {
		rb.Add("line")
	}
	if rb.Len() != ringBufferSize {
		t.Errorf("expected %d lines, got %d", ringBufferSize, rb.Len())
	}
}

func TestRingBuffer_DrainEmpty(t *testing.T) {
	rb := newRingBuffer(5)
	if result := rb.Drain(); result != "" {
		t.Errorf("expected empty string from empty buffer, got %q", result)
	}
}

func TestMonitorManager_StartAndStop(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	id, err := mm.Start(context.Background(), "sleep 5", "test sleep", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty monitor ID")
	}

	monitors := mm.List()
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}
	if monitors[0].ID != id {
		t.Errorf("expected ID %s, got %s", id, monitors[0].ID)
	}
	if !monitors[0].Running {
		t.Error("expected monitor to be running")
	}

	if err := mm.Stop(id); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	monitors = mm.List()
	if len(monitors) != 0 {
		t.Errorf("expected 0 monitors after stop, got %d", len(monitors))
	}
}

func TestMonitorManager_DrainAll(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	id, err := mm.Start(context.Background(), "echo hello; echo world", "echo test", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait for the command to finish producing output.
	time.Sleep(500 * time.Millisecond)

	output := mm.DrainAll()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", output)
	}
	if !strings.Contains(output, "world") {
		t.Errorf("expected output to contain 'world', got %q", output)
	}
	if !strings.Contains(output, id) {
		t.Errorf("expected output to contain monitor ID %s, got %q", id, output)
	}

	// Second drain should return empty (buffer was cleared and process exited).
	output2 := mm.DrainAll()
	if output2 != "" {
		t.Errorf("expected empty output on second drain, got %q", output2)
	}
}

func TestMonitorManager_ConcurrentCap(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	dir := t.TempDir()
	ids := make([]string, 0, maxMonitors)
	for i := 0; i < maxMonitors; i++ {
		id, err := mm.Start(context.Background(), "sleep 2", "test", dir, 5*time.Second)
		if err != nil {
			t.Fatalf("start %d failed: %v", i, err)
		}
		ids = append(ids, id)
	}
	_ = ids

	// 6th monitor should fail.
	_, err := mm.Start(context.Background(), "sleep 2", "test", dir, 5*time.Second)
	if err == nil {
		t.Fatal("expected error when exceeding max concurrent monitors")
	}
	if !strings.Contains(err.Error(), "maximum concurrent monitors") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMonitorManager_TimeoutEnforcement(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	id, err := mm.Start(context.Background(), "sleep 5", "timeout test", t.TempDir(), 500*time.Millisecond)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait for the timeout to expire.
	time.Sleep(1 * time.Second)

	monitors := mm.List()
	for _, m := range monitors {
		if m.ID == id && m.Running {
			t.Error("expected monitor to have stopped after timeout")
		}
	}
}

func TestMonitorManager_Shutdown(t *testing.T) {
	mm := NewMonitorManager()

	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		_, err := mm.Start(context.Background(), "sleep 2", "test", dir, 5*time.Second)
		if err != nil {
			t.Fatalf("start failed: %v", err)
		}
	}

	mm.Shutdown()

	monitors := mm.List()
	if len(monitors) != 0 {
		t.Errorf("expected 0 monitors after shutdown, got %d", len(monitors))
	}
}

func TestMonitorTool_StartViaExecute(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command":"echo test_output","description":"echo test"}`)
	output, isErr, err := r.Execute(context.Background(), "monitor", args, "ses-mon")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Errorf("expected no error, got: %s", output)
	}
	if !strings.Contains(output, "Monitor started") {
		t.Errorf("expected 'Monitor started' in output, got %q", output)
	}
	if !strings.Contains(output, "mon_") {
		t.Errorf("expected monitor ID in output, got %q", output)
	}
}

func TestMonitorTool_ListAction(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	// Start a monitor first.
	startArgs := json.RawMessage(`{"command":"sleep 2","description":"list test"}`)
	r.Execute(context.Background(), "monitor", startArgs, "ses-list")

	// List monitors.
	listArgs := json.RawMessage(`{"action":"list"}`)
	output, isErr, err := r.Execute(context.Background(), "monitor", listArgs, "ses-list")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Errorf("expected no error, got: %s", output)
	}

	var monitors []MonitorInfo
	if err := json.Unmarshal([]byte(output), &monitors); err != nil {
		t.Fatalf("failed to unmarshal monitor list: %v", err)
	}
	if len(monitors) != 1 {
		t.Errorf("expected 1 monitor, got %d", len(monitors))
	}
}

func TestMonitorTool_StopAction(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	// Start a monitor.
	startArgs := json.RawMessage(`{"command":"sleep 2","description":"stop test"}`)
	output, _, _ := r.Execute(context.Background(), "monitor", startArgs, "ses-stop")

	// Extract monitor ID from output.
	// Output format: "Monitor started: stop test (ID: mon_1)\n..."
	monID := extractMonitorID(t, output)
	if monID == "" {
		t.Fatalf("could not extract monitor ID from: %q", output)
	}

	// Stop it.
	stopArgs := json.RawMessage(`{"action":"stop","monitor_id":"` + monID + `"}`)
	output, isErr, err := r.Execute(context.Background(), "monitor", stopArgs, "ses-stop")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Errorf("expected no error, got: %s", output)
	}
	if !strings.Contains(output, "stopped") {
		t.Errorf("expected 'stopped' in output, got %q", output)
	}
}

// extractMonitorID finds "ID: mon_N" in the output and returns "mon_N".
func extractMonitorID(t *testing.T, output string) string {
	t.Helper()
	idx := strings.Index(output, "ID: ")
	if idx < 0 {
		return ""
	}
	rest := output[idx+4:]
	end := strings.IndexAny(rest, ")\n ")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func TestMonitorManager_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mm := NewMonitorManager()
	defer mm.Shutdown()

	_, err := mm.Start(ctx, "sleep 2", "cancel test", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Cancel the parent context.
	cancel()
	time.Sleep(200 * time.Millisecond)

	monitors := mm.List()
	for _, m := range monitors {
		if m.Running {
			t.Error("expected monitor to stop after context cancellation")
		}
	}
}

func TestMonitorTool_EmptyCommand(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command":""}`)
	output, isErr, _ := r.Execute(context.Background(), "monitor", args, "ses-empty")
	if !isErr {
		t.Error("expected error for empty command")
	}
	if !strings.Contains(output, "command is required") {
		t.Errorf("expected 'command is required' error, got %q", output)
	}
}
