package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	pkgplugin "github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

var ErrPluginNotFound = errors.New("plugin not found")

const (
	hookTimeout     = 5 * time.Second
	toolCallTimeout = 30 * time.Second
	killTimeout     = 3 * time.Second
	maxOutputSize   = 10 * 1024 * 1024 // 10MB — cap plugin stdout to prevent OOM
)

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ToolManifest is an alias for the canonical type in pkg/plugin.
type ToolManifest = pkgplugin.ToolManifest

// pluginProcess tracks a running plugin subprocess.
type pluginProcess struct {
	info    *PluginInfo
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	encoder *json.Encoder
	decoder *json.Decoder
	counter *countingReader // per-response byte counter for stdout
	mu      sync.Mutex     // protects writes/reads to stdin/stdout
	hooks   []string       // hooks declared during initialize
	tools   []ToolManifest
	nextID  atomic.Int64
	dead    atomic.Bool
	done    chan struct{} // closed when the process exits
}

// countingReader wraps a reader and counts bytes read since the last reset.
// Used to enforce per-response size limits on plugin stdout.
type countingReader struct {
	r     io.Reader
	n     int64
	limit int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n >= c.limit {
		return 0, fmt.Errorf("response exceeded %d byte limit", c.limit)
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.limit {
		return n, fmt.Errorf("response exceeded %d byte limit", c.limit)
	}
	return n, err
}

func (c *countingReader) reset() {
	c.n = 0
}

// CommandFactory creates exec.Cmd instances. Tests can override this.
type CommandFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

// ResolveFunc resolves a plugin name to a binary path.
type ResolveFunc func(name string) (string, error)

// defaultCommandFactory uses exec.CommandContext.
func defaultCommandFactory(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// Manager manages loaded plugins and the curated plugin registry.
type Manager struct {
	mu             sync.RWMutex
	plugins        map[string]*pluginProcess
	registry       []RegistryEntry
	logger         *slog.Logger
	directory      string
	commandFactory CommandFactory
	resolveFunc    ResolveFunc
}

// NewManager creates an empty plugin manager using the built-in registry.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		plugins:        make(map[string]*pluginProcess),
		registry:       Registry(),
		logger:         logger,
		commandFactory: defaultCommandFactory,
		resolveFunc:    ResolveBinary,
	}
}

// NewManagerWithRegistry creates a plugin manager with a custom registry.
// This is intended for tests that need to control the registry entries.
func NewManagerWithRegistry(entries []RegistryEntry) *Manager {
	if entries == nil {
		entries = []RegistryEntry{}
	}
	return &Manager{
		plugins:        make(map[string]*pluginProcess),
		registry:       entries,
		logger:         slog.Default(),
		commandFactory: defaultCommandFactory,
		resolveFunc:    ResolveBinary,
	}
}

// SetDirectory sets the working directory used in the initialize handshake.
func (m *Manager) SetDirectory(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.directory = dir
}

// SetCommandFactory overrides the command factory (for testing).
func (m *Manager) SetCommandFactory(f CommandFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandFactory = f
}

// SetResolveFunc overrides the binary resolution function (for testing).
func (m *Manager) SetResolveFunc(f ResolveFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveFunc = f
}

// List returns all loaded plugins.
func (m *Manager) List() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]PluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, *p.info)
	}
	return out
}

// Load resolves a plugin binary, starts it as a subprocess, performs the
// JSON-RPC initialize handshake, and registers the plugin.
func (m *Manager) Load(name string, options map[string]any) (*PluginInfo, error) {
	if name == "" {
		return nil, errors.New("plugin name is required")
	}

	m.mu.RLock()
	found := false
	for _, entry := range m.registry {
		if entry.Name == name {
			found = true
			break
		}
	}
	dir := m.directory
	factory := m.commandFactory
	resolve := m.resolveFunc
	m.mu.RUnlock()

	if !found {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}

	binPath, err := resolve(name)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin %s: %w", name, err)
	}

	pid, err := id.Ascending("plugin")
	if err != nil {
		return nil, err
	}

	// Use Background context for the process lifetime — not a timeout context,
	// because the process should live until explicitly stopped.
	cmd := factory(context.Background(), binPath)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin %s: %w", name, err)
	}

	info := &PluginInfo{ID: pid, Name: name}
	doneCh := make(chan struct{})
	cr := &countingReader{r: stdoutPipe, limit: maxOutputSize}
	proc := &pluginProcess{
		info:    info,
		cmd:     cmd,
		stdin:   stdinPipe,
		stdout:  stdoutPipe,
		encoder: json.NewEncoder(stdinPipe),
		decoder: json.NewDecoder(cr),
		counter: cr,
		done:    doneCh,
	}

	// Start health monitor before handshake so we capture early exits.
	safego.Go(func() {
		_ = cmd.Wait()
		proc.dead.Store(true)
		close(doneCh)
		m.logger.Warn("plugin process exited", "id", pid, "name", name)
	})

	// Initialize handshake.
	result, err := proc.sendRPC("initialize", pkgplugin.InitializeParams{
		Version:   "1.0",
		Directory: dir,
		Options:   options,
	}, hookTimeout)
	if err != nil {
		// Kill the process on handshake failure.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-doneCh
		return nil, fmt.Errorf("initialize plugin %s: %w", name, err)
	}

	var initResult pkgplugin.InitializeResult
	if result != nil {
		_ = json.Unmarshal(result, &initResult)
	}
	proc.hooks = initResult.Hooks
	proc.tools = initResult.Tools

	m.mu.Lock()
	m.plugins[pid] = proc
	m.mu.Unlock()

	m.logger.Info("plugin loaded", "id", pid, "name", name, "hooks", proc.hooks)
	return info, nil
}

// Unload removes a plugin by ID and stops its subprocess.
func (m *Manager) Unload(pluginID string) error {
	m.mu.Lock()
	proc, ok := m.plugins[pluginID]
	if !ok {
		m.mu.Unlock()
		return errors.New("plugin not found")
	}
	delete(m.plugins, pluginID)
	m.mu.Unlock()

	m.stopProcess(proc)
	m.logger.Info("plugin unloaded", "id", pluginID)
	return nil
}

// Registry returns available registry entries.
func (m *Manager) Registry() []RegistryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.registry == nil {
		return []RegistryEntry{}
	}
	out := make([]RegistryEntry, len(m.registry))
	copy(out, m.registry)
	return out
}

// Plugins returns info about all loaded plugins.
// Deprecated: Use List instead.
func (m *Manager) Plugins() []PluginInfo {
	return m.List()
}

// UnloadPlugin removes a plugin by ID.
// Deprecated: Use Unload instead.
func (m *Manager) UnloadPlugin(id string) error {
	return m.Unload(id)
}

// Shutdown sends dispose to all plugins and stops their subprocesses.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	procs := make([]*pluginProcess, 0, len(m.plugins))
	for _, p := range m.plugins {
		procs = append(procs, p)
	}
	m.plugins = make(map[string]*pluginProcess)
	m.mu.Unlock()

	for _, proc := range procs {
		m.stopProcess(proc)
	}

	m.logger.Info("plugin manager shut down", "count", len(procs))
}

// broadcastHook sends a JSON-RPC hook to a specific plugin. Returns the result
// field from the response. Goroutine-safe via per-process mutex.
func (m *Manager) broadcastHook(pluginID, method string, params any) (json.RawMessage, error) {
	m.mu.RLock()
	proc, ok := m.plugins[pluginID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plugin %s not found", pluginID)
	}

	if proc.dead.Load() {
		return nil, fmt.Errorf("plugin %s process is dead", pluginID)
	}

	return proc.sendRPC(method, params, hookTimeout)
}

// pluginsWithHook returns processes that declared a given hook.
func (m *Manager) pluginsWithHook(hookName string) []*pluginProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*pluginProcess
	for _, proc := range m.plugins {
		for _, h := range proc.hooks {
			if h == hookName {
				result = append(result, proc)
				break
			}
		}
	}
	return result
}

// sendRPC sends a JSON-RPC request and reads the response with a timeout.
func (p *pluginProcess) sendRPC(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.dead.Load() {
		return nil, fmt.Errorf("process is dead")
	}

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	req := pkgplugin.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      p.nextID.Add(1),
		Method:  method,
		Params:  paramsRaw,
	}

	if err := p.encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	// Reset per-response byte counter before each decode (#382).
	if p.counter != nil {
		p.counter.reset()
	}

	// Use a channel + goroutine for timeout on the blocking decode.
	type rpcResult struct {
		resp pkgplugin.JSONRPCResponse
		err  error
	}
	ch := make(chan rpcResult, 1)
	safego.Go(func() {
		var resp pkgplugin.JSONRPCResponse
		err := p.decoder.Decode(&resp)
		ch <- rpcResult{resp: resp, err: err}
	})

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("decode response: %w", r.err)
		}
		if r.resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", r.resp.Error.Code, r.resp.Error.Message)
		}
		return r.resp.Result, nil
	case <-time.After(timeout):
		// Close stdout to unblock the decode goroutine, then kill the
		// process to prevent a goroutine leak (#386).
		_ = p.stdout.Close()
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		p.dead.Store(true)
		return nil, fmt.Errorf("timeout waiting for response to %s", method)
	case <-p.done:
		return nil, fmt.Errorf("process exited while waiting for response to %s", method)
	}
}

// Tools returns the tools declared by all loaded plugins.
func (m *Manager) Tools() []ToolManifest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []ToolManifest
	for _, proc := range m.plugins {
		out = append(out, proc.tools...)
	}
	return out
}

// CallTool invokes a tool on the plugin that owns it. Returns the tool output
// as raw JSON, or an error if the plugin or tool is not found.
func (m *Manager) CallTool(pluginID, toolName string, args json.RawMessage, sessionID string) (json.RawMessage, error) {
	m.mu.RLock()
	proc, ok := m.plugins[pluginID]
	dir := m.directory
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plugin %s not found", pluginID)
	}

	raw, err := proc.sendRPC("tool/call", pkgplugin.ToolCallParams{
		Name: toolName,
		Args: args,
		Context: pkgplugin.ToolContext{
			SessionID: sessionID,
			Directory: dir,
		},
	}, toolCallTimeout)
	if err != nil {
		return nil, err
	}

	var result pkgplugin.ToolCallResult
	if raw != nil {
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("unmarshal tool result: %w", err)
		}
	}

	if result.IsError {
		return nil, fmt.Errorf("tool %s error: %s", toolName, result.Content)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal tool result: %w", err)
	}
	return resultJSON, nil
}

// stopProcess sends dispose (best-effort) and then kills the process.
func (m *Manager) stopProcess(proc *pluginProcess) {
	if proc.dead.Load() {
		return
	}

	// Best-effort dispose via hook/invoke so the SDK's dispatchHook handles it.
	_, err := proc.sendHook("dispose", nil)
	if err != nil {
		m.logger.Debug("dispose hook failed", "plugin", proc.info.Name, "error", err)
	}

	// Close stdin to signal EOF to the child.
	_ = proc.stdin.Close()

	// Wait for graceful exit using the health monitor's done channel.
	select {
	case <-proc.done:
		// Exited gracefully.
	case <-time.After(killTimeout):
		// Force kill.
		if proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
		<-proc.done
	}
}
