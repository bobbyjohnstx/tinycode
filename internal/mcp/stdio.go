package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/procenv"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	defaultStdioTimeout = 60 * time.Second
	maxLineSize         = 10 * 1024 * 1024 // 10MB per JSON-RPC line
	maxStderrSize       = 64 * 1024        // 64KB stderr ring buffer
)

type StdioTransport struct {
	command string
	args    []string
	env     map[string]string
	Timeout time.Duration

	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stderrBuf *cappedBuffer
	nextID    atomic.Int64

	messages chan *jsonrpcResponse
	done     chan struct{}
	doneOnce sync.Once

	onDisconnect   func()
	onNotification func(method string)
}

func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport {
	return &StdioTransport{
		command: command,
		args:    args,
		env:     env,
	}
}

func (t *StdioTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	cmd := exec.CommandContext(ctx, t.command, t.args...)

	cmd.Env = procenv.Child(t.env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	t.stderrBuf = &cappedBuffer{max: maxStderrSize}
	cmd.Stderr = t.stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting MCP server %s: %w", t.command, err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.messages = make(chan *jsonrpcResponse, 64)
	t.done = make(chan struct{})
	t.doneOnce = sync.Once{}

	safego.Go(func() { t.readLoop(ctx, stdout) })

	initReq := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      t.nextID.Add(1),
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "tinycode",
				"version": "0.1.0",
			},
		},
	}

	resp, err := t.roundTrip(ctx, initReq)
	if err != nil {
		_ = t.closeInternal()
		return fmt.Errorf("MCP initialize: %w", err)
	}
	if resp.Error != nil {
		_ = t.closeInternal()
		return fmt.Errorf("MCP initialize error: %s", resp.Error.Message)
	}

	notif := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := t.sendMessage(notif); err != nil {
		_ = t.closeInternal()
		return fmt.Errorf("sending initialized notification: %w", err)
	}

	return nil
}

func (t *StdioTransport) readLoop(ctx context.Context, r io.Reader) {
	defer t.doneOnce.Do(func() { close(t.done) })

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()

		var resp jsonrpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		// Handle notifications (no ID, has method, no result/error)
		if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
			if resp.Method != "" && t.onNotification != nil {
				safego.Go(func() { t.onNotification(resp.Method) })
			}
			continue
		}

		select {
		case t.messages <- &resp:
		case <-ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("MCP stdio read error", "command", t.command, "error", err)
	}

	if t.stderrBuf != nil {
		if stderr := t.stderrBuf.String(); stderr != "" {
			slog.Debug("MCP server stderr", "command", t.command, "stderr", stderr)
		}
	}

	if ctx.Err() == nil && t.onDisconnect != nil {
		t.onDisconnect()
	}
}

func (t *StdioTransport) ListTools(ctx context.Context) ([]MCPTool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var allTools []MCPTool
	var cursor string

	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		req := jsonrpcRequest{
			JSONRPC: "2.0",
			ID:      t.nextID.Add(1),
			Method:  "tools/list",
			Params:  params,
		}

		resp, err := t.roundTrip(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("listing tools: %w", err)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("listing tools: %s", resp.Error.Message)
		}

		var result struct {
			Tools      []MCPTool `json:"tools"`
			NextCursor string    `json:"nextCursor,omitempty"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, fmt.Errorf("parsing tools response: %w", err)
		}

		allTools = append(allTools, result.Tools...)

		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	return allTools, nil
}

func (t *StdioTransport) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", fmt.Errorf("parsing tool arguments: %w", err)
		}
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      t.nextID.Add(1),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}

	resp, err := t.roundTrip(ctx, req)
	if err != nil {
		return "", fmt.Errorf("calling tool %s: %w", name, err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("tool %s error: %s", name, resp.Error.Message)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parsing tool result: %w", err)
	}

	var output string
	for _, c := range result.Content {
		if c.Type == "text" {
			if output != "" {
				output += "\n"
			}
			output += c.Text
		}
	}

	if result.IsError {
		return "", fmt.Errorf("tool error: %s", output)
	}

	return output, nil
}

func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeInternal()
}

func (t *StdioTransport) closeInternal() error {
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}
	t.doneOnce.Do(func() { close(t.done) })
	return nil
}

func (t *StdioTransport) sendMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}

func (t *StdioTransport) roundTrip(ctx context.Context, req jsonrpcRequest) (*jsonrpcResponse, error) {
	if err := t.sendMessage(req); err != nil {
		return nil, err
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = defaultStdioTimeout
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case resp, ok := <-t.messages:
			if !ok {
				return nil, fmt.Errorf("transport closed")
			}
			if resp.ID == req.ID {
				return resp, nil
			}
			slog.Debug("discarding non-matching RPC response", "expected", req.ID, "got", resp.ID)
		case <-timer.C:
			return nil, fmt.Errorf("roundTrip timeout after %v", timeout)
		case <-t.done:
			return nil, fmt.Errorf("transport disconnected")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cappedBuffer is a thread-safe writer that retains the last max bytes.
type cappedBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) >= b.max {
		b.buf = append(b.buf[:0], p[len(p)-b.max:]...)
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
