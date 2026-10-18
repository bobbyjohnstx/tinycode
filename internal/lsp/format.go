package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

func formatDocumentSymbols(symbols []DocumentSymbolNode) string {
	if len(symbols) == 0 {
		return "No symbols found."
	}

	var sb strings.Builder
	for _, sym := range symbols {
		formatDocumentSymbolNode(&sb, sym, 0)
	}
	return sb.String()
}

func formatCodeActions(actions []CodeAction) string {
	if len(actions) == 0 {
		return "No code actions available."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d code actions:\n", len(actions)))
	for i, a := range actions {
		kind := ""
		if a.Kind != "" {
			kind = fmt.Sprintf(" [%s]", a.Kind)
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, a.Title, kind))
		if a.Edit != nil {
			sb.WriteString(formatWorkspaceEditSummary(a.Edit, "   "))
		}
	}
	return sb.String()
}

func formatWorkspaceEdit(edit *WorkspaceEdit) string {
	if edit == nil || workspaceEditEmpty(edit) {
		return "No edits."
	}
	var sb strings.Builder
	sb.WriteString("WorkspaceEdit (not applied — use edit/write/apply_patch):\n")
	sb.WriteString(formatWorkspaceEditSummary(edit, "  "))
	return sb.String()
}

func workspaceEditEmpty(edit *WorkspaceEdit) bool {
	if edit == nil {
		return true
	}
	if len(edit.Changes) > 0 {
		return false
	}
	return len(edit.DocumentChanges) == 0
}

func formatWorkspaceEditSummary(edit *WorkspaceEdit, indent string) string {
	if edit == nil {
		return ""
	}
	var sb strings.Builder
	uris := make([]string, 0, len(edit.Changes))
	for uri := range edit.Changes {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		edits := edit.Changes[uri]
		path := fileFromURI(uri)
		sb.WriteString(fmt.Sprintf("%s%s: %d edit(s)\n", indent, path, len(edits)))
		for _, e := range edits {
			preview := e.NewText
			if len(preview) > 40 {
				preview = preview[:40] + "…"
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
			sb.WriteString(fmt.Sprintf("%s  L%d:%d→L%d:%d %q\n",
				indent,
				e.Range.Start.Line+1, e.Range.Start.Character+1,
				e.Range.End.Line+1, e.Range.End.Character+1,
				preview,
			))
		}
	}
	for _, raw := range edit.DocumentChanges {
		var tde TextDocumentEdit
		if err := json.Unmarshal(raw, &tde); err != nil || tde.TextDocument.URI == "" {
			// Create/Rename/DeleteFile or unknown shape
			sb.WriteString(fmt.Sprintf("%sunsupported op (non-TextDocumentEdit in documentChanges)\n", indent))
			continue
		}
		path := fileFromURI(tde.TextDocument.URI)
		sb.WriteString(fmt.Sprintf("%s%s: %d edit(s)\n", indent, path, len(tde.Edits)))
		for _, e := range tde.Edits {
			preview := e.NewText
			if len(preview) > 40 {
				preview = preview[:40] + "…"
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
			sb.WriteString(fmt.Sprintf("%s  L%d:%d→L%d:%d %q\n",
				indent,
				e.Range.Start.Line+1, e.Range.Start.Character+1,
				e.Range.End.Line+1, e.Range.End.Character+1,
				preview,
			))
		}
	}
	return sb.String()
}

func formatDocumentSymbolNode(sb *strings.Builder, sym DocumentSymbolNode, depth int) {
	indent := strings.Repeat("  ", depth)
	kind := symbolKindStr(sym.Kind)
	line := sym.Line + 1
	if sym.Flat {
		container := ""
		if sym.ContainerName != "" {
			container = fmt.Sprintf(" in %s", sym.ContainerName)
		}
		sb.WriteString(fmt.Sprintf("%s%s %s%s — L%d\n", indent, kind, sym.Name, container, line))
		return
	}
	sb.WriteString(fmt.Sprintf("%s%s %s — L%d\n", indent, kind, sym.Name, line))
	for _, child := range sym.Children {
		formatDocumentSymbolNode(sb, child, depth+1)
	}
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
