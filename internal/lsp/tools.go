package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

// RegisterTools registers all LSP tools on the tool registry.
func RegisterTools(r *tool.Registry, mgr *Manager) {
	r.Register(diagnosticsTool(mgr))
	r.Register(hoverTool(mgr))
	r.Register(definitionTool(mgr))
	r.Register(referencesTool(mgr))
	r.Register(symbolsTool(mgr))
	r.Register(documentSymbolsTool(mgr))
	r.Register(codeActionsTool(mgr))
	r.Register(renameTool(mgr))
}

func disabledResult() *tool.ExecuteResult {
	return &tool.ExecuteResult{Output: "LSP is disabled", IsError: true}
}

func validateLineColumn(line, column int) error {
	if line < 1 {
		return fmt.Errorf("line must be >= 1, got %d", line)
	}
	if column < 1 {
		return fmt.Errorf("column must be >= 1, got %d", column)
	}
	return nil
}

func diagnosticsTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_diagnostics",
		Description: "Get compiler errors and warnings for a file from the language server (LSP). Returns diagnostics with severity, line number, and message.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to check for errors/warnings",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			diags, err := client.Diagnostics(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP diagnostics failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatDiagnostics(file, diags)}, nil
		},
	}
}

func hoverTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_hover",
		Description: "Get type information and documentation for a symbol at a specific position from the language server (LSP).",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"line": map[string]any{
					"type":        "integer",
					"description": "Line number (1-indexed)",
				},
				"column": map[string]any{
					"type":        "integer",
					"description": "Column number (1-indexed)",
				},
			},
			"required": []string{"file_path", "line", "column"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath string `json:"file_path"`
				Line     int    `json:"line"`
				Column   int    `json:"column"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}
			if err := validateLineColumn(input.Line, input.Column); err != nil {
				return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			content, err := client.Hover(ctx, file, input.Line-1, input.Column-1)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP hover failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatHover(content)}, nil
		},
	}
}

func definitionTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_definition",
		Description: "Go to the definition of a symbol at a specific position using the language server (LSP). Returns the file path and line number of the definition.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"line": map[string]any{
					"type":        "integer",
					"description": "Line number (1-indexed)",
				},
				"column": map[string]any{
					"type":        "integer",
					"description": "Column number (1-indexed)",
				},
			},
			"required": []string{"file_path", "line", "column"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath string `json:"file_path"`
				Line     int    `json:"line"`
				Column   int    `json:"column"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}
			if err := validateLineColumn(input.Line, input.Column); err != nil {
				return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			locs, err := client.Definition(ctx, file, input.Line-1, input.Column-1)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP definition failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatDefinitions(locs)}, nil
		},
	}
}

func referencesTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_references",
		Description: "Find all references to a symbol at a specific position using the language server (LSP). Returns file paths and line numbers.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"line": map[string]any{
					"type":        "integer",
					"description": "Line number (1-indexed)",
				},
				"column": map[string]any{
					"type":        "integer",
					"description": "Column number (1-indexed)",
				},
				"include_declaration": map[string]any{
					"type":        "boolean",
					"description": "Include the declaration in results (default: true)",
				},
			},
			"required": []string{"file_path", "line", "column"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath           string `json:"file_path"`
				Line               int    `json:"line"`
				Column             int    `json:"column"`
				IncludeDeclaration *bool  `json:"include_declaration,omitempty"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}
			if err := validateLineColumn(input.Line, input.Column); err != nil {
				return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
			}

			includeDecl := true
			if input.IncludeDeclaration != nil {
				includeDecl = *input.IncludeDeclaration
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			locs, err := client.References(ctx, file, input.Line-1, input.Column-1, includeDecl)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP references failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatReferences(locs)}, nil
		},
	}
}

func symbolsTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_symbols",
		Description: "Search for symbols (functions, types, variables) across the workspace using the language server (LSP). Useful for finding where something is defined without knowing the file.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Symbol name to search for (fuzzy match)",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Language to search in (go, typescript, python, rust). Auto-detected from project if omitted.",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				Query    string `json:"query"`
				Language string `json:"language,omitempty"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}

			lang := input.Language
			if lang == "" {
				lang = mgr.PreferredLanguage()
			}
			if lang == "" {
				return &tool.ExecuteResult{Output: "No LSP server available. No supported language detected in this project."}, nil
			}

			client := mgr.ClientForLanguage(ctx, lang)
			if client == nil {
				hint := installHints[lang]
				return &tool.ExecuteResult{Output: fmt.Sprintf("No LSP server available for %s. %s", lang, hint)}, nil
			}

			symbols, err := client.Symbols(ctx, input.Query)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP symbols failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatSymbols(symbols)}, nil
		},
	}
}

func documentSymbolsTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_document_symbols",
		Description: "List symbols (functions, types, variables) in a single file as an outline from the language server (LSP).",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to outline",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			symbols, err := client.DocumentSymbols(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP document symbols failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatDocumentSymbols(symbols)}, nil
		},
	}
}

func codeActionsTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_code_actions",
		Description: "List available code actions (quick fixes, refactors) for a file or range from the language server (LSP). Returns titles and edit summaries only — does not apply edits.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"line": map[string]any{
					"type":        "integer",
					"description": "Start line (1-indexed). Required with column when either is set; omit both for full document.",
				},
				"column": map[string]any{
					"type":        "integer",
					"description": "Start column (1-indexed). Required with line when either is set.",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "End line (1-indexed). Defaults to start line when omitted.",
				},
				"end_column": map[string]any{
					"type":        "integer",
					"description": "End column (1-indexed). Defaults to start column when omitted.",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath  string `json:"file_path"`
				Line      *int   `json:"line"`
				Column    *int   `json:"column"`
				EndLine   *int   `json:"end_line"`
				EndColumn *int   `json:"end_column"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}

			hasLine := input.Line != nil
			hasCol := input.Column != nil
			if hasLine != hasCol {
				return &tool.ExecuteResult{Output: "line and column must both be set or both omitted", IsError: true}, nil
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			var rng Range
			if !hasLine {
				content, err := os.ReadFile(file)
				if err != nil {
					return &tool.ExecuteResult{Output: fmt.Sprintf("reading %s: %v", file, err), IsError: true}, nil
				}
				rng = fullDocumentRange(string(content))
			} else {
				if err := validateLineColumn(*input.Line, *input.Column); err != nil {
					return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
				}
				start := Position{Line: *input.Line - 1, Character: *input.Column - 1}
				end := start
				if input.EndLine != nil || input.EndColumn != nil {
					if input.EndLine == nil || input.EndColumn == nil {
						return &tool.ExecuteResult{Output: "end_line and end_column must both be set or both omitted", IsError: true}, nil
					}
					if err := validateLineColumn(*input.EndLine, *input.EndColumn); err != nil {
						return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
					}
					end = Position{Line: *input.EndLine - 1, Character: *input.EndColumn - 1}
				}
				rng = Range{Start: start, End: end}
			}

			actions, err := client.CodeActions(ctx, file, rng)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP code actions failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatCodeActions(actions)}, nil
		},
	}
}

func renameTool(mgr *Manager) *tool.Def {
	return &tool.Def{
		ID:          "lsp_rename",
		Description: "Compute a rename WorkspaceEdit for a symbol via the language server (LSP). Returns the edit summary only — does not write files. Apply with edit/write/apply_patch.",
		Permission:  "edit",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"line": map[string]any{
					"type":        "integer",
					"description": "Line number (1-indexed)",
				},
				"column": map[string]any{
					"type":        "integer",
					"description": "Column number (1-indexed)",
				},
				"new_name": map[string]any{
					"type":        "string",
					"description": "New symbol name",
				},
			},
			"required": []string{"file_path", "line", "column", "new_name"},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			if mgr.Disabled() {
				return disabledResult(), nil
			}
			var input struct {
				FilePath string `json:"file_path"`
				Line     int    `json:"line"`
				Column   int    `json:"column"`
				NewName  string `json:"new_name"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}
			if err := validateLineColumn(input.Line, input.Column); err != nil {
				return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
			}
			if input.NewName == "" {
				return &tool.ExecuteResult{Output: "new_name is required", IsError: true}, nil
			}

			file := resolvePath(input.FilePath, tc.Directory)
			client, lang, err := mgr.ClientForFile(ctx, file)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
			}
			if client == nil {
				return &tool.ExecuteResult{Output: noServerMessage(lang, file)}, nil
			}

			edit, err := client.Rename(ctx, file, Position{Line: input.Line - 1, Character: input.Column - 1}, input.NewName)
			if err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("LSP rename failed: %v", err), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: formatWorkspaceEdit(edit)}, nil
		},
	}
}

func noServerMessage(lang, file string) string {
	if lang == "" {
		return fmt.Sprintf("No LSP support for file type: %s", filepath.Ext(file))
	}
	hint := installHints[lang]
	if hint != "" {
		return fmt.Sprintf("No LSP server available for %s files. %s", lang, hint)
	}
	return fmt.Sprintf("No LSP server available for %s files.", lang)
}

func resolvePath(path, dir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}
