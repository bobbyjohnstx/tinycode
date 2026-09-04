package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type StdioTransport struct {
	service *Service
	writer  io.Writer
	mu      sync.Mutex
}

func NewStdioTransport(service *Service, writer io.Writer) *StdioTransport {
	return &StdioTransport{
		service: service,
		writer:  writer,
	}
}

func (t *StdioTransport) HandleStdio(ctx context.Context, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			t.sendError(nil, ParseError, "invalid JSON")
			continue
		}

		if req.JSONRPC != "2.0" {
			t.sendError(req.ID, InvalidRequest, "unsupported JSON-RPC version")
			continue
		}

		if req.Method == "" {
			t.sendError(req.ID, InvalidRequest, "missing method")
			continue
		}

		result, rpcErr := t.service.HandleRequest(ctx, req.Method, req.Params)

		if req.ID == nil {
			continue
		}

		if rpcErr != nil {
			t.sendError(req.ID, rpcErr.Code, rpcErr.Message)
		} else {
			t.sendResult(req.ID, result)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	return nil
}

func (t *StdioTransport) SendNotification(method string, params any) {
	notif := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	t.send(notif)
}

func (t *StdioTransport) sendResult(id any, result any) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	t.send(resp)
}

func (t *StdioTransport) sendError(id any, code int, message string) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	t.send(resp)
}

func (t *StdioTransport) send(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal JSON-RPC message", "error", err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	data = append(data, '\n')
	if _, err := t.writer.Write(data); err != nil {
		slog.Error("failed to write JSON-RPC message", "error", err)
	}
}
