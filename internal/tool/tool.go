package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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

// AfterHookFunc is called after tool execution with the tool output.
// If it returns non-empty modifiedOutput, that replaces the original.
type AfterHookFunc func(sessionID, toolName, output string, isError bool) (modifiedOutput string, modifiedIsError bool, modified bool)

type SubagentRunnerFunc func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error)

type Context struct {
	SessionID      string
	Directory      string
	Perms          *permission.Service
	Bus            *bus.Bus
	JobManager     *session.JobManager
	SubagentRunner SubagentRunnerFunc
	SubagentDepth  int
	DB             *sql.DB
	AfterHook      AfterHookFunc
	SubagentCount  *atomic.Int32 // concurrent subagent counter (shared across copies)
	SubagentBudget *atomic.Int32 // per-session spawn budget (shared across copies)
	TaskRoundDone  *atomic.Bool  // set after first foreground task batch completes (shared across copies)
	AutoApprove    bool          // skip permission checks when true
	ReadFiles      map[string]bool // tracks files the model has read or edited (shared across copies)
	Findings       *[]Finding    // accumulated code review findings (shared across copies)
	Notepad        *map[string]string // session scratch notes (shared across copies)
	MonitorManager *MonitorManager // background process watcher (shared across copies)
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
	mu         sync.RWMutex
	tools      map[string]*Def
	order      []string
	disabled   map[string]bool
	ctx        *Context
}

func NewRegistry(toolCtx *Context) *Registry {
	if toolCtx.ReadFiles == nil {
		toolCtx.ReadFiles = make(map[string]bool)
	}
	if toolCtx.Findings == nil {
		findings := make([]Finding, 0)
		toolCtx.Findings = &findings
	}
	if toolCtx.Notepad == nil {
		notepad := make(map[string]string)
		toolCtx.Notepad = &notepad
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

	toolCtx := &Context{
		SessionID:      sessionID,
		Directory:      r.ctx.Directory,
		Perms:          r.ctx.Perms,
		Bus:            r.ctx.Bus,
		JobManager:     r.ctx.JobManager,
		SubagentRunner: r.ctx.SubagentRunner,
		SubagentDepth:  r.ctx.SubagentDepth,
		DB:             r.ctx.DB,
		AfterHook:      r.ctx.AfterHook,
		SubagentCount:  r.ctx.SubagentCount,
		SubagentBudget: r.ctx.SubagentBudget,
		TaskRoundDone:  r.ctx.TaskRoundDone,
		AutoApprove:    r.ctx.AutoApprove,
		ReadFiles:      r.ctx.ReadFiles,
		Findings:       r.ctx.Findings,
		Notepad:        r.ctx.Notepad,
		MonitorManager: r.ctx.MonitorManager,
	}

	// Check permissions if service is available and tool has a permission requirement
	if toolCtx.Perms != nil && def.Permission != "" && !toolCtx.AutoApprove {
		askErr := toolCtx.Perms.Ask(ctx, permission.AskInput{
			SessionID:  sessionID,
			Permission: def.Permission,
			Patterns:   []string{name},
			Metadata:   map[string]any{"tool": name, "args": string(args)},
		})
		if askErr != nil {
			slog.Warn("tool permission denied", "tool", name, "sessionID", sessionID, "error", askErr)
			return askErr.Error(), true, nil
		}
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

	if toolCtx.AfterHook != nil {
		if mod, modErr, changed := toolCtx.AfterHook(sessionID, name, output, isError); changed {
			output = mod
			isError = modErr
		}
	}

	if toolCtx.Bus != nil {
		toolCtx.Bus.Publish("tool.execute.after", map[string]any{
			"sessionID": sessionID,
			"tool":      name,
			"output":    output,
			"isError":   isError,
		})
	}

	if !isError {
		truncated := TruncPreview(output)
		output = truncated.Content
	}

	return output, isError, nil
}

func (r *Registry) ToolDefs(agentPerms []string) []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	permSet := make(map[string]bool)
	for _, p := range agentPerms {
		permSet[p] = true
	}

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
		if len(agentPerms) > 0 && !permSet[def.Permission] && !permSet["*"] {
			continue
		}
		tools = append(tools, def.ToLLMTool())
	}
	return tools
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

	newCtx := &Context{
		SessionID:      r.ctx.SessionID,
		Directory:      dir,
		Perms:          r.ctx.Perms,
		Bus:            r.ctx.Bus,
		JobManager:     r.ctx.JobManager,
		SubagentRunner: r.ctx.SubagentRunner,
		SubagentDepth:  r.ctx.SubagentDepth,
		DB:             r.ctx.DB,
		AfterHook:      r.ctx.AfterHook,
		SubagentCount:  r.ctx.SubagentCount,
		SubagentBudget: r.ctx.SubagentBudget,
		TaskRoundDone:  r.ctx.TaskRoundDone,
		AutoApprove:    r.ctx.AutoApprove,
		ReadFiles:      r.ctx.ReadFiles,
		Findings:       r.ctx.Findings,
		Notepad:        r.ctx.Notepad,
		MonitorManager: r.ctx.MonitorManager,
	}

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

	newCtx := &Context{
		SessionID:      r.ctx.SessionID,
		Directory:      r.ctx.Directory,
		Perms:          r.ctx.Perms,
		Bus:            r.ctx.Bus,
		JobManager:     r.ctx.JobManager,
		SubagentRunner: r.ctx.SubagentRunner,
		SubagentDepth:  depth,
		DB:             r.ctx.DB,
		AfterHook:      r.ctx.AfterHook,
		SubagentCount:  r.ctx.SubagentCount,
		SubagentBudget: r.ctx.SubagentBudget,
		TaskRoundDone:  r.ctx.TaskRoundDone,
		AutoApprove:    r.ctx.AutoApprove,
		ReadFiles:      r.ctx.ReadFiles,
		Findings:       r.ctx.Findings,
		Notepad:        r.ctx.Notepad,
		MonitorManager: r.ctx.MonitorManager,
	}

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

	newCtx := &Context{
		SessionID:      r.ctx.SessionID,
		Directory:      r.ctx.Directory,
		Perms:          r.ctx.Perms,
		Bus:            r.ctx.Bus,
		JobManager:     r.ctx.JobManager,
		SubagentRunner: r.ctx.SubagentRunner,
		SubagentDepth:  r.ctx.SubagentDepth,
		DB:             r.ctx.DB,
		AfterHook:      r.ctx.AfterHook,
		SubagentCount:  r.ctx.SubagentCount,
		SubagentBudget: r.ctx.SubagentBudget,
		TaskRoundDone:  r.ctx.TaskRoundDone,
		AutoApprove:    true,
		ReadFiles:      r.ctx.ReadFiles,
		Findings:       r.ctx.Findings,
		Notepad:        r.ctx.Notepad,
		MonitorManager: r.ctx.MonitorManager,
	}

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
