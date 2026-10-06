package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

type ExecuteResult struct {
	Output  string
	IsError bool
}

// BeforeHookFunc is called before tool execution. If it returns an error,
// the tool execution is aborted. Otherwise, any additionalContext strings
// are appended to the tool result.
type BeforeHookFunc func(sessionID, toolName, toolArgs string) (additionalContext []string, err error)

// AfterHookFunc is called after tool execution with the tool output.
// If it returns non-empty modifiedOutput, that replaces the original.
// Any additionalContext strings are appended to the tool result.
type AfterHookFunc func(sessionID, toolName, output string, isError bool) (modifiedOutput string, modifiedIsError bool, modified bool, additionalContext []string)

// ShellEnvHookFunc is called before shell command execution so plugins can
// inject or modify environment variables. Returning nil or an empty map
// leaves default process environment inheritance.
type ShellEnvHookFunc func(sessionID, directory string, env map[string]string) map[string]string

type SubagentRunnerFunc func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error)

// SafeReadFiles is a mutex-protected set of file paths that tracks which files
// the model has read or edited. Safe for concurrent access across Context copies.
type SafeReadFiles struct {
	mu    sync.Mutex
	files map[string]bool
}

func NewSafeReadFiles() *SafeReadFiles {
	return &SafeReadFiles{files: make(map[string]bool)}
}

func (s *SafeReadFiles) Mark(path string) {
	s.mu.Lock()
	s.files[path] = true
	s.mu.Unlock()
}

func (s *SafeReadFiles) Has(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.files[path]
}

// SafeNotepad is a mutex-protected key-value store for session scratch notes.
// Safe for concurrent access across Context copies.
type SafeNotepad struct {
	mu    sync.Mutex
	notes map[string]string
}

func NewSafeNotepad() *SafeNotepad {
	return &SafeNotepad{notes: make(map[string]string)}
}

func (s *SafeNotepad) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.notes[key]
	return v, ok
}

func (s *SafeNotepad) Set(key, value string) {
	s.mu.Lock()
	s.notes[key] = value
	s.mu.Unlock()
}

func (s *SafeNotepad) Delete(key string) {
	s.mu.Lock()
	delete(s.notes, key)
	s.mu.Unlock()
}

func (s *SafeNotepad) List() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]string, len(s.notes))
	for k, v := range s.notes {
		cp[k] = v
	}
	return cp
}

func (s *SafeNotepad) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.notes)
}

// SafeFindings is a mutex-protected slice of code review findings.
// Safe for concurrent access across Context copies.
type SafeFindings struct {
	mu       sync.Mutex
	findings []Finding
}

func NewSafeFindings() *SafeFindings {
	return &SafeFindings{findings: make([]Finding, 0)}
}

func (s *SafeFindings) Add(items ...Finding) {
	s.mu.Lock()
	s.findings = append(s.findings, items...)
	s.mu.Unlock()
}

func (s *SafeFindings) All() []Finding {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Finding, len(s.findings))
	copy(cp, s.findings)
	return cp
}

func (s *SafeFindings) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.findings)
}

type Context struct {
	SessionID      string
	Directory      string
	Perms          *permission.Service
	Ruleset        permission.Ruleset // agent permission rules for Ask evaluation
	Bus            *bus.Bus
	JobManager     *session.JobManager
	SubagentRunner SubagentRunnerFunc
	SubagentDepth  int
	DB             *sql.DB
	BeforeHook     BeforeHookFunc
	AfterHook      AfterHookFunc
	ShellEnvHook   ShellEnvHookFunc
	SubagentCount  *atomic.Int32   // concurrent subagent counter (shared across copies)
	SubagentBudget *atomic.Int32   // per-session spawn budget (shared across copies)
	TaskRoundDone  *atomic.Bool    // set after first foreground task batch completes (shared across copies)
	AutoApprove    bool            // skip permission checks when true
	ReadFiles      *SafeReadFiles  // tracks files the model has read or edited (shared across copies)
	Findings       *SafeFindings   // accumulated code review findings (shared across copies)
	Notepad        *SafeNotepad    // session scratch notes (shared across copies)
	MonitorManager *MonitorManager // background process watcher (shared across copies)
}

// clone returns a shallow copy of the Context. Pointer/interface fields
// (ReadFiles, Notepad, Findings, MonitorManager, etc.) are intentionally
// shared across copies so mutations propagate.
func (c Context) clone() Context {
	return c
}

type Def struct {
	ID          string
	Description string
	Parameters  map[string]any
	Permission  string
	Execute     func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error)
}

func (d *Def) ToLLMTool() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        d.ID,
			Description: d.Description,
			Parameters:  d.Parameters,
		},
	}
}

type Registry struct {
	mu       sync.RWMutex
	tools    map[string]*Def
	order    []string
	disabled map[string]bool
	ctx      *Context
}

func NewRegistry(toolCtx *Context) *Registry {
	if toolCtx.ReadFiles == nil {
		toolCtx.ReadFiles = NewSafeReadFiles()
	}
	if toolCtx.Findings == nil {
		toolCtx.Findings = NewSafeFindings()
	}
	if toolCtx.Notepad == nil {
		toolCtx.Notepad = NewSafeNotepad()
	}
	if toolCtx.MonitorManager == nil {
		toolCtx.MonitorManager = NewMonitorManager()
	}
	return &Registry{
		tools:    make(map[string]*Def),
		disabled: make(map[string]bool),
		ctx:      toolCtx,
	}
}

func (r *Registry) Register(def *Def) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[def.ID]; !exists {
		r.order = append(r.order, def.ID)
	}
	r.tools[def.ID] = def
}

// Unregister removes a tool by ID from the registry.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[id]; !exists {
		return
	}
	delete(r.tools, id)
	delete(r.disabled, id)
	for i, name := range r.order {
		if name == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

func (r *Registry) SetDisabled(disabled map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disabled = disabled
}

func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error) {
	r.mu.RLock()
	def, ok := r.tools[name]
	disabled := r.disabled[name]
	r.mu.RUnlock()

	if !ok {
		slog.Warn("tool not found", "tool", name, "sessionID", sessionID)
		return fmt.Sprintf("Unknown tool: %s", name), true, nil
	}

	if disabled {
		slog.Warn("tool disabled", "tool", name, "sessionID", sessionID)
		return fmt.Sprintf("Tool %s is disabled", name), true, nil
	}

	cp := r.ctx.clone()
	cp.SessionID = sessionID
	toolCtx := &cp

	// Check permissions if service is available and tool has a permission requirement
	if toolCtx.Perms != nil && def.Permission != "" && !toolCtx.AutoApprove {
		askErr := toolCtx.Perms.Ask(ctx, permission.AskInput{
			SessionID:  sessionID,
			Permission: def.Permission,
			Patterns:   askPatterns(name, args),
			Metadata:   map[string]any{"tool": name, "args": string(args)},
			Ruleset:    toolCtx.Ruleset,
		})
		if askErr != nil {
			slog.Warn("tool permission denied", "tool", name, "sessionID", sessionID, "error", askErr)
			return askErr.Error(), true, nil
		}
	}

	// Before-hook: synchronous, can abort and/or provide context.
	var beforeContext []string
	if toolCtx.BeforeHook != nil {
		bctx, berr := toolCtx.BeforeHook(sessionID, name, string(args))
		if berr != nil {
			slog.Warn("tool before-hook aborted", "tool", name, "sessionID", sessionID, "error", berr)
			return berr.Error(), true, nil
		}
		beforeContext = bctx
	}

	if toolCtx.Bus != nil {
		toolCtx.Bus.Publish("tool.execute.before", map[string]any{
			"sessionID": sessionID,
			"tool":      name,
			"args":      string(args),
		})
	}

	start := time.Now()
	slog.Info("tool executing", "tool", name, "sessionID", sessionID)
	result, err := def.Execute(ctx, toolCtx, args)
	elapsed := time.Since(start)

	if err != nil {
		slog.Error("tool execution error", "tool", name, "sessionID", sessionID, "elapsed", elapsed, "error", err)
		if toolCtx.Bus != nil {
			toolCtx.Bus.Publish("tool.execute.after", map[string]any{
				"sessionID": sessionID,
				"tool":      name,
				"success":   false,
			})
		}
		return err.Error(), true, nil
	}

	slog.Info("tool completed", "tool", name, "sessionID", sessionID, "elapsed", elapsed, "isError", result.IsError, "outputLen", len(result.Output))

	output := result.Output
	isError := result.IsError

	var afterContext []string
	if toolCtx.AfterHook != nil {
		mod, modErr, changed, actx := toolCtx.AfterHook(sessionID, name, output, isError)
		if changed {
			output = mod
			isError = modErr
		}
		afterContext = actx
	}

	if toolCtx.Bus != nil {
		toolCtx.Bus.Publish("tool.execute.after", map[string]any{
			"sessionID": sessionID,
			"tool":      name,
			"output":    output,
			"isError":   isError,
		})
	}

	truncated := TruncPreview(output)
	output = truncated.Content

	// Append hook context to tool output so the model sees it.
	hookCtx := append(beforeContext, afterContext...)
	if len(hookCtx) > 0 {
		output += "\n\n[Hook Context]\n" + strings.Join(hookCtx, "\n")
	}

	return output, isError, nil
}

func (r *Registry) ToolDefs(agentPerms []string) []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	permSet := expandPermAliases(agentPerms)

	var tools []llm.Tool
	for _, name := range r.order {
		def := r.tools[name]
		if r.disabled[name] {
			continue
		}
		// The "invalid" tool is an internal fallback; never expose it to the LLM.
		if name == "invalid" {
			continue
		}
		if len(agentPerms) > 0 && !permSet[def.Permission] && !permSet[name] && !permSet["*"] {
			continue
		}
		tools = append(tools, def.ToLLMTool())
	}
	return tools
}

// expandPermAliases builds a permission set that treats bash↔shell and
// list↔glob as interchangeable allowlist entries.
func expandPermAliases(agentPerms []string) map[string]bool {
	permSet := make(map[string]bool, len(agentPerms)+4)
	for _, p := range agentPerms {
		permSet[p] = true
		switch p {
		case "bash", "shell":
			permSet["bash"] = true
			permSet["shell"] = true
		case "list", "glob":
			permSet["list"] = true
			permSet["glob"] = true
		}
	}
	return permSet
}

func (r *Registry) Get(name string) *Def {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for _, name := range r.order {
		if !r.disabled[name] {
			names = append(names, name)
		}
	}
	return names
}

// WithDirectory returns a shallow copy of the Registry whose tool context uses
// the given directory instead of the original. The copy shares Def pointers
// and is safe for concurrent reads.
func (r *Registry) WithDirectory(dir string) *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cp := r.ctx.clone()
	cp.Directory = dir
	newCtx := &cp

	tools := make(map[string]*Def, len(r.tools))
	for k, v := range r.tools {
		tools[k] = v
	}
	order := make([]string, len(r.order))
	copy(order, r.order)
	disabled := make(map[string]bool, len(r.disabled))
	for k, v := range r.disabled {
		disabled[k] = v
	}

	return &Registry{
		tools:    tools,
		order:    order,
		disabled: disabled,
		ctx:      newCtx,
	}
}

// WithDepth returns a shallow copy of the Registry whose tool context uses
// the given subagent depth instead of the original. The copy shares Def
// pointers and is safe for concurrent reads.
func (r *Registry) WithDepth(depth int) *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cp := r.ctx.clone()
	cp.SubagentDepth = depth
	newCtx := &cp

	tools := make(map[string]*Def, len(r.tools))
	for k, v := range r.tools {
		tools[k] = v
	}
	order := make([]string, len(r.order))
	copy(order, r.order)
	disabled := make(map[string]bool, len(r.disabled))
	for k, v := range r.disabled {
		disabled[k] = v
	}

	return &Registry{
		tools:    tools,
		order:    order,
		disabled: disabled,
		ctx:      newCtx,
	}
}

// WithAutoApprove returns a shallow copy of the Registry whose tool context
// has AutoApprove set to true. The copy shares Def pointers and is safe for
// concurrent reads.
func (r *Registry) WithAutoApprove() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cp := r.ctx.clone()
	cp.AutoApprove = true
	newCtx := &cp

	tools := make(map[string]*Def, len(r.tools))
	for k, v := range r.tools {
		tools[k] = v
	}
	order := make([]string, len(r.order))
	copy(order, r.order)
	disabled := make(map[string]bool, len(r.disabled))
	for k, v := range r.disabled {
		disabled[k] = v
	}

	return &Registry{
		tools:    tools,
		order:    order,
		disabled: disabled,
		ctx:      newCtx,
	}
}

// WithAgentRules returns a shallow copy of the Registry with the agent's
// ruleset applied for Ask evaluation and tools denied by the ruleset marked
// disabled (so ToolDefs/Execute hide them). Existing disabled entries are kept.
func (r *Registry) WithAgentRules(ruleset permission.Ruleset) *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cp := r.ctx.clone()
	cp.Ruleset = ruleset
	newCtx := &cp

	tools := make(map[string]*Def, len(r.tools))
	for k, v := range r.tools {
		tools[k] = v
	}
	order := make([]string, len(r.order))
	copy(order, r.order)

	allNames := make([]string, len(r.order))
	copy(allNames, r.order)
	disabled := permission.Disabled(allNames, ruleset)
	for k, v := range r.disabled {
		if v {
			disabled[k] = true
		}
	}

	return &Registry{
		tools:    tools,
		order:    order,
		disabled: disabled,
		ctx:      newCtx,
	}
}

// WithOnlyTools returns a shallow copy of the Registry that only includes the
// named tools. All other tools are excluded from List() and ToolDefs() output.
// The copy shares Def pointers and the same Context as the original.
func (r *Registry) WithOnlyTools(names ...string) *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keep := make(map[string]bool, len(names))
	for _, n := range names {
		keep[n] = true
	}

	tools := make(map[string]*Def, len(names))
	var order []string
	for _, name := range r.order {
		if keep[name] {
			tools[name] = r.tools[name]
			order = append(order, name)
		}
	}
	disabled := make(map[string]bool, len(r.disabled))
	for k, v := range r.disabled {
		if keep[k] {
			disabled[k] = v
		}
	}

	return &Registry{
		tools:    tools,
		order:    order,
		disabled: disabled,
		ctx:      r.ctx,
	}
}

// Snapshot returns a shallow copy of the Registry with independent maps and
// slices. The copy shares *Def pointers but mutations to the copy's
// maps/slices do not affect the original, making it safe for concurrent use.
func (r *Registry) Snapshot() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make(map[string]*Def, len(r.tools))
	for k, v := range r.tools {
		tools[k] = v
	}
	order := make([]string, len(r.order))
	copy(order, r.order)
	disabled := make(map[string]bool, len(r.disabled))
	for k, v := range r.disabled {
		disabled[k] = v
	}

	return &Registry{
		tools:    tools,
		order:    order,
		disabled: disabled,
		ctx:      r.ctx,
	}
}

// ResetTaskRound clears the TaskRoundDone flag so new task calls are allowed.
// Called at the start of each user prompt to allow a fresh round of tasks.
func (r *Registry) ResetTaskRound() {
	if r.ctx != nil && r.ctx.TaskRoundDone != nil {
		r.ctx.TaskRoundDone.Store(false)
	}
}

// DrainMonitorOutput returns any buffered output from background monitors
// and clears the buffers. Returns empty string if nothing to report.
func (r *Registry) DrainMonitorOutput() string {
	if r.ctx != nil && r.ctx.MonitorManager != nil {
		return r.ctx.MonitorManager.DrainAll()
	}
	return ""
}

// ShutdownMonitors cancels all running background monitors.
func (r *Registry) ShutdownMonitors() {
	if r.ctx != nil && r.ctx.MonitorManager != nil {
		r.ctx.MonitorManager.Shutdown()
	}
}

// ResetBudget resets the shared SubagentBudget counter to the given value.
// Called at the start of each user prompt so budget exhaustion in one prompt
// doesn't block future prompts.
func (r *Registry) ResetBudget(value int32) {
	if r.ctx != nil && r.ctx.SubagentBudget != nil {
		r.ctx.SubagentBudget.Store(value)
	}
}

// askPatterns extracts path or command strings from tool args for permission
// evaluation. Falls back to the tool name when no usable pattern is present.
func askPatterns(toolName string, args json.RawMessage) []string {
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil || m == nil {
		return []string{toolName}
	}

	switch toolName {
	case "bash", "shell", "monitor":
		if cmd, ok := m["command"].(string); ok && cmd != "" {
			return []string{cmd}
		}
	}

	for _, key := range []string{"file_path", "path", "file", "filepath"} {
		if v, ok := m[key].(string); ok && v != "" {
			return []string{v}
		}
	}

	return []string{toolName}
}
