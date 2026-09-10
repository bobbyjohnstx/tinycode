package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ExportSessionMsg is emitted when a session export completes.
type ExportSessionMsg struct {
	Path string
	Err  error
}

// exportSession writes the active session transcript to a Markdown file in cwd.
func exportSession(messages []MessageView, session SessionInfo, cwd string) tea.Cmd {
	return func() tea.Msg {
		if len(messages) == 0 {
			return ExportSessionMsg{Err: fmt.Errorf("no messages to export")}
		}

		transcript := formatTranscript(session, messages)

		slug := sanitizeFilename(session.Title)
		if slug == "" {
			slug = session.ID
			if len(slug) > 8 {
				slug = slug[:8]
			}
		}
		filename := fmt.Sprintf("session-%s.md", slug)

		dir := cwd
		if dir == "" {
			dir, _ = os.Getwd()
		}
		path := filepath.Join(dir, filename)

		if err := os.WriteFile(path, []byte(transcript), 0644); err != nil {
			return ExportSessionMsg{Err: fmt.Errorf("write failed: %w", err)}
		}

		return ExportSessionMsg{Path: path}
	}
}

func formatTranscript(session SessionInfo, messages []MessageView) string {
	var sb strings.Builder

	title := session.Title
	if title == "" {
		title = "Untitled Session"
	}
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("**Session ID:** %s\n", session.ID))
	if session.CreatedAt > 0 {
		sb.WriteString(fmt.Sprintf("**Created:** %s\n", time.UnixMilli(session.CreatedAt).Format(time.RFC1123)))
	}
	if session.UpdatedAt > 0 {
		sb.WriteString(fmt.Sprintf("**Updated:** %s\n", time.UnixMilli(session.UpdatedAt).Format(time.RFC1123)))
	}
	sb.WriteString("\n---\n\n")

	for _, msg := range messages {
		sb.WriteString(formatMessageExport(msg))
		sb.WriteString("---\n\n")
	}

	return sb.String()
}

func formatMessageExport(msg MessageView) string {
	var sb strings.Builder

	switch msg.Info.Role {
	case "user":
		sb.WriteString("## User\n\n")
	case "assistant":
		sb.WriteString(formatAssistantHeader(msg))
	default:
		sb.WriteString(fmt.Sprintf("## %s\n\n", msg.Info.Role))
	}

	for _, part := range msg.Parts {
		sb.WriteString(formatPartExport(part))
	}

	return sb.String()
}

func formatAssistantHeader(msg MessageView) string {
	agent := msg.Info.Agent
	if agent == "" {
		agent = "Build"
	} else {
		agent = strings.ToUpper(agent[:1]) + agent[1:]
	}

	parts := []string{agent}
	if msg.Info.ModelID != "" {
		parts = append(parts, msg.Info.ModelID)
	}

	return fmt.Sprintf("## Assistant (%s)\n\n", strings.Join(parts, " · "))
}

func formatPartExport(part PartView) string {
	switch part.Type {
	case "text":
		if part.Text == "" {
			return ""
		}
		return part.Text + "\n\n"

	case "reasoning":
		if part.Text == "" {
			return ""
		}
		return fmt.Sprintf("_Thinking:_\n\n%s\n\n", part.Text)

	case "tool-call":
		name := part.ToolName
		if name == "" {
			name = "unknown"
		}
		result := fmt.Sprintf("**Tool: %s**\n", name)
		if part.ToolArgs != "" {
			result += fmt.Sprintf("\n**Input:**\n```json\n%s\n```\n", part.ToolArgs)
		}
		result += "\n"
		return result

	case "tool-result":
		if part.Text == "" {
			return ""
		}
		result := "**Output:**\n```\n"
		lines := strings.Split(part.Text, "\n")
		if len(lines) > 50 {
			result += strings.Join(lines[:50], "\n")
			result += fmt.Sprintf("\n... (%d more lines)", len(lines)-50)
		} else {
			result += part.Text
		}
		result += "\n```\n\n"
		return result
	}

	return ""
}

func sanitizeFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
