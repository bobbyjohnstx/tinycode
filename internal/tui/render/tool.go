package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ToolIcon returns a unicode icon for the given tool name.
func ToolIcon(name string) string {
	switch strings.ToLower(name) {
	case "read":
		return "▸" // small right triangle
	case "write":
		return "←" // left arrow
	case "edit":
		return "←" // left arrow
	case "shell", "bash":
		return "$"
	case "glob":
		return "≡" // triple bar
	case "grep":
		return "⌕" // search
	case "web_fetch", "webfetch":
		return "☁" // cloud
	case "task":
		return "■" // square
	default:
		return "•" // bullet
	}
}

var (
	styleToolName  = lipgloss.NewStyle().Bold(true)
	styleToolDim   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	styleToolError = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"})
	styleToolBox   = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#555555"}).
			Padding(0, 1)
)

// RenderToolInline renders a tool call as a single-line summary.
// Format: "  <icon> <name> <detail>"
func RenderToolInline(toolName, toolArgs string, isError bool) string {
	icon := ToolIcon(toolName)
	name := styleToolName.Render(toolName)

	detail := inlineDetail(toolName, toolArgs)
	if isError {
		detail = styleToolError.Render("error")
	}

	if detail != "" {
		return fmt.Sprintf("  %s %s %s", icon, name, detail)
	}
	return fmt.Sprintf("  %s %s", icon, name)
}

// RenderToolBlock renders a tool call as a bordered box with detail content.
func RenderToolBlock(toolName, toolArgs string, width int, isError bool) string {
	if width < 20 {
		width = 20
	}

	icon := ToolIcon(toolName)
	name := styleToolName.Render(toolName)
	header := fmt.Sprintf("%s %s", icon, name)

	body := blockDetail(toolName, toolArgs, width-4)
	if isError {
		body = styleToolError.Render("error")
	}

	if body == "" {
		return fmt.Sprintf("  %s", header)
	}

	content := fmt.Sprintf("%s\n%s", header, body)
	box := styleToolBox.Width(width - 4).Render(content)
	return fmt.Sprintf("  %s", box)
}

// inlineDetail extracts a short summary from tool args for inline display.
func inlineDetail(toolName, toolArgs string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(toolArgs), &args); err != nil {
		return styleToolDim.Render(truncate(toolArgs, 60))
	}

	switch strings.ToLower(toolName) {
	case "bash", "shell":
		if cmd, ok := args["command"].(string); ok {
			line := firstLine(cmd)
			return styleToolDim.Render("$ " + truncate(line, 58))
		}
	case "read":
		if path, ok := args["file_path"].(string); ok {
			return styleToolDim.Render(truncate(path, 60))
		}
	case "write":
		if path, ok := args["file_path"].(string); ok {
			return styleToolDim.Render(truncate(path, 60))
		}
	case "edit":
		if path, ok := args["file_path"].(string); ok {
			return styleToolDim.Render(truncate(path, 60))
		}
	case "glob":
		if pattern, ok := args["pattern"].(string); ok {
			return styleToolDim.Render(truncate(pattern, 60))
		}
	case "grep":
		if pattern, ok := args["pattern"].(string); ok {
			return styleToolDim.Render(truncate(pattern, 60))
		}
	case "web_fetch", "webfetch":
		if url, ok := args["url"].(string); ok {
			return styleToolDim.Render(truncate(url, 60))
		}
	case "task":
		if desc, ok := args["description"].(string); ok {
			return styleToolDim.Render(truncate(desc, 60))
		}
	}
	return ""
}

// blockDetail formats the full args content for a bordered box.
func blockDetail(toolName, toolArgs string, width int) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(toolArgs), &args); err != nil {
		if toolArgs == "" {
			return ""
		}
		return truncate(toolArgs, width)
	}

	switch strings.ToLower(toolName) {
	case "bash", "shell":
		if cmd, ok := args["command"].(string); ok {
			return styleToolDim.Render("$ " + cmd)
		}
	case "read":
		if path, ok := args["file_path"].(string); ok {
			return styleToolDim.Render(path)
		}
	case "write":
		if path, ok := args["file_path"].(string); ok {
			return styleToolDim.Render(path)
		}
	case "edit":
		if path, ok := args["file_path"].(string); ok {
			return styleToolDim.Render(path)
		}
	case "glob":
		if pattern, ok := args["pattern"].(string); ok {
			return styleToolDim.Render(pattern)
		}
	case "grep":
		if pattern, ok := args["pattern"].(string); ok {
			if path, ok := args["path"].(string); ok {
				return styleToolDim.Render(fmt.Sprintf("%s in %s", pattern, path))
			}
			return styleToolDim.Render(pattern)
		}
	case "web_fetch", "webfetch":
		if url, ok := args["url"].(string); ok {
			return styleToolDim.Render(url)
		}
	case "task":
		if desc, ok := args["description"].(string); ok {
			return styleToolDim.Render(desc)
		}
	}
	return ""
}

// SetToolNameStyle updates the tool name style (called by the theme system).
func SetToolNameStyle(s lipgloss.Style) {
	styleToolName = s
}

// truncate shortens s to maxLen, appending an ellipsis if needed.
func truncate(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// firstLine returns the first line of a multi-line string.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
