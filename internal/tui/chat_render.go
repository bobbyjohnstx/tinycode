package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/render"
)

var (
	styleUserBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.ThickBorder()).
		BorderLeft(true).BorderTop(false).BorderRight(false).BorderBottom(false).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#CC4444"}).
		PaddingLeft(1)
	styleTimestamp = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#666666"})
	styleAgentFooter = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	styleReasoningLabel = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"})
)

// renderMessage renders a single message (user or assistant) as a string.
func renderMessage(msg MessageView, width int, md *render.MarkdownRenderer) string {
	return renderMessageWithHits(msg, width, md, nil)
}

// renderMessageWithHits renders a message and optionally collects thought label positions.
func renderMessageWithHits(msg MessageView, width int, md *render.MarkdownRenderer, hits *[]thoughtHit) string {
	switch msg.Info.Role {
	case "user":
		return renderUserMessage(msg, width)
	case "assistant":
		return renderAssistantMessage(msg, width, md, hits)
	default:
		return renderParts(msg.Parts, width-4, md)
	}
}

func renderUserMessage(msg MessageView, width int) string {
	var lines []string
	for _, part := range msg.Parts {
		if part.Type == "text" && part.Text != "" {
			lines = append(lines, lipgloss.NewStyle().Bold(true).Render(part.Text))
		}
	}
	ts := formatTimestamp(msg.Info.CreatedAt)
	if ts != "" {
		lines = append(lines, styleTimestamp.Render(ts))
	}
	inner := strings.Join(lines, "\n")
	return styleUserBorder.Width(width - 4).Render(inner)
}

func renderAssistantMessage(msg MessageView, width int, md *render.MarkdownRenderer, hits *[]thoughtHit) string {
	var sb strings.Builder
	lineNum := 0

	for _, part := range msg.Parts {
		switch part.Type {
		case "text":
			rendered := renderTextPart(part, width-4, md)
			if rendered != "" {
				sb.WriteString(rendered)
				sb.WriteString("\n")
				lineNum += strings.Count(rendered, "\n") + 1
			}
		case "reasoning":
			if hits != nil && part.ID != "" {
				*hits = append(*hits, thoughtHit{lineOffset: lineNum, partID: part.ID})
			}
			prefix := "+"
			if part.ThoughtExpanded {
				prefix = "-"
			}
			label := styleReasoningLabel.Render(prefix + " Thought")
			if part.Time != nil {
				if dur := partDuration(part); dur != "" {
					label += styleReasoningLabel.Render(": " + dur)
				}
			}
			sb.WriteString(label)
			sb.WriteString("\n")
			lineNum++
			if part.ThoughtExpanded && part.Text != "" {
				wrapped := wordwrap.String(part.Text, width-6)
				for _, line := range strings.Split(wrapped, "\n") {
					sb.WriteString(styleReasoningLabel.Render("  " + line))
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
		case "tool-call":
			sb.WriteString(renderToolCallPart(part))
			sb.WriteString("\n")
		case "tool-result":
			result := renderToolResultPart(part, width-4)
			if result != "" {
				sb.WriteString(result)
				sb.WriteString("\n")
			}
		default:
			rendered := renderTextPart(part, width-4, md)
			if rendered != "" {
				sb.WriteString(rendered)
				sb.WriteString("\n")
			}
		}
	}

	// Agent/model footer
	footer := renderAgentFooter(msg)
	if footer != "" {
		sb.WriteString(footer)
	}

	return sb.String()
}

func renderAgentFooter(msg MessageView) string {
	var parts []string
	agent := msg.Info.Agent
	if agent == "" {
		agent = "Build"
	} else {
		agent = cases.Title(language.English).String(agent)
	}
	parts = append(parts, agent)
	if msg.Info.ModelID != "" {
		parts = append(parts, msg.Info.ModelID)
	}
	label := strings.Join(parts, " · ")
	return styleAgentFooter.Render("■ " + label)
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
		default:
			sb.WriteString(renderTextPart(part, width, md))
		}
	}
	return sb.String()
}

// renderTextPart renders a text part through the markdown renderer.
func renderTextPart(part PartView, width int, md *render.MarkdownRenderer) string {
	if part.Text == "" {
		return ""
	}
	if part.Streaming {
		return md.RenderStreaming(part.Text)
	}
	return md.RenderFinal(part.Text)
}

// renderToolCallPart renders a tool call with a status indicator.
func renderToolCallPart(part PartView) string {
	inline := render.RenderToolInline(part.ToolName, part.ToolArgs, part.ToolError)
	status := toolStatus(part)
	return inline + " " + status
}

// renderToolResultPart renders a tool result with truncated output.
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

func renderSuccess(s string) string {
	return lipgloss.NewStyle().Foreground(colorSuccess).Render(s)
}

func formatTimestamp(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return ""
		}
	}
	return t.Local().Format("3:04 PM")
}

func partDuration(part PartView) string {
	if part.Time == nil {
		return ""
	}
	start, ok1 := part.Time["start"].(float64)
	end, ok2 := part.Time["end"].(float64)
	if !ok1 || !ok2 || end <= start {
		return ""
	}
	d := time.Duration(end-start) * time.Millisecond
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
