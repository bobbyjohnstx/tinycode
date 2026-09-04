package plugin

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

	pluginsdk "github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// PluginInfo describes a loaded plugin for external consumers.
type PluginInfo struct {
	ID    string                   `json:"id"`
	Tools []pluginsdk.ToolManifest `json:"tools"`
	Hooks []string                 `json:"hooks"`
}

// pluginProcess is a running plugin subprocess.
type pluginProcess struct {
	id       string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Scanner
	manifest pluginsdk.InitializeResult
	hooks    map[string]bool

	mu     sync.Mutex
	nextID atomic.Int64
}

// Manager manages plugin subprocesses, routing tool calls and hooks to them.
type Manager struct {
	plugins map[string]*pluginProcess
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewManager creates an empty plugin manager.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		plugins: make(map[string]*pluginProcess),
		logger:  logger,
	}
}

// LoadPlugin starts a plugin binary, performs the initialize handshake, and
// registers the plugin for tool calls and hook dispatch.
func (m *Manager) LoadPlugin(ctx context.Context, binaryPath string, options map[string]any) error {
	cmd := exec.CommandContext(ctx, binaryPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting plugin %s: %w", binaryPath, err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Send initialize request.
	initParams := pluginsdk.InitializeParams{
		Version: "0.1.0",
		Options: options,
	}
	paramsJSON, err := json.Marshal(initParams)
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("marshaling init params: %w", err)
	}

	req := pluginsdk.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  paramsJSON,
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("marshaling init request: %w", err)
	}
	reqData = append(reqData, '\n')

	if _, err := stdin.Write(reqData); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("writing init request: %w", err)
	}

	// Read initialize response.
	if !scanner.Scan() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		scanErr := scanner.Err()
		if scanErr != nil {
			return fmt.Errorf("reading init response: %w", scanErr)
		}
		return fmt.Errorf("plugin closed before responding to initialize")
	}

	var resp pluginsdk.JSONRPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("parsing init response: %w", err)
	}
	if resp.Error != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("plugin init error: %s", resp.Error.Message)
	}

	var manifest pluginsdk.InitializeResult
	if err := json.Unmarshal(resp.Result, &manifest); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("parsing init result: %w", err)
	}

	hooks := make(map[string]bool, len(manifest.Hooks))
	for _, h := range manifest.Hooks {
		hooks[h] = true
	}

	pp := &pluginProcess{
		id:       manifest.ID,
		cmd:      cmd,
		stdin:    stdin,
		stdout:   scanner,
		manifest: manifest,
		hooks:    hooks,
	}
	pp.nextID.Store(1) // ID 1 was used for initialize

	m.mu.Lock()
	m.plugins[manifest.ID] = pp
	m.mu.Unlock()

	m.logger.Info("plugin loaded", "id", manifest.ID, "tools", len(manifest.Tools), "hooks", len(manifest.Hooks))
	return nil
}

// UnloadPlugin sends a shutdown notification and kills the plugin process.
func (m *Manager) UnloadPlugin(id string) error {
	m.mu.Lock()
	pp, ok := m.plugins[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown plugin: %s", id)
	}
	delete(m.plugins, id)
	m.mu.Unlock()

	killPlugin(pp)
	m.logger.Info("plugin unloaded", "id", id)
	return nil
}

// CallTool invokes a tool on the specified plugin and returns the result.
func (m *Manager) CallTool(ctx context.Context, pluginID, toolName string, args json.RawMessage, tc pluginsdk.ToolContext) (pluginsdk.ToolCallResult, error) {
	m.mu.RLock()
	pp, ok := m.plugins[pluginID]
	m.mu.RUnlock()
	if !ok {
		return pluginsdk.ToolCallResult{}, fmt.Errorf("unknown plugin: %s", pluginID)
	}

	params := pluginsdk.ToolCallParams{
		Name:    toolName,
		Args:    args,
		Context: tc,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return pluginsdk.ToolCallResult{}, fmt.Errorf("marshaling tool call params: %w", err)
	}

	respResult, err := pp.roundTrip(pluginsdk.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      pp.nextID.Add(1),
		Method:  "tool/call",
		Params:  paramsJSON,
	})
	if err != nil {
		return pluginsdk.ToolCallResult{}, fmt.Errorf("tool call %s/%s: %w", pluginID, toolName, err)
	}

	var result pluginsdk.ToolCallResult
	if err := json.Unmarshal(respResult, &result); err != nil {
		return pluginsdk.ToolCallResult{}, fmt.Errorf("parsing tool result: %w", err)
	}
	return result, nil
}

// DispatchHook fans out a hook invocation to all plugins that registered for it
// and collects their outputs.
func (m *Manager) DispatchHook(hookName string, input any) ([]any, error) {
	m.mu.RLock()
	var targets []*pluginProcess
	for _, pp := range m.plugins {
		if pp.hooks[hookName] {
			targets = append(targets, pp)
		}
	}
	m.mu.RUnlock()

	if len(targets) == 0 {
		return nil, nil
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshaling hook input: %w", err)
	}

	var results []any
	for _, pp := range targets {
		params := pluginsdk.HookParams{
			Name:  hookName,
			Input: inputJSON,
		}
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			m.logger.Warn("marshaling hook params", "plugin", pp.id, "hook", hookName, "error", err)
			continue
		}

		respResult, err := pp.roundTrip(pluginsdk.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      pp.nextID.Add(1),
			Method:  "hook/invoke",
			Params:  paramsJSON,
		})
		if err != nil {
			m.logger.Warn("hook dispatch failed", "plugin", pp.id, "hook", hookName, "error", err)
			continue
		}

		var hookResult pluginsdk.HookResult
		if err := json.Unmarshal(respResult, &hookResult); err != nil {
			m.logger.Warn("parsing hook result", "plugin", pp.id, "hook", hookName, "error", err)
			continue
		}

		if hookResult.Output != nil {
			var out any
			if err := json.Unmarshal(hookResult.Output, &out); err == nil {
				results = append(results, out)
			}
		}
	}

	return results, nil
}

// Plugins returns info about all loaded plugins.
func (m *Manager) Plugins() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(m.plugins))
	for _, pp := range m.plugins {
		infos = append(infos, PluginInfo{
			ID:    pp.manifest.ID,
			Tools: pp.manifest.Tools,
			Hooks: pp.manifest.Hooks,
		})
	}
	return infos
}

// Shutdown unloads all plugins.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	plugins := make([]*pluginProcess, 0, len(m.plugins))
	for _, pp := range m.plugins {
		plugins = append(plugins, pp)
	}
	m.plugins = make(map[string]*pluginProcess)
	m.mu.Unlock()

	for _, pp := range plugins {
		killPlugin(pp)
	}
	m.logger.Info("plugin manager shut down", "count", len(plugins))
}

// roundTrip sends a JSON-RPC request and reads the response. It serializes
// access to stdin/stdout for the plugin process.
func (pp *pluginProcess) roundTrip(req pluginsdk.JSONRPCRequest) (json.RawMessage, error) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	data = append(data, '\n')

	if _, err := pp.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	if !pp.stdout.Scan() {
		if err := pp.stdout.Err(); err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}
		return nil, fmt.Errorf("plugin closed connection")
	}

	var resp pluginsdk.JSONRPCResponse
	if err := json.Unmarshal(pp.stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("plugin error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// killPlugin closes stdin and kills the process.
func killPlugin(pp *pluginProcess) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if pp.stdin != nil {
		_ = pp.stdin.Close()
	}
	if pp.cmd != nil && pp.cmd.Process != nil {
		_ = pp.cmd.Process.Kill()
		_ = pp.cmd.Wait()
	}
}
