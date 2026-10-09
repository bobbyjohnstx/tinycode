package plugin

import (
	"bufio"
	"bytes"
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
	"github.com/bobbyjohnstx/tinycode/internal/procenv"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
	pkgplugin "github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

var (
	ErrPluginNotFound = errors.New("plugin not found")
	ErrAlreadyLoaded  = errors.New("already loaded")
	ErrPluginSkipped  = errors.New("plugin skipped")
	hookTimeout       = 5 * time.Second
	toolCallTimeout   = 30 * time.Second
	killTimeout       = 3 * time.Second
	maxOutputSize     = int64(10 * 1024 * 1024) // 10MB — cap plugin stdout to prevent OOM
	maxRPCTimeouts    = 3                       // consecutive timeouts before treating process as wedged
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
	info                *PluginInfo
	cmd                 *exec.Cmd
	stdin               io.WriteCloser
	stdout              io.ReadCloser
	encoder             *json.Encoder
	reader              *bufio.Reader   // line-oriented stdout reader (skips non-JSON pollution)
	counter             *countingReader // per-response byte counter for stdout
	mu                  sync.Mutex      // protects writes/reads to stdin/stdout
	hooks               []string        // hooks declared during initialize
	tools               []ToolManifest
	toolIDs             []string // tool registry IDs registered for this plugin
	nextID              atomic.Int64
	dead                atomic.Bool
	done                chan struct{}       // closed when the process exits
	onRemove            func(reason string) // removes this plugin from the manager map + tools
	consecutiveTimeouts atomic.Int32        // RPC timeouts since last success
	drainDone           chan struct{}       // closed when a timed-out read finishes
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
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = procenv.Child(nil)
	return cmd
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
	toolReg        *tool.Registry // optional; when set, Load/Unload register/unregister tools
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

// SetToolRegistry sets the tool registry used to register plugin tools on Load
// and unregister them on Unload/Shutdown. When nil, tools are not registered.
func (m *Manager) SetToolRegistry(reg *tool.Registry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolReg = reg
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

// knownBuiltinIDs are in-process builtins (see builtin_*.go). Config entries
// for these names are soft-skipped when no external binary is resolvable.
var knownBuiltinIDs = map[string]struct{}{
	"notify":          {},
	"code-review":     {},
	"handoff":         {},
	"context-pruning": {},
}

// Load resolves a plugin binary, starts it as a subprocess, performs the
// JSON-RPC initialize handshake, and registers the plugin.
// The name need not be in the curated registry when resolveFunc finds a binary
// (e.g. ~/.config/tinycode/plugins/<name> or PATH tinycode-plugin-<name>).
func (m *Manager) Load(name string, options map[string]any) (*PluginInfo, error) {
	if name == "" {
		return nil, errors.New("plugin name is required")
	}

	m.mu.RLock()
	for _, p := range m.plugins {
		if p.info.Name == name {
			m.mu.RUnlock()
			return nil, fmt.Errorf("%w: %s", ErrAlreadyLoaded, name)
		}
	}
	inRegistry := false
	for _, entry := range m.registry {
		if entry.Name == name {
			inRegistry = true
			break
		}
	}
	dir := m.directory
	factory := m.commandFactory
	resolve := m.resolveFunc
	logger := m.logger
	m.mu.RUnlock()

	binPath, err := resolve(name)
	if err != nil {
		if !inRegistry {
			if _, ok := knownBuiltinIDs[name]; ok {
				logger.Info("skipping load for builtin plugin listed in config", "name", name)
				return nil, fmt.Errorf("%w: %s", ErrPluginSkipped, name)
			}
			return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, name)
		}
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
		reader:  bufio.NewReader(cr),
		counter: cr,
		done:    doneCh,
	}
	proc.onRemove = func(reason string) {
		m.removeProcess(pid, reason)
	}

	// Start health monitor before handshake so we capture early exits.
	// Do not call Unload here — Wait already completed; Unload would deadlock
	// waiting on the process again. Only map delete + tool unregister.
	safego.Go(func() {
		_ = cmd.Wait()
		proc.dead.Store(true)
		close(doneCh)
		m.logger.Warn("plugin process exited", "id", pid, "name", name)
		m.removeProcess(pid, "process exited")
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

	// Register tools before publishing the plugin so Unload cannot race
	// and leave orphaned registry entries.
	m.registerPluginTools(proc)

	m.mu.Lock()
	m.plugins[pid] = proc
	m.mu.Unlock()

	m.logger.Info("plugin loaded", "id", pid, "name", name, "hooks", proc.hooks)
	return info, nil
}

// Unload removes a plugin by ID and stops its subprocess.
func (m *Manager) Unload(pluginID string) error {
	m.mu.RLock()
	proc, ok := m.plugins[pluginID]
	m.mu.RUnlock()
	if !ok {
		return errors.New("plugin not found")
	}

	m.stopProcess(proc)
	m.removeProcess(pluginID, "unload")
	m.logger.Info("plugin unloaded", "id", pluginID)
	return nil
}

// removeProcess deletes a plugin from the manager map and unregisters its tools.
// It does not Kill/Wait the subprocess — callers that need process teardown
// must stopProcess first (or the process is already dead). Idempotent.
func (m *Manager) removeProcess(pluginID string, reason string) {
	m.mu.Lock()
	proc, ok := m.plugins[pluginID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.plugins, pluginID)
	m.mu.Unlock()

	m.unregisterPluginTools(proc)
	m.logger.Info("plugin removed", "id", pluginID, "reason", reason)
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
		m.unregisterPluginTools(proc)
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

// killForTimeout marks the process dead and removes it from the manager.
// Caller must hold p.mu.
func (p *pluginProcess) killForTimeout() {
	_ = p.stdout.Close()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.dead.Store(true)
	if p.onRemove != nil {
		p.onRemove("rpc timeout")
	}
}

// waitForDrainIfNeeded blocks until a previous timed-out read finishes so the
// next RPC does not race two readers on stdout. Caller must hold p.mu; unlocks
// while waiting and re-locks before return.
func (p *pluginProcess) waitForDrainIfNeeded() error {
	drain := p.drainDone
	if drain == nil {
		return nil
	}
	p.mu.Unlock()
	select {
	case <-drain:
	case <-p.done:
	case <-time.After(toolCallTimeout):
		p.mu.Lock()
		if p.drainDone == drain {
			p.killForTimeout()
			p.drainDone = nil
		}
		p.mu.Unlock()
		return fmt.Errorf("plugin wedged after prior RPC timeout")
	}
	p.mu.Lock()
	if p.drainDone == drain {
		p.drainDone = nil
	}
	if p.dead.Load() {
		return fmt.Errorf("process is dead")
	}
	return nil
}

// sendRPC sends a JSON-RPC request and reads the response with a timeout.
func (p *pluginProcess) sendRPC(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.dead.Load() {
		return nil, fmt.Errorf("process is dead")
	}
	if err := p.waitForDrainIfNeeded(); err != nil {
		return nil, err
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

	// Use a channel + goroutine for timeout on the blocking read.
	type rpcResult struct {
		resp pkgplugin.JSONRPCResponse
		err  error
	}
	ch := make(chan rpcResult, 1)
	safego.Go(func() {
		resp, err := p.readMatchingResponse(req.ID)
		ch <- rpcResult{resp: resp, err: err}
	})

	select {
	case r := <-ch:
		p.consecutiveTimeouts.Store(0)
		if r.err != nil {
			return nil, fmt.Errorf("decode response: %w", r.err)
		}
		if r.resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", r.resp.Error.Code, r.resp.Error.Message)
		}
		return r.resp.Result, nil
	case <-time.After(timeout):
		n := p.consecutiveTimeouts.Add(1)
		if int(n) >= maxRPCTimeouts {
			// Process appears wedged — kill so callers are not stuck forever.
			p.killForTimeout()
			return nil, fmt.Errorf("timeout waiting for response to %s", method)
		}
		// Fail this call but keep the process alive. Drain the late response
		// in the background so the next CallTool can proceed if the plugin recovers.
		drain := make(chan struct{})
		p.drainDone = drain
		safego.Go(func() {
			<-ch
			close(drain)
		})
		return nil, fmt.Errorf("timeout waiting for response to %s", method)
	case <-p.done:
		return nil, fmt.Errorf("process exited while waiting for response to %s", method)
	}
}

// readMatchingResponse reads stdout line-by-line, skipping empty lines and
// non-JSON pollution, until it finds a JSON-RPC response whose ID matches.
func (p *pluginProcess) readMatchingResponse(wantID int64) (pkgplugin.JSONRPCResponse, error) {
	for {
		line, err := p.reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var resp pkgplugin.JSONRPCResponse
			if jsonErr := json.Unmarshal(line, &resp); jsonErr == nil && resp.ID == wantID {
				return resp, nil
			}
		}
		if err != nil {
			return pkgplugin.JSONRPCResponse{}, err
		}
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

// CallTool invokes a tool on the plugin that owns it.
// Transport/RPC failures return a Go error. Tool-level failures
// (ToolCallResult.IsError) return content with isError=true and a nil error
// so Registry.Execute can surface them as ExecuteResult.IsError.
func (m *Manager) CallTool(pluginID, toolName string, args json.RawMessage, sessionID string) (content string, isError bool, err error) {
	m.mu.RLock()
	proc, ok := m.plugins[pluginID]
	dir := m.directory
	m.mu.RUnlock()

	if !ok {
		return "", false, fmt.Errorf("plugin %s not found", pluginID)
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
		return "", false, err
	}

	var result pkgplugin.ToolCallResult
	if raw != nil {
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", false, fmt.Errorf("unmarshal tool result: %w", err)
		}
	}

	return result.Content, result.IsError, nil
}

// registerPluginTools registers each tool declared by the plugin on the
// optional tool registry with ID plugin__{pluginName}__{toolName}.
func (m *Manager) registerPluginTools(proc *pluginProcess) {
	m.mu.RLock()
	reg := m.toolReg
	m.mu.RUnlock()
	if reg == nil || len(proc.tools) == 0 {
		return
	}

	pluginID := proc.info.ID
	pluginName := proc.info.Name
	ids := make([]string, 0, len(proc.tools))
	for _, tm := range proc.tools {
		tm := tm
		toolID := fmt.Sprintf("plugin__%s__%s", pluginName, tm.Name)
		toolName := tm.Name
		params := tm.InputSchema
		if params == nil {
			params = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		perm := tm.Permission
		if perm == "" {
			perm = "plugin"
		}
		reg.Register(&tool.Def{
			ID:          toolID,
			Description: tm.Description,
			Parameters:  params,
			Permission:  perm,
			Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
				content, isErr, callErr := m.CallTool(pluginID, toolName, args, tc.SessionID)
				if callErr != nil {
					return &tool.ExecuteResult{Output: callErr.Error(), IsError: true}, nil
				}
				return &tool.ExecuteResult{Output: content, IsError: isErr}, nil
			},
		})
		ids = append(ids, toolID)
	}
	proc.toolIDs = ids
}

// unregisterPluginTools removes tools previously registered for the plugin.
func (m *Manager) unregisterPluginTools(proc *pluginProcess) {
	m.mu.RLock()
	reg := m.toolReg
	m.mu.RUnlock()
	if reg == nil || len(proc.toolIDs) == 0 {
		return
	}
	for _, id := range proc.toolIDs {
		reg.Unregister(id)
	}
	proc.toolIDs = nil
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
