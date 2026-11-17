package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

func TestRingBuffer_AddAndDrain(t *testing.T) {
	rb := newRingBuffer(5)
	rb.Add("line1")
	rb.Add("line2")
	rb.Add("line3")

	if rb.Len() != 3 {
		t.Errorf("expected 3 lines, got %d", rb.Len())
	}

	result, _ := rb.Drain()
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

	result, _ := rb.Drain()
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
	if result, _ := rb.Drain(); result != "" {
		t.Errorf("expected empty string from empty buffer, got %q", result)
	}
}

func TestMonitorManager_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	mm := NewMonitorManager()
	defer mm.Shutdown()

	if _, err := mm.Start(context.Background(), `printf 'ran:%s\n' "$OPENROUTER_API_KEY"`, "env", t.TempDir(), 5*time.Second); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		got += mm.DrainAll()
		if strings.Contains(got, "ran:") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Contains(got, "super-secret") {
		t.Fatalf("monitor leaked the API key: %q", got)
	}
	if !strings.Contains(got, "ran:") {
		t.Fatalf("monitor output = %q", got)
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

func TestMonitorTool_BlocksSecretAccess(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command":"cat .env","description":"secret"}`)
	output, isErr, err := r.Execute(context.Background(), "monitor", args, "ses-secret")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !isErr {
		t.Fatalf("expected error for secret access, got: %s", output)
	}
	if !strings.Contains(output, "Access to secret") {
		t.Errorf("expected secret block message, got %q", output)
	}
}

func TestMonitorTool_SecretAsksWhenShellIsAllowed(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	defer perms.Close()

	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		Bus:       b,
		Perms:     perms,
		Ruleset: permission.Ruleset{
			{Permission: "shell", Pattern: "*", Action: permission.ActionAllow},
		},
	})
	RegisterBuiltins(r)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output string
	var isErr bool
	go func() {
		output, isErr, _ = r.Execute(ctx, "monitor", json.RawMessage(`{"command":"cat .env"}`), "ses-secret")
		close(done)
	}()

	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "secret-shell" {
		t.Fatalf("permission %q, want secret-shell", req.Permission)
	}
	if err := perms.RespondToAsk(permission.ReplyInput{
		RequestID: req.ID,
		Reply:     permission.ReplyReject,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for rejected monitor command")
	}
	if !isErr {
		t.Fatalf("expected rejection to fail the monitor, got %q", output)
	}
}

func TestMonitorTool_BlocksDestructiveWithoutPerms(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command":"rm -rf /tmp/x","description":"destructive"}`)
	output, isErr, err := r.Execute(context.Background(), "monitor", args, "ses-dest")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !isErr {
		t.Fatalf("expected error for destructive command without perms, got: %s", output)
	}
}

func TestMonitorManager_WriteStdin(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	// Start cat, which echoes stdin to stdout.
	id, err := mm.Start(context.Background(), "cat", "stdin test", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	output, truncated, err := mm.WriteStdin(id, "hello\n")
	if err != nil {
		t.Fatalf("write stdin failed: %v", err)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", output)
	}
	if truncated {
		t.Error("unexpected truncation")
	}
}

func TestMonitorManager_WriteStdin_MultipleWrites(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	id, err := mm.Start(context.Background(), "cat", "multi stdin", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	out1, _, err := mm.WriteStdin(id, "first\n")
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if !strings.Contains(out1, "first") {
		t.Errorf("expected 'first' in output, got %q", out1)
	}

	out2, _, err := mm.WriteStdin(id, "second\n")
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if !strings.Contains(out2, "second") {
		t.Errorf("expected 'second' in output, got %q", out2)
	}
}

func TestMonitorManager_WriteStdin_StoppedProcess(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	id, err := mm.Start(context.Background(), "echo done", "exit test", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait for the process to exit.
	time.Sleep(500 * time.Millisecond)

	_, _, err = mm.WriteStdin(id, "text\n")
	if err == nil {
		t.Fatal("expected error writing to exited process")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected 'exited' in error, got %v", err)
	}
}

func TestMonitorManager_Command(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	id, err := mm.Start(context.Background(), "sleep 2", "cmd test", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	cmd := mm.Command(id)
	if cmd != "sleep 2" {
		t.Errorf("expected command 'sleep 2', got %q", cmd)
	}

	if got := mm.Command("nonexistent"); got != "" {
		t.Errorf("expected empty command for nonexistent monitor, got %q", got)
	}
}

func TestMonitorTool_StdinAction(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	// Start cat.
	startArgs := json.RawMessage(`{"command":"cat","description":"stdin tool test"}`)
	startOut, isErr, err := r.Execute(context.Background(), "monitor", startArgs, "ses-stdin")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Fatalf("start failed: %s", startOut)
	}

	monID := extractMonitorID(t, startOut)
	if monID == "" {
		t.Fatalf("could not extract monitor ID from: %q", startOut)
	}

	// Write stdin.
	stdinArgs := json.RawMessage(fmt.Sprintf(`{"action":"stdin","monitor_id":%q,"input":"test_line\n"}`, monID))
	output, isErr, err := r.Execute(context.Background(), "monitor", stdinArgs, "ses-stdin")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Errorf("stdin action failed: %s", output)
	}
	if !strings.Contains(output, "test_line") {
		t.Errorf("expected 'test_line' in output, got %q", output)
	}
}

func TestMonitorTool_StdinAction_MissingFields(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	// Missing monitor_id.
	args := json.RawMessage(`{"action":"stdin","input":"text"}`)
	output, isErr, _ := r.Execute(context.Background(), "monitor", args, "ses-stdin")
	if !isErr {
		t.Error("expected error for missing monitor_id")
	}
	if !strings.Contains(output, "monitor_id is required") {
		t.Errorf("expected 'monitor_id is required', got %q", output)
	}

	// Missing input.
	args = json.RawMessage(`{"action":"stdin","monitor_id":"mon_1"}`)
	output, isErr, _ = r.Execute(context.Background(), "monitor", args, "ses-stdin")
	if !isErr {
		t.Error("expected error for missing input")
	}
	if !strings.Contains(output, "input is required") {
		t.Errorf("expected 'input is required', got %q", output)
	}
}

func TestRingBuffer_ByteCap(t *testing.T) {
	rb := newRingBuffer(1000)
	rb.maxBytes = 100 // small cap for testing

	// Add lines until over cap.
	for i := 0; i < 20; i++ {
		rb.Add(strings.Repeat("x", 10))
	}

	result, truncated := rb.Drain()
	if !truncated {
		t.Error("expected truncation flag to be set")
	}
	// Total bytes should be <= 100.
	if len(result) > 110 { // some slack for newline joins
		t.Errorf("output too large after truncation: %d bytes", len(result))
	}
}
