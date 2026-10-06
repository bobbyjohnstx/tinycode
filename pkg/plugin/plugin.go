package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

// Plugin defines a tinycode plugin with its tools and hooks.
type Plugin struct {
	ID    string
	Tools []ToolDef
	Hooks HookHandlers
}

// Run starts the plugin's stdin/stdout JSON-RPC loop. It blocks until stdin is
// closed or the context is cancelled. This is the entry point for external
// plugin binaries.
func Run(p Plugin) {
	RunWithOptions(func(_ InitializeParams) (Plugin, error) {
		return p, nil
	})
}

// RunWithOptions starts the plugin using a factory that receives initialize params.
func RunWithOptions(factory func(InitializeParams) (Plugin, error)) {
	if err := runWithFactory(context.Background(), factory, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "plugin: %v\n", err)
		os.Exit(1)
	}
}

// run is the testable core of Run.
func run(ctx context.Context, p Plugin, stdin io.Reader, stdout io.Writer) error {
	return runWithFactory(ctx, func(_ InitializeParams) (Plugin, error) {
		return p, nil
	}, stdin, stdout)
}

// runWithFactory is the core JSON-RPC loop that uses a factory to create the plugin
// after receiving initialize params.
func runWithFactory(ctx context.Context, factory func(InitializeParams) (Plugin, error), stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	enc := json.NewEncoder(stdout)

	// First message must be initialize.
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading initialize: %w", err)
		}
		return fmt.Errorf("stdin closed before initialize")
	}

	var initReq JSONRPCRequest
	if err := json.Unmarshal(scanner.Bytes(), &initReq); err != nil {
		return fmt.Errorf("parsing initialize request: %w", err)
	}
	if initReq.Method != "initialize" {
		return fmt.Errorf("expected initialize, got %q", initReq.Method)
	}

	var params InitializeParams
	if initReq.Params != nil {
		_ = json.Unmarshal(initReq.Params, &params)
	}

	p, err := factory(params)
	if err != nil {
		return fmt.Errorf("creating plugin: %w", err)
	}

	result := buildManifest(p)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := enc.Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      initReq.ID,
		Result:  resultJSON,
	}); err != nil {
		return fmt.Errorf("writing initialize response: %w", err)
	}

	// Main dispatch loop.
	toolMap := make(map[string]*ToolDef, len(p.Tools))
	for i := range p.Tools {
		toolMap[p.Tools[i].Name] = &p.Tools[i]
	}

	// disposed ensures Dispose runs at most once (hook/invoke or EOF, not both).
	var disposed atomic.Bool

	for scanner.Scan() {
		var req JSONRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		// Notifications (no ID) don't get responses.
		if req.ID == 0 {
			continue
		}

		resp := dispatch(ctx, req, toolMap, &p.Hooks, &disposed)
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("writing response: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	// Call Dispose on clean EOF only if hook/invoke dispose did not already run.
	if p.Hooks.Dispose != nil && disposed.CompareAndSwap(false, true) {
		_ = p.Hooks.Dispose(ctx)
	}

	return nil
}

// buildManifest converts a Plugin into an InitializeResult for the wire.
func buildManifest(p Plugin) InitializeResult {
	manifest := InitializeResult{
		ID:    p.ID,
		Tools: make([]ToolManifest, len(p.Tools)),
		Hooks: registeredHooks(&p.Hooks),
	}
	for i, t := range p.Tools {
		manifest.Tools[i] = ToolManifest{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		}
	}
	return manifest
}

// registeredHooks returns the list of hook names that have handlers set.
func registeredHooks(h *HookHandlers) []string {
	var hooks []string
	if h.SessionStart != nil {
		hooks = append(hooks, "session.start")
	}
	if h.SessionEnd != nil {
		hooks = append(hooks, "session.end")
	}
	if h.PermissionAsk != nil {
		hooks = append(hooks, "permission.ask")
	}
	if h.ShellEnv != nil {
		hooks = append(hooks, "shell.env")
	}
	if h.ToolExecBefore != nil {
		hooks = append(hooks, "tool.execute.before")
	}
	if h.ToolExecAfter != nil {
		hooks = append(hooks, "tool.execute.after")
	}
	if h.Dispose != nil {
		hooks = append(hooks, "dispose")
	}
	return hooks
}

// dispatch routes a JSON-RPC request to the appropriate tool or hook handler.
// disposed tracks whether Dispose already ran so EOF does not run it twice.
func dispatch(ctx context.Context, req JSONRPCRequest, tools map[string]*ToolDef, hooks *HookHandlers, disposed *atomic.Bool) JSONRPCResponse {
	switch req.Method {
	case "tool/call":
		return dispatchToolCall(ctx, req, tools)
	case "hook/invoke":
		return dispatchHook(ctx, req, hooks, disposed)
	default:
		errJSON, _ := json.Marshal(JSONRPCError{
			Code:    -32601,
			Message: fmt.Sprintf("unknown method: %s", req.Method),
		})
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   jsonPtr(errJSON),
		}
	}
}

func dispatchToolCall(ctx context.Context, req JSONRPCRequest, tools map[string]*ToolDef) JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, fmt.Sprintf("invalid tool call params: %v", err))
	}

	def, ok := tools[params.Name]
	if !ok {
		return errorResponse(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}

	output, err := def.Execute(ctx, params.Args, params.Context)
	result := ToolCallResult{Content: output}
	if err != nil {
		result.Content = err.Error()
		result.IsError = true
	}

	resultJSON, _ := json.Marshal(result)
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}
}

func dispatchHook(ctx context.Context, req JSONRPCRequest, hooks *HookHandlers, disposed *atomic.Bool) JSONRPCResponse {
	var params HookParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook params: %v", err))
	}

	var hookResult HookResult
	var hookErr error

	switch params.Name {
	case "session.start":
		if hooks.SessionStart != nil {
			var event SessionStartEvent
			if err := json.Unmarshal(params.Input, &event); err != nil {
				return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook input: %v", err))
			}
			var out *SessionStartOutput
			out, hookErr = hooks.SessionStart(ctx, event)
			if out != nil && hookErr == nil {
				hookResult.Output, _ = json.Marshal(out)
			}
		}
	case "session.end":
		if hooks.SessionEnd != nil {
			var event SessionEndEvent
			if err := json.Unmarshal(params.Input, &event); err != nil {
				return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook input: %v", err))
			}
			hookErr = hooks.SessionEnd(ctx, event)
		}
	case "permission.ask":
		if hooks.PermissionAsk != nil {
			var input PermissionInput
			if err := json.Unmarshal(params.Input, &input); err != nil {
				return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook input: %v", err))
			}
			var out *PermissionOutput
			out, hookErr = hooks.PermissionAsk(ctx, input)
			if out != nil && hookErr == nil {
				hookResult.Output, _ = json.Marshal(out)
			}
		}
	case "shell.env":
		if hooks.ShellEnv != nil {
			var input ShellEnvInput
			if err := json.Unmarshal(params.Input, &input); err != nil {
				return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook input: %v", err))
			}
			var out *ShellEnvOutput
			out, hookErr = hooks.ShellEnv(ctx, input)
			if out != nil && hookErr == nil {
				hookResult.Output, _ = json.Marshal(out)
			}
		}
	case "tool.execute.before":
		if hooks.ToolExecBefore != nil {
			var input ToolExecBeforeInput
			if err := json.Unmarshal(params.Input, &input); err != nil {
				return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook input: %v", err))
			}
			var out *ToolExecBeforeOutput
			out, hookErr = hooks.ToolExecBefore(ctx, input)
			if out != nil && hookErr == nil {
				hookResult.Output, _ = json.Marshal(out)
			}
		}
	case "tool.execute.after":
		if hooks.ToolExecAfter != nil {
			var input ToolExecAfterInput
			if err := json.Unmarshal(params.Input, &input); err != nil {
				return errorResponse(req.ID, -32602, fmt.Sprintf("invalid hook input: %v", err))
			}
			var out *ToolExecAfterOutput
			out, hookErr = hooks.ToolExecAfter(ctx, input)
			if out != nil && hookErr == nil {
				hookResult.Output, _ = json.Marshal(out)
			}
		}
	case "dispose":
		if hooks.Dispose != nil && (disposed == nil || disposed.CompareAndSwap(false, true)) {
			hookErr = hooks.Dispose(ctx)
		}
	}

	if hookErr != nil {
		return errorResponse(req.ID, -32000, hookErr.Error())
	}

	resultJSON, _ := json.Marshal(hookResult)
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}
}

func errorResponse(id int64, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
}

// jsonPtr unmarshals raw JSON into a *JSONRPCError. Used in the unknown-method
// path where we already have the error serialized.
func jsonPtr(data []byte) *JSONRPCError {
	var e JSONRPCError
	_ = json.Unmarshal(data, &e)
	return &e
}
