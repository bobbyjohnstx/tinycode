package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
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
	Output   string
	IsError  bool
	Metadata map[string]any // optional; e.g. mutation paths for rewind tracking
}

// BeforeHookFunc is called before tool execution. If it returns an error,
// the tool execution is aborted. Otherwise, any additionalContext strings
// are appended to the tool result.
type BeforeHookFunc func(sessionID, toolName, toolArgs string) (additionalContext []string, err error)

// AfterHookFunc is called after tool execution with the tool output.
// toolArgs is the raw argument JSON. If modified is true, modifiedOutput
// replaces the original, including an empty string. Any additionalContext
// strings are appended to the tool result.
type AfterHookFunc func(sessionID, toolName, toolArgs, output string, isError bool) (modifiedOutput string, modifiedIsError bool, modified bool, additionalContext []string)

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

// Clear removes all entries from the set. Used to reset the image dedup
// cache after compaction drops image parts from the message window.
func (s *SafeReadFiles) Clear() {
	s.mu.Lock()
	s.files = make(map[string]bool)
	s.mu.Unlock()
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

// SafeWritableRoots is a mutex-protected list of directory paths the model
// is allowed to write into. Safe for concurrent access across Context copies.
type SafeWritableRoots struct {
	mu    sync.Mutex
	roots []string
}

func NewSafeWritableRoots(initial ...string) *SafeWritableRoots {
	roots := make([]string, 0, len(initial))
	for _, p := range initial {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			roots = append(roots, r)
		} else {
			roots = append(roots, p)
		}
	}
	return &SafeWritableRoots{roots: roots}
}

func (s *SafeWritableRoots) Add(path string) {
	resolved := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		resolved = r
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.roots {
		if r == resolved {
			return
		}
	}
	s.roots = append(s.roots, resolved)
}

// Contains reports whether path is inside any writable root.
// It resolves the directory portion of path so that symlink aliases
// (e.g. /var vs /private/var on macOS) are handled correctly.
func (s *SafeWritableRoots) Contains(path string) bool {
	resolved := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		resolved = r
	} else {
		// File/dir may not exist yet — walk up to find the nearest
		// existing ancestor and resolve that, then reattach the tail.
		tail := ""
		cur := path
		for cur != "/" && cur != "." {
			parent := filepath.Dir(cur)
			tail = filepath.Join(filepath.Base(cur), tail)
			if rp, err2 := filepath.EvalSymlinks(parent); err2 == nil {
				resolved = filepath.Join(rp, tail)
				break
			}
			cur = parent
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, root := range s.roots {
		if isUnderDir(resolved, root) || isUnderDir(path, root) {
			return true
		}
	}
	return false
}

// isUnderDir reports whether path equals root or is a descendant of root.
func isUnderDir(path, root string) bool {
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

func (s *SafeWritableRoots) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]string, len(s.roots))
	copy(cp, s.roots)
	return cp
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
	SubagentCount  *atomic.Int32    // concurrent subagent counter (shared across copies)
	SubagentBudget *atomic.Int32    // per-session spawn budget (shared across copies)
	TaskRoundDone  *atomic.Bool     // set after first foreground task batch completes (shared across copies)
	AutoApprove    bool             // skip permission checks when true
	ReadFiles               *SafeReadFiles   // tracks files the model has read or edited (shared across copies)
	ReadImages              *SafeReadFiles   // tracks image paths already attached (shared across copies)
	DiscoveredInstructions  *SafeReadFiles   // tracks directories already checked for AGENTS.md/CLAUDE.md (shared across copies)
	Findings       *SafeFindings    // accumulated code review findings (shared across copies)
	Notepad        *SafeNotepad     // session scratch notes (shared across copies)
	MonitorManager *MonitorManager  // background process watcher (shared across copies)
	AgentSwitch    *SafeAgentSwitch  // pending plan_enter/plan_exit agent switch (shared across copies)
	WritableRoots  *SafeWritableRoots // directories the model may write into (shared across copies)
	ScopedRules    *SafeScopedRules   // path-scoped rules pending attachment (shared across copies)
	// ToolCallID is the in-flight LLM tool call id (ACP session/request_permission toolCallId).
	ToolCallID string
	// FormatFile runs after a successful write or edit. name is the formatter
	// that matched the file. changed is true when the file bytes differ.
	FormatFile func(ctx context.Context, path string) (name string, changed bool, err error)
	// NotesDir is the per-repo persistent notes directory. Empty when notes are disabled.
	NotesDir string
}

func askToolRef(tc *Context) *permission.ToolRef {
	if tc == nil || tc.ToolCallID == "" {
		return nil
	}
	return &permission.ToolRef{CallID: tc.ToolCallID}
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
	Deferred    bool // true for tools whose schema is omitted until searched
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
	mu             sync.RWMutex
	tools          map[string]*Def
	order          []string
	disabled       map[string]bool
	ctx            *Context
	activated      *SafeReadFiles // deferred tools activated by tool_search
	deferralActive *atomic.Bool   // true when deferral thresholds are met
}

// ToolContext returns the registry's shared tool context.
func (r *Registry) ToolContext() *Context { return r.ctx }

func NewRegistry(toolCtx *Context) *Registry {
	if toolCtx.ReadFiles == nil {
		toolCtx.ReadFiles = NewSafeReadFiles()
	}
	if toolCtx.ReadImages == nil {
		toolCtx.ReadImages = NewSafeReadFiles()
	}
	if toolCtx.DiscoveredInstructions == nil {
		toolCtx.DiscoveredInstructions = NewSafeReadFiles()
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
	if toolCtx.AgentSwitch == nil {
		toolCtx.AgentSwitch = NewSafeAgentSwitch()
	}
	if toolCtx.WritableRoots == nil {
		initDir := toolCtx.Directory
		if resolved, err := filepath.EvalSymlinks(initDir); err == nil {
			initDir = resolved
		}
		toolCtx.WritableRoots = NewSafeWritableRoots(initDir)
	}
	return &Registry{
		tools:          make(map[string]*Def),
		disabled:       make(map[string]bool),
		ctx:            toolCtx,
		activated:      NewSafeReadFiles(),
		deferralActive: &atomic.Bool{},
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

	if def.Deferred && r.deferralActive != nil && r.deferralActive.Load() && !r.activated.Has(name) {
		return fmt.Sprintf("Tool %s is deferred. Use tool_search to find and activate it first.", name), true, nil
	}

	cp := r.ctx.clone()
	cp.SessionID = sessionID
	if id := permission.ToolCallIDFrom(ctx); id != "" {
		cp.ToolCallID = id
	}
	toolCtx := &cp

	// Writable-root enforcement: deny writes outside allowed directories.
	// Runs before permission checks so AutoApprove cannot bypass it.
	if toolCtx.WritableRoots != nil {
		if denied, msg := checkWritableRoots(name, args, toolCtx); denied {
			slog.Warn("write denied by writable roots", "tool", name, "sessionID", sessionID, "detail", msg)
			return msg, true, nil
		}
	}

	// Check permissions if service is available and tool has a permission requirement
	var permNote string
	if toolCtx.Perms != nil && def.Permission != "" && !toolCtx.AutoApprove {
		var askErr error
		permNote, askErr = toolCtx.Perms.Ask(ctx, permission.AskInput{
			SessionID:  sessionID,
			Permission: def.Permission,
			Patterns:   askPatterns(name, args),
			Metadata:   map[string]any{"tool": name, "args": string(args)},
			Ruleset:    toolCtx.Ruleset,
			Tool:       askToolRef(toolCtx),
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

	execCtx := ctx
	var cancel context.CancelFunc
	if d, ok := bodyTimeout(name, args); ok {
		execCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	start := time.Now()
	slog.Info("tool executing", "tool", name, "sessionID", sessionID)
	result, err := def.Execute(execCtx, toolCtx, args)
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
		mod, modErr, changed, actx := toolCtx.AfterHook(sessionID, name, string(args), output, isError)
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
	output = SpillIfTruncated(sessionID, output, &truncated)

	// Append hook context to tool output so the model sees it.
	hookCtx := append(beforeContext, afterContext...)
	if len(hookCtx) > 0 {
		output += "\n\n[Hook Context]\n" + strings.Join(hookCtx, "\n")
	}

	// Append the user's permission note so the model sees it.
	if permNote != "" {
		output += "\n\nUser note: " + permNote
	}

	return output, isError, nil
}

func (r *Registry) ToolDefs(agentPerms []string) []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	permSet := expandPermAliases(agentPerms)

	var nonDeferred []llm.Tool
	var deferred []llm.Tool

	for _, name := range r.order {
		def := r.tools[name]
		if r.disabled[name] {
			continue
		}
		// The "invalid" tool is an internal fallback; never expose it to the LLM.
		if name == "invalid" {
			continue
		}
		// tool_search is handled separately below.
		if name == "tool_search" {
			continue
		}
		if len(agentPerms) > 0 && !permSet[def.Permission] && !permSet[name] && !permSet["*"] {
			continue
		}
		lt := def.ToLLMTool()
		if def.Deferred {
			deferred = append(deferred, lt)
		} else {
			nonDeferred = append(nonDeferred, lt)
		}
	}

	// Check if deferral should be active.
	active := len(deferred) >= DeferredToolCountThreshold || deferredSchemaSize(deferred) >= DeferredSchemaSizeBytes
	if r.deferralActive != nil {
		r.deferralActive.Store(active)
	}

	if active {
		result := make([]llm.Tool, 0, len(nonDeferred)+1)
		result = append(result, nonDeferred...)
		// Include only activated deferred tools.
		for _, lt := range deferred {
			if r.activated != nil && r.activated.Has(lt.Function.Name) {
				result = append(result, lt)
			}
		}
		// Add tool_search when deferral is active.
		if searchDef, ok := r.tools["tool_search"]; ok && !r.disabled["tool_search"] {
			result = append(result, searchDef.ToLLMTool())
		}
		return result
	}

	// No deferral — return all tools normally.
	return append(nonDeferred, deferred...)
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
		tools:          tools,
		order:          order,
		disabled:       disabled,
		ctx:            newCtx,
		activated:      r.activated,
		deferralActive: r.deferralActive,
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
		tools:          tools,
		order:          order,
		disabled:       disabled,
		ctx:            newCtx,
		activated:      r.activated,
		deferralActive: r.deferralActive,
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
		tools:          tools,
		order:          order,
		disabled:       disabled,
		ctx:            newCtx,
		activated:      r.activated,
		deferralActive: r.deferralActive,
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
		tools:          tools,
		order:          order,
		disabled:       disabled,
		ctx:            newCtx,
		activated:      r.activated,
		deferralActive: r.deferralActive,
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
		tools:          tools,
		order:          order,
		disabled:       disabled,
		ctx:            r.ctx,
		activated:      r.activated,
		deferralActive: r.deferralActive,
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
		tools:          tools,
		order:          order,
		disabled:       disabled,
		ctx:            r.ctx,
		activated:      r.activated,
		deferralActive: r.deferralActive,
	}
}

// ClearImageCache resets the image dedup set so images dropped during
// compaction can be re-attached in subsequent turns.
func (r *Registry) ClearImageCache() {
	if r.ctx != nil && r.ctx.ReadImages != nil {
		r.ctx.ReadImages.Clear()
	}
}

// ClearDiscoveredInstructions resets the per-directory instruction discovery
// set so that session switches and compaction get fresh instruction attachments.
func (r *Registry) ClearDiscoveredInstructions() {
	if r.ctx != nil && r.ctx.DiscoveredInstructions != nil {
		r.ctx.DiscoveredInstructions.Clear()
	}
}

// ClearScopedRules resets the scoped rules attached set so rules dropped
// during compaction can be re-attached on subsequent tool calls.
func (r *Registry) ClearScopedRules() {
	if r.ctx != nil && r.ctx.ScopedRules != nil {
		r.ctx.ScopedRules.ClearAttached()
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

// AddWritableRoot adds a directory path to the writable-root list.
// Symlinks are resolved before storing so that alias paths match correctly.
func (r *Registry) AddWritableRoot(path string) {
	if r.ctx != nil && r.ctx.WritableRoots != nil {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		r.ctx.WritableRoots.Add(path)
	}
}

// WritableRootsList returns a copy of the current writable roots.
func (r *Registry) WritableRootsList() []string {
	if r.ctx != nil && r.ctx.WritableRoots != nil {
		return r.ctx.WritableRoots.List()
	}
	return nil
}

// checkWritableRoots enforces that file-modifying tools only write inside
// allowed directories. Returns (true, message) if the write is denied.
func checkWritableRoots(toolName string, args json.RawMessage, tc *Context) (bool, string) {
	paths := extractWritePaths(toolName, args, tc.Directory)
	if len(paths) == 0 {
		return false, ""
	}
	for _, p := range paths {
		if !tc.WritableRoots.Contains(p) {
			return true, fmt.Sprintf("Write denied: %s is outside writable roots", p)
		}
	}
	return false, ""
}

// extractWritePaths returns the absolute file paths a tool will modify.
func extractWritePaths(toolName string, args json.RawMessage, dir string) []string {
	switch toolName {
	case "write", "edit":
		var m struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(args, &m); err != nil || m.FilePath == "" {
			return nil
		}
		return []string{resolvePath(m.FilePath, dir)}

	case "apply_patch":
		var m struct {
			Patch string `json:"patch"`
		}
		if err := json.Unmarshal(args, &m); err != nil || m.Patch == "" {
			return nil
		}
		patches, err := parsePatch(m.Patch)
		if err != nil {
			return nil
		}
		var paths []string
		for _, fp := range patches {
			paths = append(paths, patchTargetPath(fp, dir))
		}
		return paths
	}
	return nil
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
