package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
)

// makeDeferredDef creates a minimal deferred tool definition for testing.
func makeDeferredDef(id, desc string) *Def {
	return &Def{
		ID:          id,
		Description: desc,
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"x": map[string]any{"type": "string"}},
		},
		Deferred: true,
		Execute: func(_ context.Context, _ *Context, _ json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "ok"}, nil
		},
	}
}

func newTestRegistry() *Registry {
	r := NewRegistry(&Context{})
	RegisterBuiltins(r)
	return r
}

func TestToolDefs_FewDeferred_AllInline(t *testing.T) {
	r := newTestRegistry()

	// Register a handful of deferred tools — below threshold.
	for i := 0; i < 5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}

	defs := r.ToolDefs(nil)
	nameSet := make(map[string]bool, len(defs))
	for _, d := range defs {
		nameSet[d.Function.Name] = true
	}

	// All deferred tools should be present.
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("mcp__srv__tool_%d", i)
		if !nameSet[name] {
			t.Errorf("expected deferred tool %s in ToolDefs (below threshold)", name)
		}
	}

	// tool_search should NOT be present.
	if nameSet["tool_search"] {
		t.Error("tool_search should not appear when deferral is not active")
	}
}

func TestToolDefs_ManyDeferred_SchemasOmitted(t *testing.T) {
	r := newTestRegistry()

	// Register enough deferred tools to exceed the threshold.
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}

	defs := r.ToolDefs(nil)
	nameSet := make(map[string]bool, len(defs))
	for _, d := range defs {
		nameSet[d.Function.Name] = true
	}

	// Deferred tools should be absent.
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		name := fmt.Sprintf("mcp__srv__tool_%d", i)
		if nameSet[name] {
			t.Errorf("deferred tool %s should be omitted when deferral is active", name)
		}
	}

	// tool_search should be present.
	if !nameSet["tool_search"] {
		t.Error("tool_search should appear when deferral is active")
	}

	// Builtins should still be present.
	if !nameSet["read"] {
		t.Error("builtin tool 'read' should still appear")
	}
}

func TestToolSearch_ReturnsMatchingTools(t *testing.T) {
	r := newTestRegistry()
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}
	r.Register(makeDeferredDef("mcp__weather__get_forecast", "Get weather forecast"))

	results := r.SearchDeferred("weather")
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].Function.Name != "mcp__weather__get_forecast" {
		t.Errorf("expected mcp__weather__get_forecast, got %s", results[0].Function.Name)
	}
}

func TestToolSearch_ActivatesTools(t *testing.T) {
	r := newTestRegistry()
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}

	// Trigger ToolDefs to set deferral active.
	r.ToolDefs(nil)

	// Search and activate tool_0.
	results := r.SearchDeferred("tool_0")
	if len(results) == 0 {
		t.Fatal("expected at least 1 search result")
	}
	names := make([]string, len(results))
	for i, t := range results {
		names[i] = t.Function.Name
	}
	r.ActivateTools(names...)

	// Now tool_0 should appear in ToolDefs.
	defs := r.ToolDefs(nil)
	nameSet := make(map[string]bool, len(defs))
	for _, d := range defs {
		nameSet[d.Function.Name] = true
	}

	if !nameSet["mcp__srv__tool_0"] {
		t.Error("activated tool mcp__srv__tool_0 should appear in ToolDefs")
	}
}

func TestExecute_DeferredToolBlocked(t *testing.T) {
	r := newTestRegistry()
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}

	// Trigger ToolDefs to set deferral active.
	r.ToolDefs(nil)

	out, isErr, err := r.Execute(context.Background(), "mcp__srv__tool_0", json.RawMessage(`{}`), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErr {
		t.Error("expected isError=true for deferred tool call")
	}
	if out == "" {
		t.Error("expected non-empty error message")
	}
}

func TestExecute_ActivatedDeferredToolAllowed(t *testing.T) {
	r := newTestRegistry()
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}

	// Trigger ToolDefs to set deferral active.
	r.ToolDefs(nil)

	// Activate tool_0.
	r.ActivateTools("mcp__srv__tool_0")

	out, isErr, err := r.Execute(context.Background(), "mcp__srv__tool_0", json.RawMessage(`{}`), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isErr {
		t.Errorf("expected isError=false for activated tool, got output: %s", out)
	}
	if out != "ok" {
		t.Errorf("expected output 'ok', got %q", out)
	}
}

func TestToolDefs_SchemaSizeThreshold(t *testing.T) {
	r := newTestRegistry()

	// Create a large schema that exceeds the size threshold with fewer tools.
	bigSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data": map[string]any{
				"type":        "string",
				"description": string(make([]byte, 3*1024)), // 3KB per tool
			},
		},
	}

	// 11 tools * 3KB each > 30KB threshold.
	for i := 0; i < 11; i++ {
		def := makeDeferredDef(
			fmt.Sprintf("mcp__big__tool_%d", i),
			fmt.Sprintf("Big tool %d", i),
		)
		def.Parameters = bigSchema
		r.Register(def)
	}

	defs := r.ToolDefs(nil)
	nameSet := make(map[string]bool, len(defs))
	for _, d := range defs {
		nameSet[d.Function.Name] = true
	}

	// 11 < DeferredToolCountThreshold, but schema size exceeds threshold.
	if !nameSet["tool_search"] {
		t.Error("tool_search should appear when schema size threshold is exceeded")
	}

	// Deferred tools should be omitted.
	for i := 0; i < 11; i++ {
		name := fmt.Sprintf("mcp__big__tool_%d", i)
		if nameSet[name] {
			t.Errorf("deferred tool %s should be omitted when schema size threshold exceeded", name)
		}
	}
}

func TestToolSearch_MaxResults(t *testing.T) {
	r := newTestRegistry()
	for i := 0; i < MaxToolSearchResults+10; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__weather_%d", i),
			fmt.Sprintf("Weather tool %d", i),
		))
	}

	results := r.SearchDeferred("weather")
	if len(results) > MaxToolSearchResults {
		t.Errorf("expected at most %d results, got %d", MaxToolSearchResults, len(results))
	}
}

func TestToolSearch_CaseInsensitive(t *testing.T) {
	r := newTestRegistry()
	r.Register(makeDeferredDef("mcp__api__GetUser", "Fetch user profile"))

	results := r.SearchDeferred("GETUSER")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for case-insensitive name match, got %d", len(results))
	}

	results = r.SearchDeferred("fetch user")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for case-insensitive description match, got %d", len(results))
	}
}

func TestToolSearch_DisabledToolsExcluded(t *testing.T) {
	r := newTestRegistry()
	r.Register(makeDeferredDef("mcp__srv__disabled_tool", "A disabled tool"))
	r.SetDisabled(map[string]bool{"mcp__srv__disabled_tool": true})

	results := r.SearchDeferred("disabled")
	if len(results) != 0 {
		t.Errorf("expected 0 results for disabled tool, got %d", len(results))
	}
}

func TestToolSearch_Execute(t *testing.T) {
	r := newTestRegistry()
	for i := 0; i < DeferredToolCountThreshold+5; i++ {
		r.Register(makeDeferredDef(
			fmt.Sprintf("mcp__srv__tool_%d", i),
			fmt.Sprintf("Test tool %d", i),
		))
	}
	r.Register(makeDeferredDef("mcp__weather__forecast", "Get weather forecast"))

	// Trigger deferral.
	r.ToolDefs(nil)

	// Execute tool_search.
	args := json.RawMessage(`{"query": "forecast"}`)
	out, isErr, err := r.Execute(context.Background(), "tool_search", args, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isErr {
		t.Errorf("expected isError=false, got output: %s", out)
	}

	// The searched tool should now be activated.
	if !r.activated.Has("mcp__weather__forecast") {
		t.Error("tool_search should activate matching tools")
	}

	// Activated tool should now be executable.
	out, isErr, err = r.Execute(context.Background(), "mcp__weather__forecast", json.RawMessage(`{}`), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isErr {
		t.Errorf("expected isError=false for activated tool, got output: %s", out)
	}
}

func TestDeferredSchemaSize(t *testing.T) {
	// Verify the helper function calculates schema size correctly.
	tools := []llm.Tool{
		{Function: llm.ToolFunction{Parameters: map[string]any{"type": "object"}}},
		{Function: llm.ToolFunction{Parameters: map[string]any{"type": "string"}}},
	}
	size := deferredSchemaSize(tools)
	if size == 0 {
		t.Error("expected non-zero schema size")
	}

	// Nil parameters should contribute 0.
	tools = append(tools, llm.Tool{Function: llm.ToolFunction{Parameters: nil}})
	sizeWithNil := deferredSchemaSize(tools)
	if sizeWithNil != size {
		t.Errorf("nil parameters should not change size, got %d vs %d", sizeWithNil, size)
	}
}
