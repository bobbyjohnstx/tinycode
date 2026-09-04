package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

type StdioTransport struct {
	command string
	args    []string
	env     map[string]string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID atomic.Int64
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

	cmd.Env = os.Environ()
	for k, v := range t.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting MCP server %s: %w", t.command, err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.reader = bufio.NewReaderSize(stdout, 1024*1024)

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

	resp, err := t.roundTrip(initReq)
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

		resp, err := t.roundTrip(req)
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

	resp, err := t.roundTrip(req)
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

func (t *StdioTransport) readMessage() (*jsonrpcResponse, error) {
	for {
		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		var resp jsonrpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
			continue
		}

		return &resp, nil
	}
}

func (t *StdioTransport) roundTrip(req jsonrpcRequest) (*jsonrpcResponse, error) {
	if err := t.sendMessage(req); err != nil {
		return nil, err
	}

	for {
		resp, err := t.readMessage()
		if err != nil {
			return nil, err
		}
		if resp.ID == req.ID {
			return resp, nil
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
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
