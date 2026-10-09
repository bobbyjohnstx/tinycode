package lsp

import (
	"os"
	"strings"
	"testing"
)

func TestFormatDiagnostics(t *testing.T) {
	t.Run("no diagnostics", func(t *testing.T) {
		result := formatDiagnostics("/path/to/file.go", nil)
		if !strings.Contains(result, "No diagnostics") {
			t.Errorf("expected 'No diagnostics', got %q", result)
		}
	})

	t.Run("with diagnostics", func(t *testing.T) {
		diags := []Diagnostic{
			{
				Range:    Range{Start: Position{Line: 9, Character: 4}},
				Severity: 1,
				Message:  "undefined: foo",
			},
			{
				Range:    Range{Start: Position{Line: 14, Character: 0}},
				Severity: 2,
				Message:  "unused variable",
			},
		}
		result := formatDiagnostics("/path/to/file.go", diags)
		if !strings.Contains(result, "file.go:10:5: error: undefined: foo") {
			t.Errorf("expected error diagnostic, got %q", result)
		}
		if !strings.Contains(result, "file.go:15:1: warning: unused variable") {
			t.Errorf("expected warning diagnostic, got %q", result)
		}
	})
}

func TestFormatDefinitions(t *testing.T) {
	t.Run("no locations", func(t *testing.T) {
		result := formatDefinitions(nil)
		if !strings.Contains(result, "No definition") {
			t.Errorf("expected 'No definition', got %q", result)
		}
	})

	t.Run("with location", func(t *testing.T) {
		locs := []Location{
			{
				URI:   "file:///src/main.go",
				Range: Range{Start: Position{Line: 41, Character: 5}},
			},
		}
		result := formatDefinitions(locs)
		if !strings.Contains(result, "/src/main.go:42:6") {
			t.Errorf("expected location, got %q", result)
		}
	})
}

func TestFormatReferences(t *testing.T) {
	locs := []Location{
		{URI: "file:///a.go", Range: Range{Start: Position{Line: 0, Character: 0}}},
		{URI: "file:///b.go", Range: Range{Start: Position{Line: 9, Character: 3}}},
	}
	result := formatReferences(locs)
	if !strings.Contains(result, "2 references") {
		t.Errorf("expected '2 references', got %q", result)
	}
	if !strings.Contains(result, "/a.go:1:1") {
		t.Errorf("expected /a.go:1:1, got %q", result)
	}
	if !strings.Contains(result, "/b.go:10:4") {
		t.Errorf("expected /b.go:10:4, got %q", result)
	}
}

func TestSeverityStr(t *testing.T) {
	tests := []struct {
		severity int
		want     string
	}{
		{1, "error"},
		{2, "warning"},
		{3, "info"},
		{4, "hint"},
		{99, "unknown"},
	}
	for _, tt := range tests {
		got := severityStr(tt.severity)
		if got != tt.want {
			t.Errorf("severityStr(%d) = %q, want %q", tt.severity, got, tt.want)
		}
	}
}

func TestSymbolKindStr(t *testing.T) {
	if got := symbolKindStr(12); got != "Function" {
		t.Errorf("symbolKindStr(12) = %q, want Function", got)
	}
	if got := symbolKindStr(23); got != "Struct" {
		t.Errorf("symbolKindStr(23) = %q, want Struct", got)
	}
	if got := symbolKindStr(999); !strings.HasPrefix(got, "Kind(") {
		t.Errorf("symbolKindStr(999) = %q, want Kind(...)", got)
	}
}

func TestFormatHover_ReturnsPlaceholderWhenEmpty(t *testing.T) {
	result := formatHover("")
	if result != "No hover information available." {
		t.Errorf("formatHover(%q) = %q, want %q", "", result, "No hover information available.")
	}
}

func TestFormatHover_ReturnsContentWhenNonEmpty(t *testing.T) {
	content := "func Println(a ...any) (n int, err error)"
	result := formatHover(content)
	if result != content {
		t.Errorf("formatHover(%q) = %q, want content returned as-is", content, result)
	}
}

func TestFormatReferences_ReturnsPlaceholderWhenEmpty(t *testing.T) {
	result := formatReferences(nil)
	if result != "No references found." {
		t.Errorf("formatReferences(nil) = %q, want %q", result, "No references found.")
	}
}

func TestFormatSymbols_ReturnsPlaceholderWhenEmpty(t *testing.T) {
	result := formatSymbols(nil)
	if result != "No symbols found." {
		t.Errorf("formatSymbols(nil) = %q, want %q", result, "No symbols found.")
	}
}

func TestFormatSymbols_FormatsSymbolsWithKindAndContainer(t *testing.T) {
	symbols := []SymbolInfo{
		{
			Name: "Println",
			Kind: 12, // Function
			Location: Location{
				URI:   "file:///usr/local/go/src/fmt/print.go",
				Range: Range{Start: Position{Line: 273, Character: 5}},
			},
			ContainerName: "fmt",
		},
		{
			Name: "Writer",
			Kind: 11, // Interface
			Location: Location{
				URI:   "file:///usr/local/go/src/io/io.go",
				Range: Range{Start: Position{Line: 99, Character: 5}},
			},
		},
	}
	result := formatSymbols(symbols)
	if !strings.Contains(result, "2 symbols found") {
		t.Errorf("expected '2 symbols found', got %q", result)
	}
	if !strings.Contains(result, "Function Println in fmt") {
		t.Errorf("expected 'Function Println in fmt', got %q", result)
	}
	if !strings.Contains(result, "Interface Writer") {
		t.Errorf("expected 'Interface Writer', got %q", result)
	}
	// Verify line numbers are 1-indexed in output
	if !strings.Contains(result, "print.go:274") {
		t.Errorf("expected 1-indexed line 274, got %q", result)
	}
}

func TestReadLinePreview_ReadsCorrectLine(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.go"
	content := "package main\n\nfunc hello() {\n\treturn\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Line 2 (0-indexed) = "func hello() {"
	result := readLinePreview(path, 2)
	if result != "func hello() {" {
		t.Errorf("readLinePreview(path, 2) = %q, want %q", result, "func hello() {")
	}
}

func TestReadLinePreview_ReturnsEmptyForMissingFile(t *testing.T) {
	result := readLinePreview("/nonexistent/file.go", 0)
	if result != "" {
		t.Errorf("readLinePreview(missing, 0) = %q, want empty string", result)
	}
}

func TestReadLinePreview_ReturnsEmptyForLinePastEnd(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/short.go"
	if err := os.WriteFile(path, []byte("line one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := readLinePreview(path, 100)
	if result != "" {
		t.Errorf("readLinePreview(path, 100) = %q, want empty string", result)
	}
}
