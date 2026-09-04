package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// renderMessage renders a single message (user or assistant) as a string.
func renderMessage(msg MessageView, width int) string {
	var sb strings.Builder

	switch msg.Info.Role {
	case "user":
		sb.WriteString(styleUserMsg.Render("> "))
	case "assistant":
		sb.WriteString(styleAssistantMsg.Render("  "))
	default:
		sb.WriteString("  ")
	}

	parts := renderParts(msg.Parts, width-4)
	sb.WriteString(parts)

	return sb.String()
}

// renderParts renders all parts of a message.
func renderParts(parts []PartView, width int) string {
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
			sb.WriteString(renderTextPart(part, width))
		case "tool-call":
			sb.WriteString(renderToolPart(part))
		case "tool-result":
			sb.WriteString(renderToolResultPart(part))
		case "reasoning":
			sb.WriteString(renderTextPart(part, width))
		default:
			sb.WriteString(renderTextPart(part, width))
		}
	}
	return sb.String()
}

// renderTextPart renders a text content part with word wrapping.
func renderTextPart(part PartView, width int) string {
	if part.Text == "" {
		return ""
	}
	return wordwrap.String(part.Text, width)
}

// renderToolPart renders a tool call with icon, name, and status indicator.
func renderToolPart(part PartView) string {
	icon := toolIcon(part.ToolName)
	status := toolStatus(part)
	name := styleToolName.Render(part.ToolName)
	return fmt.Sprintf("  %s %s %s", icon, name, status)
}

// renderToolResultPart renders a tool result.
func renderToolResultPart(part PartView) string {
	if part.ToolError {
		return fmt.Sprintf("  %s %s",
			styleToolName.Render(part.ToolName),
			renderError("error"),
		)
	}
	return ""
}

// toolIcon returns a unicode icon for the given tool name.
func toolIcon(name string) string {
	switch name {
	case "read", "Read":
		return "▸" // small right triangle
	case "write", "Write":
		return "←" // left arrow
	case "edit", "Edit":
		return "←" // left arrow
	case "shell", "Shell", "bash", "Bash":
		return "$"
	case "glob", "Glob":
		return "≡" // triple bar
	case "grep", "Grep":
		return "⌕" // search
	case "web_fetch", "WebFetch":
		return "☁" // cloud
	case "task", "Task":
		return "■" // square
	default:
		return "•" // bullet
	}
}

// toolStatus returns a status indicator based on whether the part is complete.
func toolStatus(part PartView) string {
	if part.Time != nil && part.Time.End > 0 {
		return renderSuccess("✓")
	}
	return styleSpinner.Render("○") // circle
}

// renderError renders text in the error color.
func renderError(s string) string {
	return lipgloss.NewStyle().Foreground(colorError).Render(s)
}

// renderSuccess renders text in the success color.
func renderSuccess(s string) string {
	return lipgloss.NewStyle().Foreground(colorSuccess).Render(s)
}
