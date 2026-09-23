package tui

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yuin/goldmark"
)

// exportSessionHTML writes the active session transcript to a self-contained HTML file.
func exportSessionHTML(messages []MessageView, session SessionInfo, cwd string) tea.Cmd {
	return func() tea.Msg {
		if len(messages) == 0 {
			return ExportSessionMsg{Err: fmt.Errorf("no messages to export")}
		}

		htmlContent, err := renderSessionHTML(session, messages)
		if err != nil {
			return ExportSessionMsg{Err: fmt.Errorf("render failed: %w", err)}
		}

		slug := sanitizeFilename(session.Title)
		if slug == "" {
			slug = session.ID
			if len(slug) > 8 {
				slug = slug[:8]
			}
		}
		filename := fmt.Sprintf("session-%s.html", slug)

		dir := cwd
		if dir == "" {
			dir, _ = os.Getwd()
		}
		path := filepath.Join(dir, filename)

		if err := os.WriteFile(path, []byte(htmlContent), 0644); err != nil {
			return ExportSessionMsg{Err: fmt.Errorf("write failed: %w", err)}
		}

		return ExportSessionMsg{Path: path}
	}
}

type htmlTemplateData struct {
	Title    string
	ID       string
	Created  string
	Updated  string
	Messages []htmlMessageData
}

type htmlMessageData struct {
	Role    string
	Label   string
	Content template.HTML
}

func renderSessionHTML(session SessionInfo, messages []MessageView) (string, error) {
	md := goldmark.New()

	data := htmlTemplateData{
		Title: session.Title,
		ID:    session.ID,
	}
	if data.Title == "" {
		data.Title = "Untitled Session"
	}
	if session.CreatedAt > 0 {
		data.Created = time.UnixMilli(session.CreatedAt).Format(time.RFC1123)
	}
	if session.UpdatedAt > 0 {
		data.Updated = time.UnixMilli(session.UpdatedAt).Format(time.RFC1123)
	}

	for _, msg := range messages {
		rendered := renderMessageHTML(msg, md)
		data.Messages = append(data.Messages, rendered)
	}

	tmpl, err := template.New("session").Parse(sessionHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

func renderMessageHTML(msg MessageView, md goldmark.Markdown) htmlMessageData {
	role := msg.Info.Role

	label := role
	if role == "assistant" {
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
		label = "Assistant (" + strings.Join(parts, " · ") + ")"
	} else if role == "user" {
		label = "User"
	}

	var sb strings.Builder
	for _, part := range msg.Parts {
		sb.WriteString(renderPartHTML(part, md))
	}

	return htmlMessageData{
		Role:    role,
		Label:   label,
		Content: template.HTML(sb.String()),
	}
}

func renderPartHTML(part PartView, md goldmark.Markdown) string {
	switch part.Type {
	case "text":
		if part.Text == "" {
			return ""
		}
		var buf bytes.Buffer
		if err := md.Convert([]byte(part.Text), &buf); err != nil {
			return "<pre>" + html.EscapeString(part.Text) + "</pre>"
		}
		return buf.String()

	case "reasoning":
		if part.Text == "" {
			return ""
		}
		return `<details class="thinking"><summary>Thinking</summary><div class="thinking-content">` +
			"<pre>" + html.EscapeString(part.Text) + "</pre></div></details>"

	case "tool-call":
		name := part.ToolName
		if name == "" {
			name = "unknown"
		}
		result := `<details class="tool-call"><summary>` + html.EscapeString(name) + `</summary>`
		if part.ToolArgs != "" {
			result += `<div class="tool-args"><pre><code>` + html.EscapeString(part.ToolArgs) + `</code></pre></div>`
		}
		result += `</details>`
		return result

	case "tool-result":
		if part.Text == "" {
			return ""
		}
		text := part.Text
		lines := strings.Split(text, "\n")
		if len(lines) > 50 {
			text = strings.Join(lines[:50], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-50)
		}
		return `<details class="tool-result"><summary>Output</summary>` +
			`<pre class="tool-output">` + html.EscapeString(text) + `</pre></details>`
	}
	return ""
}

const sessionHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0d1117;color:#c9d1d9;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;line-height:1.6;padding:2rem;max-width:900px;margin:0 auto}
a{color:#58a6ff}
header{border-bottom:1px solid #21262d;padding-bottom:1rem;margin-bottom:2rem}
header h1{font-size:1.5rem;font-weight:600;color:#f0f6fc;margin-bottom:.5rem}
header .meta{font-size:.85rem;color:#8b949e}
header .meta span{margin-right:1.5rem}
.message{margin-bottom:1.5rem;padding:1rem 1.25rem;border-radius:8px;background:#161b22}
.message-user{border-left:3px solid #58a6ff}
.message-assistant{border-left:3px solid #8b949e}
.message .role{font-size:.8rem;font-weight:600;text-transform:uppercase;color:#8b949e;margin-bottom:.5rem;letter-spacing:.03em}
.message-user .role{color:#58a6ff}
.message .content{font-size:.95rem}
.content p{margin:.5em 0}
.content pre{background:#0d1117;border:1px solid #21262d;border-radius:6px;padding:1rem;overflow-x:auto;margin:.75em 0;font-size:.85rem;line-height:1.45}
.content code{font-family:"SFMono-Regular",Consolas,"Liberation Mono",Menlo,monospace;font-size:.85em}
.content p code,.content li code{background:#21262d;padding:.15em .4em;border-radius:4px}
.content ul,.content ol{padding-left:1.5em;margin:.5em 0}
.content blockquote{border-left:3px solid #30363d;padding-left:1em;color:#8b949e;margin:.75em 0}
.content h1,.content h2,.content h3,.content h4{color:#f0f6fc;margin:.75em 0 .25em}
.content table{border-collapse:collapse;margin:.75em 0;width:100%}
.content th,.content td{border:1px solid #21262d;padding:.4em .8em;text-align:left}
.content th{background:#161b22;color:#f0f6fc}
details{margin:.5em 0}
details summary{cursor:pointer;font-weight:600;font-size:.85rem;color:#8b949e;padding:.25em 0}
details summary:hover{color:#c9d1d9}
.tool-call summary{color:#3fb950}
.tool-call .tool-args pre{background:#0d1117;border:1px solid #21262d;border-radius:6px;padding:.75rem;font-size:.8rem;margin-top:.5rem}
.tool-result .tool-output{background:#0d1117;border:1px solid #21262d;border-radius:6px;padding:.75rem;font-size:.8rem;margin-top:.5rem;max-height:400px;overflow-y:auto}
.thinking summary{color:#d2a8ff}
.thinking-content pre{background:#0d1117;border:1px solid #21262d;border-radius:6px;padding:.75rem;font-size:.8rem;margin-top:.5rem;white-space:pre-wrap;word-wrap:break-word}
@media(max-width:600px){body{padding:1rem}header h1{font-size:1.25rem}.message{padding:.75rem 1rem}}
</style>
</head>
<body>
<header>
<h1>{{.Title}}</h1>
<div class="meta">
<span>ID: {{.ID}}</span>
{{- if .Created}}<span>Created: {{.Created}}</span>{{end}}
{{- if .Updated}}<span>Updated: {{.Updated}}</span>{{end}}
</div>
</header>
<div class="messages">
{{- range .Messages}}
<div class="message message-{{.Role}}">
<div class="role">{{.Label}}</div>
<div class="content">{{.Content}}</div>
</div>
{{- end}}
</div>
</body>
</html>
`
