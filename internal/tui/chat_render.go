package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/bobbyjohnstx/tinycode/internal/tui/render"
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
	styleReasoningText = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#999999"})
)

// subagentHit records a subagent header line's offset within rendered output.
type subagentHit struct {
	lineOffset int
	label      string
}

// renderOpts bundles rendering state and hit collectors.
type renderOpts struct {
	thoughtHits      *[]thoughtHit
	subagentHits     *[]subagentHit
	subagentExpanded map[string]bool
	subagentStatus   map[string]SubagentStatus
}

// subagentGroup holds consecutive parts belonging to the same subagent.
type subagentGroup struct {
	label string
	parts []PartView
}

// renderMessage renders a single message (user or assistant) as a string.
func renderMessage(msg MessageView, width int, md *render.MarkdownRenderer) string {
	return renderMessageWithOpts(msg, width, md, nil)
}

// renderMessageWithHits renders a message and optionally collects thought label positions.
func renderMessageWithHits(msg MessageView, width int, md *render.MarkdownRenderer, hits *[]thoughtHit) string {
	if hits == nil {
		return renderMessageWithOpts(msg, width, md, nil)
	}
	opts := &renderOpts{thoughtHits: hits}
	return renderMessageWithOpts(msg, width, md, opts)
}

// renderMessageWithOpts renders a message with full render options.
func renderMessageWithOpts(msg MessageView, width int, md *render.MarkdownRenderer, opts *renderOpts) string {
	switch msg.Info.Role {
	case "user":
		return renderUserMessage(msg, width)
	case "assistant":
		return renderAssistantMessage(msg, width, md, opts)
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

func renderAssistantMessage(msg MessageView, width int, md *render.MarkdownRenderer, opts *renderOpts) string {
	var sb strings.Builder
	lineNum := 0

	agentThoughtStyle := styleReasoningLabel
	agent := msg.Info.Agent
	if agent == "" {
		agent = "build"
	}
	agentThoughtStyle = lipgloss.NewStyle().Foreground(AgentColor(agent))

	groups := groupSubagentParts(msg.Parts)

	for _, g := range groups {
		if g.label != "" {
			// Render subagent group
			expanded := false
			var status SubagentStatus
			if opts != nil {
				if opts.subagentExpanded != nil {
					expanded = opts.subagentExpanded[g.label]
				}
				if opts.subagentStatus != nil {
					status = opts.subagentStatus[g.label]
				}
				if opts.subagentHits != nil {
					*opts.subagentHits = append(*opts.subagentHits, subagentHit{lineOffset: lineNum, label: g.label})
				}
			}
			rendered := renderSubagentGroup(g, expanded, status, width)
			sb.WriteString(rendered)
			sb.WriteString("\n")
			lineNum += strings.Count(rendered, "\n") + 1
			continue
		}
		// Render individual parts (no subagent label)
		for _, part := range g.parts {
			switch part.Type {
			case "text":
				rendered := renderTextPart(part, width-4, md)
				if rendered != "" {
					sb.WriteString(rendered)
					sb.WriteString("\n")
					lineNum += strings.Count(rendered, "\n") + 1
				}
			case "reasoning":
				if opts != nil && opts.thoughtHits != nil && part.ID != "" {
					*opts.thoughtHits = append(*opts.thoughtHits, thoughtHit{lineOffset: lineNum, partID: part.ID})
				}
				prefix := "+"
				if part.ThoughtExpanded {
					prefix = "-"
				}
				label := agentThoughtStyle.Render(prefix + " Thought")
				if dur := partDuration(part); dur != "" {
					label += agentThoughtStyle.Render(": " + dur)
				}
				sb.WriteString(label)
				sb.WriteString("\n")
				lineNum++
				if !part.ThoughtExpanded || part.Text == "" {
					break
				}
				wrapped := wordwrap.String(part.Text, width-6)
				expandedLines := strings.Split(wrapped, "\n")
				for _, line := range expandedLines {
					sb.WriteString(styleReasoningText.Render("  " + line))
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
				lineNum += len(expandedLines) + 1
			case "tool-call":
				tc := renderToolCallPart(part)
				sb.WriteString(tc)
				sb.WriteString("\n")
				lineNum += strings.Count(tc, "\n") + 1
			case "tool-result":
				result := renderToolResultPart(part, width-4)
				if result != "" {
					sb.WriteString(result)
					sb.WriteString("\n")
					lineNum += strings.Count(result, "\n") + 1
				}
			case "tool":
				tc := renderToolCallPart(part)
				sb.WriteString(tc)
				sb.WriteString("\n")
				lineNum += strings.Count(tc, "\n") + 1
				if part.Text != "" && !part.ToolError {
					result := renderToolResultPart(part, width-4)
					if result != "" {
						sb.WriteString(result)
						sb.WriteString("\n")
						lineNum += strings.Count(result, "\n") + 1
					}
				}
			default:
				rendered := renderTextPart(part, width-4, md)
				if rendered != "" {
					sb.WriteString(rendered)
					sb.WriteString("\n")
					lineNum += strings.Count(rendered, "\n") + 1
				}
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
	agent := msg.Info.Agent
	if agent == "" {
		agent = "build"
	}
	agentLabel := cases.Title(language.English).String(agent)
	agentStyle := lipgloss.NewStyle().Foreground(AgentColor(agent))

	var parts []string
	parts = append(parts, agentLabel)
	if msg.Info.ModelID != "" {
		parts = append(parts, msg.Info.ModelID)
	}
	label := strings.Join(parts, " · ")
	return agentStyle.Render("■") + " " + styleAgentFooter.Render(label)
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
		case "tool":
			sb.WriteString(renderToolCallPart(part))
			if part.Text != "" && !part.ToolError {
				sb.WriteString("\n")
				sb.WriteString(renderToolResultPart(part, width))
			}
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
// Read tool results are collapsed by default to avoid walls of file content.
func renderToolResultPart(part PartView, width int) string {
	if part.ToolError {
		return render.RenderToolInline(part.ToolName, "", true)
	}
	if part.Text == "" {
		return ""
	}
	if part.Collapsed || part.ToolName == "read" {
		return ""
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

// --- Subagent group rendering ---

// groupSubagentParts groups consecutive parts with the same non-empty SubagentLabel.
// Parts without a label become single-element groups with label "".
func groupSubagentParts(parts []PartView) []subagentGroup {
	var groups []subagentGroup
	for _, p := range parts {
		if p.SubagentLabel == "" {
			groups = append(groups, subagentGroup{label: "", parts: []PartView{p}})
		} else if len(groups) > 0 && groups[len(groups)-1].label == p.SubagentLabel {
			groups[len(groups)-1].parts = append(groups[len(groups)-1].parts, p)
		} else {
			groups = append(groups, subagentGroup{label: p.SubagentLabel, parts: []PartView{p}})
		}
	}
	return groups
}

// renderSubagentGroup renders a collapsible subagent group.
func renderSubagentGroup(g subagentGroup, expanded bool, status SubagentStatus, width int) string {
	agentType := extractAgentType(g.label)
	agentStyle := lipgloss.NewStyle().Foreground(AgentColor(agentType))

	desc := extractTaskDescription(g.parts)

	prefix := "+"
	if expanded {
		prefix = "-"
	}

	header := agentStyle.Render(prefix + " " + g.label)
	if desc != "" {
		header += "  " + styleMetadata.Render(truncateStr(desc, 50))
	}

	// Build status indicator
	var statusParts []string
	allDone := groupAllDone(g.parts)
	if allDone || status.Done {
		statusParts = append(statusParts, renderSuccess("done"))
		if dur := groupDuration(g.parts); dur != "" {
			statusParts = append(statusParts, styleMetadata.Render(dur))
		}
	} else {
		statusParts = append(statusParts, styleSpinner.Render("..."))
	}
	if status.Done && (status.InputTokens > 0 || status.OutputTokens > 0) {
		tokStr := formatTokens(status.InputTokens + status.OutputTokens)
		statusParts = append(statusParts, styleMetadata.Render("["+tokStr+" tok]"))
	}

	var sb strings.Builder
	sb.WriteString(header)
	if len(statusParts) > 0 {
		sb.WriteString("  ")
		sb.WriteString(strings.Join(statusParts, " "))
	}

	if !expanded {
		return sb.String()
	}

	// Expanded: render child tool calls with colored left border
	borderStr := agentStyle.Render("  ┃")
	for _, part := range g.parts {
		if part.Type == "tool" || part.Type == "tool-call" {
			if strings.EqualFold(part.ToolName, "task") {
				continue
			}
			inline := render.RenderToolInline(part.ToolName, part.ToolArgs, part.ToolError)
			st := toolStatus(part)
			sb.WriteString("\n")
			sb.WriteString(borderStr + " " + inline + " " + st)
		} else if part.Type == "text" && part.Text != "" {
			sb.WriteString("\n")
			sb.WriteString(borderStr + " " + styleMetadata.Render("Result: "+truncateStr(part.Text, width-10)))
		}
	}

	return sb.String()
}

// extractAgentType extracts the agent type from a label like "executor-1" → "executor".
func extractAgentType(label string) string {
	if idx := strings.LastIndex(label, "-"); idx > 0 {
		return label[:idx]
	}
	return label
}

// extractTaskDescription extracts the description from a task tool call's args.
func extractTaskDescription(parts []PartView) string {
	for _, p := range parts {
		if strings.EqualFold(p.ToolName, "task") && p.ToolArgs != "" {
			var args map[string]any
			if err := json.Unmarshal([]byte(p.ToolArgs), &args); err == nil {
				if desc, ok := args["description"].(string); ok {
					return desc
				}
			}
		}
	}
	return ""
}

// groupAllDone returns true if all tool parts in the group have end times.
func groupAllDone(parts []PartView) bool {
	hasTool := false
	for _, p := range parts {
		if p.Type != "tool" && p.Type != "tool-call" {
			continue
		}
		hasTool = true
		if p.Time == nil {
			return false
		}
		if end, ok := p.Time["end"].(float64); !ok || end <= 0 {
			return false
		}
	}
	return hasTool
}

// groupDuration returns the total duration from earliest start to latest end.
func groupDuration(parts []PartView) string {
	var minStart, maxEnd float64
	for _, p := range parts {
		if p.Time == nil {
			continue
		}
		if start, ok := p.Time["start"].(float64); ok {
			if minStart == 0 || start < minStart {
				minStart = start
			}
		}
		if end, ok := p.Time["end"].(float64); ok {
			if end > maxEnd {
				maxEnd = end
			}
		}
	}
	if minStart == 0 || maxEnd == 0 || maxEnd <= minStart {
		return ""
	}
	d := time.Duration(maxEnd-minStart) * time.Millisecond
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// truncateStr shortens s to maxLen, appending an ellipsis if needed.
func truncateStr(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	// Replace newlines with spaces for inline display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
