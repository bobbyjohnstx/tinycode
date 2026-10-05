package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/bobbyjohnstx/tinycode/internal/safego"
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
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type pendingRequest struct {
	ch chan rpcResponse
}

type StdioTransport struct {
	service *Service
	writer  io.Writer
	mu      sync.Mutex
	nextID  atomic.Int64
	pending sync.Map // id (int64) -> *pendingRequest
	closed  atomic.Bool
}

func NewStdioTransport(service *Service, writer io.Writer) *StdioTransport {
	t := &StdioTransport{
		service: service,
		writer:  writer,
	}
	service.SetTransport(t)
	return t
}

func (t *StdioTransport) HandleStdio(ctx context.Context, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
		t.closed.Store(true)
		t.rejectPending(fmt.Errorf("transport closed"))
	}()

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

		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.sendError(nil, ParseError, "invalid JSON")
			continue
		}

		var version string
		_ = json.Unmarshal(envelope["jsonrpc"], &version)
		if version != "2.0" {
			var id any
			_ = json.Unmarshal(envelope["id"], &id)
			t.sendError(id, InvalidRequest, "unsupported JSON-RPC version")
			continue
		}

		// Response to an outbound request (has id, no method).
		if _, hasMethod := envelope["method"]; !hasMethod {
			if rawID, ok := envelope["id"]; ok {
				t.deliverResponse(line, rawID)
			}
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			t.sendError(nil, ParseError, "invalid JSON")
			continue
		}

		if req.Method == "" {
			t.sendError(req.ID, InvalidRequest, "missing method")
			continue
		}

		// Notifications (no id) and requests are handled concurrently so
		// permission replies can arrive while a prompt is blocked.
		reqCopy := req
		lineCopy := append([]byte(nil), line...)
		_ = lineCopy
		wg.Add(1)
		safego.Go(func() {
			defer wg.Done()
			result, rpcErr := t.service.HandleRequest(ctx, reqCopy.Method, reqCopy.Params)
			if reqCopy.ID == nil {
				return
			}
			if rpcErr != nil {
				t.sendError(reqCopy.ID, rpcErr.Code, rpcErr.Message)
			} else {
				t.sendResult(reqCopy.ID, result)
			}
		})
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	return nil
}

func (t *StdioTransport) deliverResponse(line []byte, rawID json.RawMessage) {
	var id any
	if err := json.Unmarshal(rawID, &id); err != nil {
		return
	}
	key := pendingKey(id)
	val, ok := t.pending.Load(key)
	if !ok {
		slog.Debug("ACP response for unknown request", "id", id)
		return
	}
	t.pending.Delete(key)

	var resp rpcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		resp = rpcResponse{Error: &RPCError{Code: ParseError, Message: "invalid response JSON"}}
	}
	pending := val.(*pendingRequest)
	select {
	case pending.ch <- resp:
	default:
	}
}

// SendRequest sends a JSON-RPC request and waits for the correlated response.
func (t *StdioTransport) SendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("transport closed")
	}

	id := t.nextID.Add(1)
	pending := &pendingRequest{ch: make(chan rpcResponse, 1)}
	t.pending.Store(id, pending)

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	t.send(msg)

	select {
	case resp := <-pending.ch:
		t.pending.Delete(id)
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		t.pending.Delete(id)
		return nil, ctx.Err()
	}
}

func (t *StdioTransport) rejectPending(err error) {
	t.pending.Range(func(key, value any) bool {
		t.pending.Delete(key)
		pending := value.(*pendingRequest)
		select {
		case pending.ch <- rpcResponse{Error: &RPCError{Code: InternalError, Message: err.Error()}}:
		default:
		}
		return true
	})
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
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
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

	if t.closed.Load() {
		return
	}

	data = append(data, '\n')
	if _, err := t.writer.Write(data); err != nil {
		slog.Error("failed to write JSON-RPC message", "error", err)
	}
}

func pendingKey(id any) any {
	switch v := id.(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return id
	}
}
