package lsp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

func TestRegisterTools_AllToolsRegistered(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	expected := []string{"lsp_diagnostics", "lsp_hover", "lsp_definition", "lsp_references", "lsp_symbols"}
	registered := reg.List()

	regSet := make(map[string]bool, len(registered))
	for _, name := range registered {
		regSet[name] = true
	}

	for _, name := range expected {
		if !regSet[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestRegisterTools_ToolDefsHaveCorrectSchema(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	tests := []struct {
		id              string
		wantPermission  string
		wantRequiredKey string // a required parameter we expect
	}{
		{"lsp_diagnostics", "read", "file_path"},
		{"lsp_hover", "read", "file_path"},
		{"lsp_definition", "read", "file_path"},
		{"lsp_references", "read", "file_path"},
		{"lsp_symbols", "read", "query"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			def := reg.Get(tt.id)
			if def == nil {
				t.Fatalf("tool %q not found", tt.id)
			}
			if def.Permission != tt.wantPermission {
				t.Errorf("permission = %q, want %q", def.Permission, tt.wantPermission)
			}
			if def.Description == "" {
				t.Error("description is empty")
			}

			// Check parameters schema has the required key.
			props, ok := def.Parameters["properties"].(map[string]any)
			if !ok {
				t.Fatal("parameters missing 'properties'")
			}
			if _, exists := props[tt.wantRequiredKey]; !exists {
				t.Errorf("parameter %q not found in properties", tt.wantRequiredKey)
			}

			required, ok := def.Parameters["required"].([]string)
			if !ok {
				t.Fatal("parameters missing 'required'")
			}
			found := false
			for _, r := range required {
				if r == tt.wantRequiredKey {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%q not in required list %v", tt.wantRequiredKey, required)
			}
		})
	}
}

func TestDiagnosticsTool_DisabledManager_ReturnsGracefulError(t *testing.T) {
	enabled := false
	mgr := NewManager(t.TempDir(), &Config{Enabled: &enabled})
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_diagnostics")
	if def == nil {
		t.Fatal("lsp_diagnostics not found")
	}

	args := json.RawMessage(`{"file_path": "/tmp/test.go"}`)
	result, err := def.Execute(context.Background(), &tool.Context{Directory: t.TempDir()}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for disabled LSP")
	}
	if result.Output != "LSP is disabled" {
		t.Errorf("output = %q, want %q", result.Output, "LSP is disabled")
	}
}

func TestHoverTool_DisabledManager_ReturnsGracefulError(t *testing.T) {
	enabled := false
	mgr := NewManager(t.TempDir(), &Config{Enabled: &enabled})
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_hover")
	args := json.RawMessage(`{"file_path": "/tmp/test.go", "line": 1, "column": 1}`)
	result, err := def.Execute(context.Background(), &tool.Context{Directory: t.TempDir()}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for disabled LSP")
	}
}

func TestDefinitionTool_DisabledManager_ReturnsGracefulError(t *testing.T) {
	enabled := false
	mgr := NewManager(t.TempDir(), &Config{Enabled: &enabled})
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_definition")
	args := json.RawMessage(`{"file_path": "/tmp/test.go", "line": 1, "column": 1}`)
	result, err := def.Execute(context.Background(), &tool.Context{Directory: t.TempDir()}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for disabled LSP")
	}
}

func TestReferencesTool_DisabledManager_ReturnsGracefulError(t *testing.T) {
	enabled := false
	mgr := NewManager(t.TempDir(), &Config{Enabled: &enabled})
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_references")
	args := json.RawMessage(`{"file_path": "/tmp/test.go", "line": 1, "column": 1}`)
	result, err := def.Execute(context.Background(), &tool.Context{Directory: t.TempDir()}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for disabled LSP")
	}
}

func TestSymbolsTool_DisabledManager_ReturnsGracefulError(t *testing.T) {
	enabled := false
	mgr := NewManager(t.TempDir(), &Config{Enabled: &enabled})
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_symbols")
	args := json.RawMessage(`{"query": "TestFunc"}`)
	result, err := def.Execute(context.Background(), &tool.Context{Directory: t.TempDir()}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for disabled LSP")
	}
}

func TestHoverTool_InvalidLineColumn_ReturnsValidationError(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	tests := []struct {
		name string
		args string
	}{
		{"line zero", `{"file_path": "/tmp/test.go", "line": 0, "column": 1}`},
		{"column zero", `{"file_path": "/tmp/test.go", "line": 1, "column": 0}`},
		{"negative line", `{"file_path": "/tmp/test.go", "line": -1, "column": 1}`},
	}

	def := reg.Get("lsp_hover")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := def.Execute(context.Background(), &tool.Context{Directory: t.TempDir()}, json.RawMessage(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Error("expected IsError=true for invalid line/column")
			}
		})
	}
}

func TestDefinitionTool_InvalidLineColumn_ReturnsValidationError(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_definition")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: t.TempDir()},
		json.RawMessage(`{"file_path": "/tmp/test.go", "line": 0, "column": 1}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid line")
	}
}

func TestReferencesTool_InvalidLineColumn_ReturnsValidationError(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_references")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: t.TempDir()},
		json.RawMessage(`{"file_path": "/tmp/test.go", "line": 1, "column": -5}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid column")
	}
}

func TestDiagnosticsTool_InvalidJSON_ReturnsError(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_diagnostics")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: t.TempDir()},
		json.RawMessage(`{not json}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON args")
	}
	if !containsStr(result.Output, "Invalid arguments") {
		t.Errorf("output = %q, want message containing 'Invalid arguments'", result.Output)
	}
}

func TestHoverTool_InvalidJSON_ReturnsError(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_hover")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: t.TempDir()},
		json.RawMessage(`{bad json}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON args")
	}
}

func TestSymbolsTool_InvalidJSON_ReturnsError(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_symbols")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: t.TempDir()},
		json.RawMessage(`{bad}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON args")
	}
}

func TestDiagnosticsTool_NoServerForFile_ReturnsMessage(t *testing.T) {
	// Manager with no servers configured. A .xyz file has no server.
	mgr := NewManager(t.TempDir(), nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_diagnostics")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: t.TempDir()},
		json.RawMessage(`{"file_path": "/tmp/test.xyz"}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Not an error — just a message indicating no server available.
	if result.Output == "" {
		t.Error("expected non-empty output for missing server")
	}
}

func TestSymbolsTool_NoLanguage_ReturnsMessage(t *testing.T) {
	// Empty directory — no project-language detection.
	dir := t.TempDir()
	mgr := NewManager(dir, nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: dir})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_symbols")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: dir},
		json.RawMessage(`{"query": "main"}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return a message about no LSP server, not a panic.
	if result.Output == "" {
		t.Error("expected non-empty output when no language detected")
	}
}

func TestSymbolsTool_WithExplicitLanguage_NoServer_ReturnsMessage(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, nil)
	defer mgr.Close()

	reg := tool.NewRegistry(&tool.Context{Directory: dir})
	RegisterTools(reg, mgr)

	def := reg.Get("lsp_symbols")
	result, err := def.Execute(
		context.Background(),
		&tool.Context{Directory: dir},
		json.RawMessage(`{"query": "main", "language": "cobol"}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsStr(result.Output, "cobol") {
		t.Errorf("output = %q, expected to mention 'cobol'", result.Output)
	}
}
