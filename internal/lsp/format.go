package lsp

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func formatDiagnostics(file string, diags []Diagnostic) string {
	if len(diags) == 0 {
		return fmt.Sprintf("No diagnostics for %s", file)
	}

	var sb strings.Builder
	for _, d := range diags {
		sb.WriteString(fmt.Sprintf("%s:%d:%d: %s: %s\n",
			file,
			d.Range.Start.Line+1,
			d.Range.Start.Character+1,
			severityStr(d.Severity),
			d.Message,
		))
	}
	return sb.String()
}

func formatDefinitions(locs []Location) string {
	if len(locs) == 0 {
		return "No definition found."
	}

	var sb strings.Builder
	for _, loc := range locs {
		file := fileFromURI(loc.URI)
		line := loc.Range.Start.Line + 1
		col := loc.Range.Start.Character + 1
		sb.WriteString(fmt.Sprintf("Defined at: %s:%d:%d\n", file, line, col))

		if preview := readLinePreview(file, loc.Range.Start.Line); preview != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", preview))
		}
	}
	return sb.String()
}

func formatReferences(locs []Location) string {
	if len(locs) == 0 {
		return "No references found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d references found:\n", len(locs)))
	for _, loc := range locs {
		file := fileFromURI(loc.URI)
		line := loc.Range.Start.Line + 1
		col := loc.Range.Start.Character + 1
		sb.WriteString(fmt.Sprintf("  %s:%d:%d\n", file, line, col))
	}
	return sb.String()
}

func formatHover(content string) string {
	if content == "" {
		return "No hover information available."
	}
	return content
}

func formatSymbols(symbols []SymbolInfo) string {
	if len(symbols) == 0 {
		return "No symbols found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d symbols found:\n", len(symbols)))
	for _, sym := range symbols {
		file := fileFromURI(sym.Location.URI)
		line := sym.Location.Range.Start.Line + 1
		kind := symbolKindStr(sym.Kind)
		container := ""
		if sym.ContainerName != "" {
			container = fmt.Sprintf(" in %s", sym.ContainerName)
		}
		sb.WriteString(fmt.Sprintf("  %s %s%s — %s:%d\n", kind, sym.Name, container, file, line))
	}
	return sb.String()
}

func severityStr(severity int) string {
	switch severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "unknown"
	}
}

var symbolKindNames = map[int]string{
	1: "File", 2: "Module", 3: "Namespace", 4: "Package",
	5: "Class", 6: "Method", 7: "Property", 8: "Field",
	9: "Constructor", 10: "Enum", 11: "Interface", 12: "Function",
	13: "Variable", 14: "Constant", 15: "String", 16: "Number",
	17: "Boolean", 18: "Array", 19: "Object", 20: "Key",
	21: "Null", 22: "EnumMember", 23: "Struct", 24: "Event",
	25: "Operator", 26: "TypeParameter",
}

func symbolKindStr(kind int) string {
	if s, ok := symbolKindNames[kind]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", kind)
}

func readLinePreview(file string, line int) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	current := 0
	for scanner.Scan() {
		if current == line {
			return strings.TrimSpace(scanner.Text())
		}
		current++
	}
	return ""
}
