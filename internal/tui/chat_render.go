package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/render"
)

// renderMessage renders a single message (user or assistant) as a string.
func renderMessage(msg MessageView, width int, md *render.MarkdownRenderer) string {
	var sb strings.Builder

	switch msg.Info.Role {
	case "user":
		sb.WriteString(styleUserMsg.Render("> "))
	case "assistant":
		sb.WriteString(styleAssistantMsg.Render("  "))
	default:
		sb.WriteString("  ")
	}

	parts := renderParts(msg.Parts, width-4, md)
	sb.WriteString(parts)

	return sb.String()
}

// renderParts renders all parts of a message.
func renderParts(parts []PartView, width int, md *render.MarkdownRenderer) string {
	if width < 10 {
		width = 10
	}

	var sb strings.Builder
	for i, part := range parts {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch part.Type {
		case "text":
			sb.WriteString(renderTextPart(part, width, md))
		case "tool-call":
			sb.WriteString(renderToolCallPart(part))
		case "tool-result":
			sb.WriteString(renderToolResultPart(part, width))
		case "reasoning":
			sb.WriteString(renderTextPart(part, width, md))
		default:
			sb.WriteString(renderTextPart(part, width, md))
		}
	}
	return sb.String()
}

// renderTextPart renders a text part through the markdown renderer.
// Streaming parts use RenderStreaming (renders complete blocks, leaves trailing
// text raw). Completed parts use RenderFinal for full glamour rendering.
func renderTextPart(part PartView, width int, md *render.MarkdownRenderer) string {
	if part.Text == "" {
		return ""
	}
	if part.Streaming {
		return md.RenderStreaming(part.Text)
	}
	return md.RenderFinal(part.Text)
}

// renderToolCallPart renders a tool call using the render package's inline
// format, appending a status indicator (done / spinning).
func renderToolCallPart(part PartView) string {
	inline := render.RenderToolInline(part.ToolName, part.ToolArgs, part.ToolError)
	status := toolStatus(part)
	return inline + " " + status
}

// renderToolResultPart renders a tool result with truncated output.
// Errors are shown as an inline error indicator. Successful results show
// up to 10 lines of output with a truncation indicator if needed.
// Collapsed results show a single-line placeholder.
func renderToolResultPart(part PartView, width int) string {
	if part.ToolError {
		return render.RenderToolInline(part.ToolName, "", true)
	}
	if part.Text == "" {
		return ""
	}
	if part.Collapsed {
		return styleMetadata.Render("  ... (collapsed)")
	}

	const maxLines = 10
	lines := strings.Split(part.Text, "\n")
	total := len(lines)
	truncated := total > maxLines
	if truncated {
		lines = lines[:maxLines]
	}

	result := wordwrap.String(strings.Join(lines, "\n"), width)
	if truncated {
		result += "\n" + styleMetadata.Render(fmt.Sprintf("  ... (%d more lines)", total-maxLines))
	}
	return result
}

// toolStatus returns a status indicator based on whether the part is complete.
func toolStatus(part PartView) string {
	if part.Time != nil {
		if end, ok := part.Time["end"].(float64); ok && end > 0 {
			return renderSuccess("done")
		}
	}
	return styleSpinner.Render("...")
}

// renderSuccess renders text in the success color.
func renderSuccess(s string) string {
	return lipgloss.NewStyle().Foreground(colorSuccess).Render(s)
}
