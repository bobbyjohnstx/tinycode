package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMergeNonZero_OverlappingScalars_LastWins(t *testing.T) {
	type S struct {
		Name  string
		Count int
		Rate  float64
	}
	dst := S{Name: "alpha", Count: 1, Rate: 0.5}
	src := S{Name: "beta", Count: 2, Rate: 0.9}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Name != "beta" {
		t.Errorf("Name = %q, want %q (last wins)", dst.Name, "beta")
	}
	if dst.Count != 2 {
		t.Errorf("Count = %d, want %d (last wins)", dst.Count, 2)
	}
	if dst.Rate != 0.9 {
		t.Errorf("Rate = %f, want %f (last wins)", dst.Rate, 0.9)
	}
}

func TestMergeNonZero_ZeroValuesDoNotOverwrite(t *testing.T) {
	type S struct {
		Name  string
		Count int
		Rate  float64
		Flag  bool
	}
	dst := S{Name: "keep", Count: 42, Rate: 3.14, Flag: true}
	src := S{} // all zero values

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Name != "keep" {
		t.Errorf("Name = %q, want %q (zero should not overwrite)", dst.Name, "keep")
	}
	if dst.Count != 42 {
		t.Errorf("Count = %d, want %d (zero should not overwrite)", dst.Count, 42)
	}
	if dst.Rate != 3.14 {
		t.Errorf("Rate = %f, want %f (zero should not overwrite)", dst.Rate, 3.14)
	}
	if !dst.Flag {
		t.Error("Flag = false, want true (zero should not overwrite)")
	}
}

func TestMergeNonZero_NilPointerDoesNotOverwrite(t *testing.T) {
	type Inner struct {
		Value string
	}
	type S struct {
		Ptr *Inner
	}
	inner := &Inner{Value: "original"}
	dst := S{Ptr: inner}
	src := S{Ptr: nil}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Ptr == nil {
		t.Fatal("Ptr = nil, want non-nil (nil pointer should not overwrite)")
	}
	if dst.Ptr.Value != "original" {
		t.Errorf("Ptr.Value = %q, want %q", dst.Ptr.Value, "original")
	}
}

func TestMergeNonZero_NonNilPointerMergesStructFields(t *testing.T) {
	type Inner struct {
		Name string
		Age  int
	}
	type S struct {
		Ptr *Inner
	}
	dst := S{Ptr: &Inner{Name: "alice", Age: 30}}
	src := S{Ptr: &Inner{Name: "bob"}} // Age is zero

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Ptr.Name != "bob" {
		t.Errorf("Ptr.Name = %q, want %q (non-zero overrides)", dst.Ptr.Name, "bob")
	}
	if dst.Ptr.Age != 30 {
		t.Errorf("Ptr.Age = %d, want %d (zero should not overwrite)", dst.Ptr.Age, 30)
	}
}

func TestMergeNonZero_SliceReplacement(t *testing.T) {
	type S struct {
		Items []string
	}
	dst := S{Items: []string{"a", "b"}}
	src := S{Items: []string{"c"}}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if !reflect.DeepEqual(dst.Items, []string{"c"}) {
		t.Errorf("Items = %v, want [c] (non-empty slice replaces)", dst.Items)
	}
}

func TestMergeNonZero_EmptySliceDoesNotOverwrite(t *testing.T) {
	type S struct {
		Items []string
	}
	dst := S{Items: []string{"a", "b"}}
	src := S{Items: []string{}}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if !reflect.DeepEqual(dst.Items, []string{"a", "b"}) {
		t.Errorf("Items = %v, want [a b] (empty slice should not overwrite)", dst.Items)
	}
}

func TestMergeNonZero_MapMerge(t *testing.T) {
	type S struct {
		Data map[string]string
	}
	dst := S{Data: map[string]string{"x": "1"}}
	src := S{Data: map[string]string{"y": "2"}}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Data["x"] != "1" {
		t.Error("expected existing key preserved")
	}
	if dst.Data["y"] != "2" {
		t.Error("expected new key merged")
	}
}

func TestMergeNonZero_MapIntoNilMap(t *testing.T) {
	type S struct {
		Data map[string]string
	}
	dst := S{Data: nil}
	src := S{Data: map[string]string{"key": "val"}}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Data == nil {
		t.Fatal("Data = nil, want non-nil (should create map)")
	}
	if dst.Data["key"] != "val" {
		t.Errorf("Data[key] = %q, want %q", dst.Data["key"], "val")
	}
}

func TestMergeNonZero_InvalidOrMismatchedTypesAreNoOp(t *testing.T) {
	type A struct{ X int }
	type B struct{ Y string }
	a := A{X: 1}
	b := B{Y: "test"}

	dv := reflect.ValueOf(&a).Elem()
	sv := reflect.ValueOf(&b).Elem()
	mergeNonZero(dv, sv) // mismatched types — should not panic

	if a.X != 1 {
		t.Errorf("X = %d, want 1 (mismatched types should be no-op)", a.X)
	}
}

func TestMergeNonZero_BoolOverridesOnlyWhenTrue(t *testing.T) {
	type S struct {
		Flag bool
	}
	dst := S{Flag: true}
	src := S{Flag: false} // zero value

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if !dst.Flag {
		t.Error("Flag = false, want true (bool zero should not overwrite)")
	}
}

func TestMergeNonZero_UintTypes(t *testing.T) {
	type S struct {
		Val uint64
	}
	dst := S{Val: 100}
	src := S{Val: 200}

	dv := reflect.ValueOf(&dst).Elem()
	sv := reflect.ValueOf(&src).Elem()
	mergeNonZero(dv, sv)

	if dst.Val != 200 {
		t.Errorf("Val = %d, want 200 (non-zero uint should overwrite)", dst.Val)
	}
}

// --- Collection field merge tests ---

func TestMergeCollectionFields_InstructionsAppendAndDedup(t *testing.T) {
	dst := &Info{Instructions: []string{"rule-a"}}
	src := &Info{Instructions: []string{"rule-b", "rule-a"}}

	result := Merge(dst, src)

	if len(result.Instructions) != 2 {
		t.Fatalf("expected 2 instructions (deduplicated), got %d: %v", len(result.Instructions), result.Instructions)
	}
	if result.Instructions[0] != "rule-a" || result.Instructions[1] != "rule-b" {
		t.Errorf("expected [rule-a rule-b], got %v", result.Instructions)
	}
}

func TestMergeCollectionFields_HooksAppend(t *testing.T) {
	dst := &Info{
		Hooks: map[string][]HookConfig{
			"before": {{Command: "echo global"}},
		},
	}
	src := &Info{
		Hooks: map[string][]HookConfig{
			"before": {{Command: "echo project"}},
			"after":  {{Command: "echo done"}},
		},
	}

	result := Merge(dst, src)

	beforeHooks := result.Hooks["before"]
	if len(beforeHooks) != 2 {
		t.Fatalf("expected 2 before hooks (appended), got %d", len(beforeHooks))
	}
	if beforeHooks[0].Command != "echo global" {
		t.Errorf("first before hook = %q, want %q", beforeHooks[0].Command, "echo global")
	}
	if beforeHooks[1].Command != "echo project" {
		t.Errorf("second before hook = %q, want %q", beforeHooks[1].Command, "echo project")
	}
	if len(result.Hooks["after"]) != 1 {
		t.Errorf("expected 1 after hook, got %d", len(result.Hooks["after"]))
	}
}

func TestMergeCollectionFields_AgentsMergeByKey(t *testing.T) {
	dst := &Info{
		Agents: map[string]json.RawMessage{
			"builder": json.RawMessage(`{"model":"fast"}`),
		},
	}
	src := &Info{
		Agents: map[string]json.RawMessage{
			"planner": json.RawMessage(`{"model":"slow"}`),
		},
	}

	result := Merge(dst, src)

	if len(result.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(result.Agents))
	}
	if string(result.Agents["builder"]) != `{"model":"fast"}` {
		t.Error("expected builder agent preserved")
	}
	if string(result.Agents["planner"]) != `{"model":"slow"}` {
		t.Error("expected planner agent added")
	}
}

func TestMergeCollectionFields_MCPMergeByKey(t *testing.T) {
	dst := &Info{
		MCP: map[string]MCPConfig{
			"server-a": {URL: "http://a"},
		},
	}
	src := &Info{
		MCP: map[string]MCPConfig{
			"server-b": {URL: "http://b"},
		},
	}

	result := Merge(dst, src)

	if len(result.MCP) != 2 {
		t.Fatalf("expected 2 MCP servers, got %d", len(result.MCP))
	}
}

func TestMergeScalarFields_LSPConfigMerge(t *testing.T) {
	enabled := true
	timeout := 60
	dst := &Info{LSP: &LSPConfig{Enabled: &enabled}}
	src := &Info{LSP: &LSPConfig{Timeout: &timeout}}

	result := Merge(dst, src)

	if result.LSP == nil {
		t.Fatal("expected LSP config")
	}
	if result.LSP.Enabled == nil || !*result.LSP.Enabled {
		t.Error("expected LSP enabled preserved from dst")
	}
	if result.LSP.Timeout == nil || *result.LSP.Timeout != 60 {
		t.Error("expected LSP timeout 60 from src")
	}
}

func TestMergeScalarFields_EffortOverride(t *testing.T) {
	dst := &Info{Effort: "low"}
	src := &Info{Effort: "high"}

	result := Merge(dst, src)

	if result.Effort != "high" {
		t.Errorf("Effort = %q, want %q", result.Effort, "high")
	}
}

func TestMergeScalarFields_AutoApproveOverride(t *testing.T) {
	val := true
	dst := &Info{}
	src := &Info{AutoApprove: &val}

	result := Merge(dst, src)

	if result.AutoApprove == nil || !*result.AutoApprove {
		t.Error("expected AutoApprove true from src")
	}
}

func TestMergeScalarFields_ThemeOverride(t *testing.T) {
	dst := &Info{Theme: "dark"}
	src := &Info{Theme: "light"}

	result := Merge(dst, src)

	if result.Theme != "light" {
		t.Errorf("Theme = %q, want %q", result.Theme, "light")
	}
}

func TestMergeScalarFields_EmptyStringDoesNotOverwrite(t *testing.T) {
	dst := &Info{Shell: "/bin/zsh", Model: "provider/fast"}
	src := &Info{} // all zero

	result := Merge(dst, src)

	if result.Shell != "/bin/zsh" {
		t.Errorf("Shell = %q, want %q (empty should not overwrite)", result.Shell, "/bin/zsh")
	}
	if result.Model != "provider/fast" {
		t.Errorf("Model = %q, want %q (empty should not overwrite)", result.Model, "provider/fast")
	}
}

// --- LSPConfig.UnmarshalJSON tests ---

func TestLSPConfig_UnmarshalJSON_BooleanTrue(t *testing.T) {
	var cfg LSPConfig
	if err := json.Unmarshal([]byte(`true`), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Enabled == nil || !*cfg.Enabled {
		t.Error("expected Enabled=true for boolean shorthand")
	}
	if cfg.Servers != nil {
		t.Error("expected nil Servers for boolean shorthand")
	}
}

func TestLSPConfig_UnmarshalJSON_BooleanFalse(t *testing.T) {
	var cfg LSPConfig
	if err := json.Unmarshal([]byte(`false`), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Enabled == nil || *cfg.Enabled {
		t.Error("expected Enabled=false for boolean shorthand")
	}
}

func TestLSPConfig_UnmarshalJSON_FullObject(t *testing.T) {
	input := `{
		"enabled": true,
		"timeout": 30,
		"servers": {
			"go": {"command": "gopls", "args": ["serve"]}
		}
	}`
	var cfg LSPConfig
	if err := json.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Enabled == nil || !*cfg.Enabled {
		t.Error("expected Enabled=true from object")
	}
	if cfg.Timeout == nil || *cfg.Timeout != 30 {
		t.Errorf("Timeout = %v, want 30", cfg.Timeout)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	goServer, ok := cfg.Servers["go"]
	if !ok {
		t.Fatal("expected 'go' server")
	}
	if goServer.Command != "gopls" {
		t.Errorf("go server command = %q, want %q", goServer.Command, "gopls")
	}
	if !reflect.DeepEqual(goServer.Args, []string{"serve"}) {
		t.Errorf("go server args = %v, want [serve]", goServer.Args)
	}
}

func TestLSPConfig_UnmarshalJSON_ObjectWithoutEnabled(t *testing.T) {
	input := `{"timeout": 120}`
	var cfg LSPConfig
	if err := json.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Enabled != nil {
		t.Errorf("expected Enabled=nil (omitted), got %v", *cfg.Enabled)
	}
	if cfg.Timeout == nil || *cfg.Timeout != 120 {
		t.Errorf("Timeout = %v, want 120", cfg.Timeout)
	}
}

func TestLSPConfig_UnmarshalJSON_InvalidInput(t *testing.T) {
	var cfg LSPConfig
	err := json.Unmarshal([]byte(`"not valid"`), &cfg)
	if err == nil {
		t.Error("expected error for string input, got nil")
	}
}

func TestLSPConfig_RoundTrip_InInfoStruct(t *testing.T) {
	// Test that LSPConfig works within the full Info struct (both forms).
	tests := []struct {
		name     string
		input    string
		wantBool bool
	}{
		{
			name:     "boolean shorthand in full config",
			input:    `{"lsp": true}`,
			wantBool: true,
		},
		{
			name:     "object form in full config",
			input:    `{"lsp": {"enabled": true, "timeout": 60}}`,
			wantBool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseConfig(tt.input, nil)
			if err != nil {
				t.Fatalf("ParseConfig: %v", err)
			}
			if info.LSP == nil {
				t.Fatal("LSP config is nil")
			}
			if info.LSP.Enabled == nil || *info.LSP.Enabled != tt.wantBool {
				t.Errorf("LSP.Enabled = %v, want %v", info.LSP.Enabled, tt.wantBool)
			}
		})
	}
}
