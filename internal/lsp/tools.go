package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

// RegisterTools registers all LSP tools on the tool registry.
func RegisterTools(r *tool.Registry, mgr *Manager) {
	r.Register(diagnosticsTool(mgr))
	r.Register(hoverTool(mgr))
	r.Register(definitionTool(mgr))
	r.Register(referencesTool(mgr))
	r.Register(symbolsTool(mgr))
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
			var input struct {
				FilePath string `json:"file_path"`
				Line     int    `json:"line"`
				Column   int    `json:"column"`
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
			var input struct {
				FilePath string `json:"file_path"`
				Line     int    `json:"line"`
				Column   int    `json:"column"`
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
			var input struct {
				FilePath           string `json:"file_path"`
				Line               int    `json:"line"`
				Column             int    `json:"column"`
				IncludeDeclaration *bool  `json:"include_declaration,omitempty"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
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
			var input struct {
				Query    string `json:"query"`
				Language string `json:"language,omitempty"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return &tool.ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}

			lang := input.Language
			if lang == "" {
				langs := mgr.AvailableLanguages()
				if len(langs) > 0 {
					lang = langs[0]
				}
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
