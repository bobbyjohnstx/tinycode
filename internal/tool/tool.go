package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
)

type ExecuteResult struct {
	Output  string
	IsError bool
}

type Context struct {
	SessionID string
	Directory string
	Perms     *permission.Service
	Bus       *bus.Bus
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
	tools      map[string]*Def
	order      []string
	disabled   map[string]bool
	ctx        *Context
}

func NewRegistry(toolCtx *Context) *Registry {
	return &Registry{
		tools:    make(map[string]*Def),
		disabled: make(map[string]bool),
		ctx:      toolCtx,
	}
}

func (r *Registry) Register(def *Def) {
	if _, exists := r.tools[def.ID]; !exists {
		r.order = append(r.order, def.ID)
	}
	r.tools[def.ID] = def
}

func (r *Registry) SetDisabled(disabled map[string]bool) {
	r.disabled = disabled
}

func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error) {
	def, ok := r.tools[name]
	if !ok {
		slog.Warn("tool not found", "tool", name, "sessionID", sessionID)
		return fmt.Sprintf("Unknown tool: %s", name), true, nil
	}

	if r.disabled[name] {
		slog.Warn("tool disabled", "tool", name, "sessionID", sessionID)
		return fmt.Sprintf("Tool %s is disabled", name), true, nil
	}

	toolCtx := &Context{
		SessionID: sessionID,
		Directory: r.ctx.Directory,
		Perms:     r.ctx.Perms,
		Bus:       r.ctx.Bus,
	}

	// Check permissions if service is available and tool has a permission requirement
	if toolCtx.Perms != nil && def.Permission != "" {
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

	if toolCtx.Bus != nil {
		toolCtx.Bus.Publish("tool.execute.after", map[string]any{
			"sessionID": sessionID,
			"tool":      name,
			"success":   !result.IsError,
		})
	}

	output := result.Output
	if !result.IsError {
		truncated := Truncate(output, TruncTail)
		output = truncated.Content
	}

	return output, result.IsError, nil
}

func (r *Registry) ToolDefs(agentPerms []string) []llm.Tool {
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
		if len(agentPerms) > 0 && !permSet[def.Permission] && !permSet["*"] {
			continue
		}
		tools = append(tools, def.ToLLMTool())
	}
	return tools
}

func (r *Registry) Get(name string) *Def {
	return r.tools[name]
}

func (r *Registry) List() []string {
	var names []string
	for _, name := range r.order {
		if !r.disabled[name] {
			names = append(names, name)
		}
	}
	return names
}
