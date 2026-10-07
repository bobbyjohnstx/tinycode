package tool

import (
	"context"
	"encoding/json"
	"sync"
)

// SafeAgentSwitch is a mutex-protected holder for a pending agent switch
// requested by the plan_enter/plan_exit tools. Safe for concurrent access
// across Context copies.
type SafeAgentSwitch struct {
	mu     sync.Mutex
	target string
	has    bool
}

// NewSafeAgentSwitch creates an empty pending-switch holder.
func NewSafeAgentSwitch() *SafeAgentSwitch {
	return &SafeAgentSwitch{}
}

// Request records a pending switch to the given agent name, overwriting any
// previously pending (and not yet taken) request.
func (s *SafeAgentSwitch) Request(agent string) {
	s.mu.Lock()
	s.target = agent
	s.has = true
	s.mu.Unlock()
}

// Take returns the pending agent switch, if any, and clears it.
func (s *SafeAgentSwitch) Take() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.has {
		return "", false
	}
	s.has = false
	target := s.target
	s.target = ""
	return target, true
}

// TakePendingAgentSwitch returns the agent name requested by a recently
// approved plan_enter/plan_exit tool call, if any, clearing the pending
// request. Implements the session package's AgentSwitcher interface.
func (r *Registry) TakePendingAgentSwitch() (string, bool) {
	if r.ctx == nil || r.ctx.AgentSwitch == nil {
		return "", false
	}
	return r.ctx.AgentSwitch.Take()
}

// PlanEnterTool switches the active session agent to "plan" once approved.
// The permission check ("plan_enter") runs automatically via Registry.Execute
// before Execute is invoked.
func PlanEnterTool() *Def {
	return &Def{
		ID:          "plan_enter",
		Description: "Switch the session into plan mode: read-only research and planning, with edits restricted to plans/* and drafts/*. Requires user approval.",
		Permission:  "plan_enter",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, tc *Context, _ json.RawMessage) (*ExecuteResult, error) {
			if tc.AgentSwitch != nil {
				tc.AgentSwitch.Request("plan")
			}
			return &ExecuteResult{Output: "Switched to plan mode. Edits are now restricted to plans/* and drafts/*."}, nil
		},
	}
}

// PlanExitTool switches the active session agent back to "build" once approved.
// The permission check ("plan_exit") runs automatically via Registry.Execute
// before Execute is invoked.
func PlanExitTool() *Def {
	return &Def{
		ID:          "plan_exit",
		Description: "Exit plan mode and return to the build agent with full tool access. Requires user approval.",
		Permission:  "plan_exit",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, tc *Context, _ json.RawMessage) (*ExecuteResult, error) {
			if tc.AgentSwitch != nil {
				tc.AgentSwitch.Request("build")
			}
			return &ExecuteResult{Output: "Switched to build mode. Full tool access restored."}, nil
		},
	}
}
