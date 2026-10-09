package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestMCPHelperProcess — mock MCP server for stdio transport integration tests.
//
// This is NOT a real test. It is invoked as a child process by setting
// GO_MCP_HELPER=1. Behaviour is controlled via GO_MCP_HELPER_BEHAVIOR:
//
//   - "normal"          — full protocol: initialize, list tools, call tool
//   - "paginated"       — return tools in two pages via nextCursor
//   - "tool_error"      — return isError=true from tools/call
//   - "rpc_error"       — return a JSON-RPC error from tools/call
//   - "timeout_on_call" — never respond to tools/call (trigger timeout)
//   - "notify"          — emit a notification after initialized
// ---------------------------------------------------------------------------

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_MCP_HELPER") != "1" {
		return
	}
	behavior := os.Getenv("GO_MCP_HELPER_BEHAVIOR")
	if behavior == "" {
		behavior = "normal"
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var req struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      int64          `json:"id,omitempty"`
			Method  string         `json:"method"`
			Params  map[string]any `json:"params,omitempty"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			stdioHelperWriteResult(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "mock-mcp-server"},
			})

		case "notifications/initialized":
			// Write to stderr to exercise cappedBuffer capture.
			fmt.Fprintln(os.Stderr, "mock server ready")
			if behavior == "notify" {
				stdioHelperWriteNotification("notifications/tools/list_changed")
			}

		case "tools/list":
			stdioHelperHandleToolsList(req.ID, req.Params, behavior)

		case "tools/call":
			stdioHelperHandleToolsCall(req.ID, req.Params, behavior)
		}
	}
	os.Exit(0)
}

func stdioHelperWriteResult(id int64, result any) {
	resultJSON, _ := json.Marshal(result)
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  json.RawMessage(resultJSON),
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

func stdioHelperWriteError(id int64, code int, message string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

func stdioHelperWriteNotification(method string) {
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	data, _ := json.Marshal(notif)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

func stdioHelperHandleToolsList(id int64, params map[string]any, behavior string) {
	if behavior == "paginated" {
		cursor, _ := params["cursor"].(string)
		if cursor == "" {
			stdioHelperWriteResult(id, map[string]any{
				"tools": []map[string]any{
					{"name": "tool_a", "description": "First tool"},
				},
				"nextCursor": "page2",
			})
		} else {
			stdioHelperWriteResult(id, map[string]any{
				"tools": []map[string]any{
					{"name": "tool_b", "description": "Second tool"},
				},
			})
		}
		return
	}

	stdioHelperWriteResult(id, map[string]any{
		"tools": []map[string]any{
			{
				"name":        "echo",
				"description": "Echoes input",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"message": map[string]any{"type": "string"},
					},
				},
			},
			{
				"name":        "add",
				"description": "Adds numbers",
			},
		},
	})
}

func stdioHelperHandleToolsCall(id int64, params map[string]any, behavior string) {
	switch behavior {
	case "timeout_on_call":
		// Don't write any response. Return to the scanner loop so the
		// process stays alive reading stdin while the transport times out.
		return
	case "tool_error":
		stdioHelperWriteResult(id, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "something went wrong"},
			},
			"isError": true,
		})
	case "rpc_error":
		stdioHelperWriteError(id, -32000, "internal server error")
	default:
		name, _ := params["name"].(string)
		stdioHelperWriteResult(id, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("result from %s", name)},
			},
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: create a stdio transport backed by the mock MCP server process
// ---------------------------------------------------------------------------

func newTestStdioTransport(t *testing.T, behavior string) *StdioTransport {
	t.Helper()
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=^TestMCPHelperProcess$"},
		map[string]string{
			"GO_MCP_HELPER":          "1",
			"GO_MCP_HELPER_BEHAVIOR": behavior,
		},
	)
}

// ---------------------------------------------------------------------------
// Stdio transport integration tests
// ---------------------------------------------------------------------------

func TestStdioTransport_Connect_PerformsInitializeHandshake(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	if transport.cmd == nil || transport.cmd.Process == nil {
		t.Fatal("expected running child process after connect")
	}
	if transport.cmd.Process.Pid <= 0 {
		t.Errorf("expected positive PID, got %d", transport.cmd.Process.Pid)
	}
}

func TestStdioTransport_Connect_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	out := filepath.Join(t.TempDir(), "env.txt")
	script := `printf '%s|%s' "$OPENROUTER_API_KEY" "$MCP_TOKEN" > ` + strconvQuote(out)
	transport := NewStdioTransport("sh", []string{"-c", script}, map[string]string{"MCP_TOKEN": "from-config"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The shell exits after writing the file, so the MCP handshake fails.
	// The child environment is applied before that handshake.
	if err := transport.Connect(ctx); err != nil && !strings.Contains(err.Error(), "MCP initialize") {
		t.Fatal(err)
	}
	t.Cleanup(func() { transport.Close() })

	deadline := time.Now().Add(2 * time.Second)
	var got []byte
	var err error
	for time.Now().Before(deadline) {
		got, err = os.ReadFile(out)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "|from-config" {
		t.Fatalf("child env = %q, want the configured token and not the parent API key", got)
	}
}

func strconvQuote(path string) string {
	return `'` + strings.ReplaceAll(path, `'`, `'\''`) + `'`
}

func TestStdioTransport_Connect_FailsOnBadCommand(t *testing.T) {
	transport := NewStdioTransport("nonexistent-binary-xyz-8675309", nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	if err == nil {
		transport.Close()
		t.Fatal("expected error for nonexistent command")
	}
	if !strings.Contains(err.Error(), "starting MCP server") {
		t.Errorf("error = %q, expected to contain 'starting MCP server'", err.Error())
	}
}

func TestStdioTransport_ListTools_ReturnsServerTools(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	tools, err := transport.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Errorf("tools[0].Name = %q, want echo", tools[0].Name)
	}
	if tools[0].Description != "Echoes input" {
		t.Errorf("tools[0].Description = %q, want 'Echoes input'", tools[0].Description)
	}
	if tools[1].Name != "add" {
		t.Errorf("tools[1].Name = %q, want add", tools[1].Name)
	}
}

func TestStdioTransport_ListTools_HandlesPagination(t *testing.T) {
	transport := newTestStdioTransport(t, "paginated")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	tools, err := transport.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools from two pages, got %d", len(tools))
	}
	if tools[0].Name != "tool_a" {
		t.Errorf("first page tool = %q, want tool_a", tools[0].Name)
	}
	if tools[1].Name != "tool_b" {
		t.Errorf("second page tool = %q, want tool_b", tools[1].Name)
	}
}

func TestStdioTransport_CallTool_ReturnsResult(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	result, err := transport.CallTool(ctx, "echo", json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result != "result from echo" {
		t.Errorf("result = %q, want 'result from echo'", result)
	}
}

func TestStdioTransport_CallTool_ServerToolError(t *testing.T) {
	transport := newTestStdioTransport(t, "tool_error")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	_, err := transport.CallTool(ctx, "failing", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for isError response")
	}
	if !strings.Contains(err.Error(), "tool error") {
		t.Errorf("error = %q, expected to contain 'tool error'", err.Error())
	}
}

func TestStdioTransport_CallTool_RPCError(t *testing.T) {
	transport := newTestStdioTransport(t, "rpc_error")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	_, err := transport.CallTool(ctx, "broken", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for JSON-RPC error response")
	}
	if !strings.Contains(err.Error(), "internal server error") {
		t.Errorf("error = %q, expected to contain 'internal server error'", err.Error())
	}
}

func TestStdioTransport_CallTool_TimesOut(t *testing.T) {
	transport := newTestStdioTransport(t, "timeout_on_call")
	transport.Timeout = 500 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	start := time.Now()
	_, err := transport.CallTool(ctx, "slow", json.RawMessage(`{}`))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %q, expected to contain 'timeout'", err.Error())
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout took %v, expected around 500ms", elapsed)
	}
}

func TestStdioTransport_Close_TerminatesProcess(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	pid := transport.cmd.Process.Pid
	if pid <= 0 {
		t.Fatal("expected positive PID before close")
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify done channel is closed after Close.
	select {
	case <-transport.done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("done channel not closed after Close")
	}
}

func TestStdioTransport_ReadLoop_DispatchesNotification(t *testing.T) {
	transport := newTestStdioTransport(t, "notify")

	notified := make(chan string, 1)
	transport.onNotification = func(method string) {
		select {
		case notified <- method:
		default:
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	select {
	case method := <-notified:
		if method != "notifications/tools/list_changed" {
			t.Errorf("notification method = %q, want notifications/tools/list_changed", method)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for notification dispatch")
	}
}

func TestStdioTransport_ReadLoop_CallsOnDisconnect(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	disconnected := make(chan struct{}, 1)
	transport.onDisconnect = func() {
		select {
		case disconnected <- struct{}{}:
		default:
		}
	}

	// Use a long-lived context so ctx.Err() == nil when readLoop checks it
	// after the process dies, ensuring onDisconnect fires.
	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Killing the process triggers readLoop's disconnect path.
	transport.Close()

	select {
	case <-disconnected:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for onDisconnect callback")
	}
}

func TestStdioTransport_ConcurrentCallTool_UniqueRequestIDs(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	const numCalls = 10
	var wg sync.WaitGroup
	results := make([]string, numCalls)
	errs := make([]error, numCalls)

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := transport.CallTool(ctx,
				fmt.Sprintf("tool_%d", idx),
				json.RawMessage(`{}`))
			results[idx] = result
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d error: %v", i, err)
		}
	}
	for i, result := range results {
		if result == "" {
			t.Errorf("call %d returned empty result", i)
		}
	}
}

func TestStdioTransport_CallTool_NilArgs(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	result, err := transport.CallTool(ctx, "echo", nil)
	if err != nil {
		t.Fatalf("CallTool with nil args: %v", err)
	}
	if result != "result from echo" {
		t.Errorf("result = %q, want 'result from echo'", result)
	}
}

func TestStdioTransport_CallTool_InvalidArgs(t *testing.T) {
	transport := newTestStdioTransport(t, "normal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { transport.Close() })

	_, err := transport.CallTool(ctx, "echo", json.RawMessage(`not-valid-json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON args")
	}
	if !strings.Contains(err.Error(), "parsing tool arguments") {
		t.Errorf("error = %q, expected to contain 'parsing tool arguments'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// cappedBuffer unit tests
// ---------------------------------------------------------------------------

func TestCappedBuffer_RetainsLastMaxBytes(t *testing.T) {
	buf := &cappedBuffer{max: 10}

	buf.Write([]byte("hello"))
	if got := buf.String(); got != "hello" {
		t.Errorf("after first write: got %q, want 'hello'", got)
	}

	buf.Write([]byte(" world!!!"))
	got := buf.String()
	if len(got) > 10 {
		t.Errorf("buffer exceeded max: len=%d, content=%q", len(got), got)
	}
	// "hello" + " world!!!" = 14 bytes; last 10 = "o world!!!"
	if got != "o world!!!" {
		t.Errorf("after overflow: got %q, want 'o world!!!'", got)
	}
}

func TestCappedBuffer_SingleLargeWrite(t *testing.T) {
	buf := &cappedBuffer{max: 5}

	n, err := buf.Write([]byte("abcdefghij"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 10 {
		t.Errorf("Write returned %d, want 10", n)
	}

	// Single write >= max keeps last max bytes.
	if got := buf.String(); got != "fghij" {
		t.Errorf("got %q, want 'fghij'", got)
	}
}

func TestCappedBuffer_ConcurrentWrites(t *testing.T) {
	buf := &cappedBuffer{max: 100}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf.Write([]byte("0123456789"))
		}()
	}
	wg.Wait()

	got := buf.String()
	if len(got) > 100 {
		t.Errorf("buffer exceeded max after concurrent writes: len=%d", len(got))
	}
}
