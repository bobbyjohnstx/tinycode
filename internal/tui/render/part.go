package render

import (
	"github.com/muesli/reflow/wordwrap"
)

// Part holds the fields needed to render a message part.
// This mirrors tui.PartView but avoids a circular import.
type Part struct {
	Type      string
	Text      string
	ToolName  string
	ToolArgs  string
	ToolError bool
}

// RenderPart dispatches rendering by part type.
// For text and reasoning, it word-wraps. For tool calls, it renders inline.
// For tool results with errors, it shows the error indicator.
func RenderPart(part Part, width int) string {
	if width < 10 {
		width = 10
	}

	switch part.Type {
	case "text":
		return renderText(part.Text, width)
	case "tool-call":
		return RenderToolInline(part.ToolName, part.ToolArgs, part.ToolError)
	case "tool-result":
		return renderToolResult(part)
	case "reasoning":
		return renderText(part.Text, width)
	default:
		return renderText(part.Text, width)
	}
}

// renderText word-wraps text content to the given width.
func renderText(text string, width int) string {
	if text == "" {
		return ""
	}
	return wordwrap.String(text, width)
}

// renderToolResult renders a tool result part.
func renderToolResult(part Part) string {
	if part.ToolError {
		return RenderToolInline(part.ToolName, "", true)
	}
	return ""
}
