package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
)

func TestPlanEnterTool_Registered(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	def := r.Get("plan_enter")
	if def == nil {
		t.Fatal("plan_enter tool not registered")
	}
	if def.Permission != "plan_enter" {
		t.Errorf("expected permission %q, got %q", "plan_enter", def.Permission)
	}
}

func TestPlanExitTool_Registered(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	def := r.Get("plan_exit")
	if def == nil {
		t.Fatal("plan_exit tool not registered")
	}
	if def.Permission != "plan_exit" {
		t.Errorf("expected permission %q, got %q", "plan_exit", def.Permission)
	}
}

func TestPlanEnterTool_RequestsSwitchToPlan(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	output, isErr, err := r.Execute(context.Background(), "plan_enter", json.RawMessage(`{}`), "ses-plan-enter")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Fatalf("expected no error, got: %s", output)
	}

	target, has := r.TakePendingAgentSwitch()
	if !has {
		t.Fatal("expected a pending agent switch after plan_enter")
	}
	if target != "plan" {
		t.Errorf("expected pending switch to %q, got %q", "plan", target)
	}

	// A second take should find nothing pending (already consumed).
	if _, has := r.TakePendingAgentSwitch(); has {
		t.Error("expected pending switch to be cleared after Take")
	}
}

func TestPlanExitTool_RequestsSwitchToBuild(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	output, isErr, err := r.Execute(context.Background(), "plan_exit", json.RawMessage(`{}`), "ses-plan-exit")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if isErr {
		t.Fatalf("expected no error, got: %s", output)
	}

	target, has := r.TakePendingAgentSwitch()
	if !has {
		t.Fatal("expected a pending agent switch after plan_exit")
	}
	if target != "build" {
		t.Errorf("expected pending switch to %q, got %q", "build", target)
	}
}

func TestPlanSwitchTools_ToolDefsFilteredByAgentPerms(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	// "build" agent: plan_enter allowed, plan_exit not.
	buildTools := r.ToolDefs([]string{"read", "edit", "shell", "plan_enter"})
	if !hasTool(buildTools, "plan_enter") {
		t.Error("expected plan_enter in build agent's tool defs")
	}
	if hasTool(buildTools, "plan_exit") {
		t.Error("did not expect plan_exit in build agent's tool defs")
	}

	// "plan" agent: plan_exit allowed, plan_enter not.
	planTools := r.ToolDefs([]string{"read", "plan_exit"})
	if !hasTool(planTools, "plan_exit") {
		t.Error("expected plan_exit in plan agent's tool defs")
	}
	if hasTool(planTools, "plan_enter") {
		t.Error("did not expect plan_enter in plan agent's tool defs")
	}
}

func hasTool(tools []llm.Tool, name string) bool {
	for _, tl := range tools {
		if tl.Function.Name == name {
			return true
		}
	}
	return false
}
