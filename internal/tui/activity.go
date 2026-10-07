package tui

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// formatActivity returns a short plain activity verb for the status bar.
func formatActivity(part PartView) string {
	switch part.Type {
	case "reasoning", "thinking":
		return "thinking"
	case "tool-call", "tool":
		return formatToolActivity(part.ToolName, part.ToolArgs)
	default:
		return ""
	}
}

func formatToolActivity(toolName, toolArgs string) string {
	const maxLen = 40
	name := strings.ToLower(toolName)

	var args map[string]any
	_ = json.Unmarshal([]byte(toolArgs), &args)

	switch name {
	case "bash", "shell":
		if args != nil {
			if cmd, ok := args["command"].(string); ok && cmd != "" {
				return truncateActivity("bash "+firstActivityLine(cmd), maxLen)
			}
		}
		return "bash"
	case "read", "write", "edit":
		if args != nil {
			if path, ok := args["file_path"].(string); ok && path != "" {
				return truncateActivity(name+" "+filepath.Base(path), maxLen)
			}
		}
		return name
	case "web_fetch", "webfetch":
		return "webfetch"
	default:
		if toolName == "" {
			return ""
		}
		return truncateActivity(toolName, maxLen)
	}
}

func firstActivityLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func truncateActivity(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
