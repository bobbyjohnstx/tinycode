import { Session } from "@/session/session"
import { MessageV2 } from "../../session/message-v2"

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;")
}

function formatDate(timestamp: number): string {
  return new Date(timestamp).toLocaleString()
}

function formatTokens(tokens: Session.Info["tokens"]): string {
  if (!tokens) return "N/A"
  return `${tokens.input.toLocaleString()} in / ${tokens.output.toLocaleString()} out`
}

function renderCodeBlock(code: string, language?: string): string {
  return `<pre><code class="language-${language || "text"}">${escapeHtml(code)}</code></pre>`
}

function renderTextPart(text: string): string {
  const parts: string[] = []
  let current = 0

  const codeBlockRegex = /```(\w+)?\n([\s\S]*?)```/g
  let match: RegExpExecArray | null

  while ((match = codeBlockRegex.exec(text)) !== null) {
    if (match.index > current) {
      const beforeText = text.slice(current, match.index)
      parts.push(`<p>${escapeHtml(beforeText).replace(/\n/g, "<br>")}</p>`)
    }

    const language = match[1]
    const code = match[2]
    parts.push(renderCodeBlock(code, language))
    current = match.index + match[0].length
  }

  if (current < text.length) {
    const remainingText = text.slice(current)
    parts.push(`<p>${escapeHtml(remainingText).replace(/\n/g, "<br>")}</p>`)
  }

  return parts.join("")
}

function renderToolPart(part: MessageV2.ToolPart): string {
  const name = part.tool
  const state = part.state

  let inputHtml = ""
  let outputHtml = ""
  let title = name

  if (state.status === "pending" || state.status === "running" || state.status === "completed") {
    if (state.input) {
      inputHtml = `<div class="tool-input"><strong>Input:</strong><pre>${escapeHtml(JSON.stringify(state.input, null, 2))}</pre></div>`
    }
  }

  if (state.status === "completed") {
    title = state.title || name
    outputHtml = `<div class="tool-output"><strong>Output:</strong><pre>${escapeHtml(state.output)}</pre></div>`

    if (state.attachments && state.attachments.length > 0) {
      const attachmentsHtml = state.attachments
        .map((att: MessageV2.FilePart) => `<div class="attachment">📎 ${escapeHtml(att.filename || "attachment")}</div>`)
        .join("")
      outputHtml += `<div class="tool-attachments">${attachmentsHtml}</div>`
    }
  } else if (state.status === "error") {
    outputHtml = `<div class="tool-error"><strong>Error:</strong><pre>${escapeHtml(state.error)}</pre></div>`
  }

  return `
    <details class="tool-call">
      <summary>🔧 ${escapeHtml(title)}</summary>
      ${inputHtml}
      ${outputHtml}
    </details>
  `
}

function renderPart(part: MessageV2.Part): string {
  switch (part.type) {
    case "text":
      return `<div class="part-text">${renderTextPart(part.text)}</div>`

    case "reasoning":
      return `
        <details class="part-reasoning">
          <summary>💭 Thinking...</summary>
          <div class="reasoning-content">${escapeHtml(part.text).replace(/\n/g, "<br>")}</div>
        </details>
      `

    case "tool":
      return renderToolPart(part)

    case "file":
      return `
        <div class="part-file">
          📄 File: <code>${escapeHtml(part.filename || part.url || "unknown")}</code>
        </div>
      `

    case "patch":
      const files = part.files.join(", ")
      return `
        <div class="part-patch">
          📝 Patch: <code>${escapeHtml(files)}</code>
        </div>
      `

    case "step-start":
      return `<div class="part-step">▶️ Step started</div>`

    case "step-finish":
      return `<div class="part-step">✓ Step finished</div>`

    case "subtask":
      return `
        <div class="part-subtask">
          <strong>Subtask:</strong> ${escapeHtml(part.description)}
          ${part.command ? `<br><code>${escapeHtml(part.command)}</code>` : ""}
        </div>
      `

    case "agent":
      return `
        <div class="part-agent">
          🤖 Agent: <code>${escapeHtml(part.name)}</code>
        </div>
      `

    default:
      return ""
  }
}

function renderMessage(msg: MessageV2.WithParts): string {
  const isUser = msg.info.role === "user"
  const className = isUser ? "message user-message" : "message assistant-message"
  const roleLabel = isUser ? "User" : "Assistant"

  const partsHtml = msg.parts.map(renderPart).join("")

  return `
    <div class="${className}">
      <div class="message-header">
        <span class="message-role">${roleLabel}</span>
        <span class="message-time">${formatDate(msg.info.time.created)}</span>
      </div>
      <div class="message-content">
        ${partsHtml}
      </div>
    </div>
  `
}

export function renderHTML(data: { info: Session.Info; messages: MessageV2.WithParts[] }): string {
  const title = data.info.title
  const model = (data.info as any).config?.model?.name || "unknown"
  const createdDate = formatDate(data.info.time.created)
  const updatedDate = formatDate(data.info.time.updated)
  const tokens = formatTokens(data.info.tokens)

  const messagesHtml = data.messages.map(renderMessage).join("")

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>${escapeHtml(title)}</title>
  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
      font-size: 14px;
      line-height: 1.6;
      color: #e6edf3;
      background: #0d1117;
      padding: 2rem;
    }

    .container {
      max-width: 900px;
      margin: 0 auto;
    }

    .header {
      background: #161b22;
      border: 1px solid #30363d;
      border-radius: 6px;
      padding: 1.5rem;
      margin-bottom: 2rem;
    }

    .header h1 {
      font-size: 1.5rem;
      margin-bottom: 0.75rem;
      color: #58a6ff;
    }

    .metadata {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 0.5rem 1rem;
      font-size: 0.875rem;
      color: #8b949e;
    }

    .metadata-label {
      font-weight: 600;
    }

    .message {
      background: #161b22;
      border: 1px solid #30363d;
      border-radius: 6px;
      padding: 1rem;
      margin-bottom: 1rem;
    }

    .user-message {
      background: #0d1117;
      border-color: #21262d;
      max-width: 85%;
    }

    .assistant-message {
      background: #161b22;
      border-color: #30363d;
    }

    .message-header {
      display: flex;
      justify-content: space-between;
      margin-bottom: 0.75rem;
      padding-bottom: 0.5rem;
      border-bottom: 1px solid #21262d;
    }

    .message-role {
      font-weight: 600;
      color: #58a6ff;
    }

    .message-time {
      font-size: 0.75rem;
      color: #8b949e;
    }

    .message-content {
      color: #e6edf3;
    }

    .part-text p {
      margin-bottom: 0.75rem;
    }

    .part-text p:last-child {
      margin-bottom: 0;
    }

    pre {
      background: #0d1117;
      border: 1px solid #30363d;
      border-radius: 6px;
      padding: 1rem;
      overflow-x: auto;
      margin: 0.75rem 0;
    }

    code {
      font-family: "SF Mono", Monaco, "Cascadia Code", "Roboto Mono", Consolas, "Courier New", monospace;
      font-size: 0.875rem;
      color: #79c0ff;
    }

    pre code {
      color: #e6edf3;
    }

    details {
      background: #0d1117;
      border: 1px solid #30363d;
      border-radius: 6px;
      margin: 0.75rem 0;
    }

    summary {
      padding: 0.75rem 1rem;
      cursor: pointer;
      font-weight: 600;
      color: #58a6ff;
      user-select: none;
    }

    summary:hover {
      background: #161b22;
    }

    details[open] summary {
      border-bottom: 1px solid #30363d;
    }

    .reasoning-content,
    .tool-input,
    .tool-output,
    .tool-error {
      padding: 1rem;
    }

    .tool-input strong,
    .tool-output strong,
    .tool-error strong {
      display: block;
      margin-bottom: 0.5rem;
      color: #8b949e;
    }

    .tool-error {
      color: #f85149;
    }

    .tool-attachments {
      margin-top: 0.5rem;
      padding-top: 0.5rem;
      border-top: 1px solid #30363d;
    }

    .attachment {
      padding: 0.25rem 0;
      color: #8b949e;
    }

    .part-file,
    .part-patch,
    .part-step,
    .part-subtask,
    .part-agent {
      background: #0d1117;
      border: 1px solid #30363d;
      border-radius: 6px;
      padding: 0.75rem 1rem;
      margin: 0.75rem 0;
      color: #8b949e;
    }

    .part-subtask strong {
      color: #e6edf3;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>${escapeHtml(title)}</h1>
      <div class="metadata">
        <div class="metadata-label">Model:</div>
        <div>${escapeHtml(model)}</div>
        <div class="metadata-label">Created:</div>
        <div>${createdDate}</div>
        <div class="metadata-label">Updated:</div>
        <div>${updatedDate}</div>
        <div class="metadata-label">Tokens:</div>
        <div>${tokens}</div>
      </div>
    </div>
    <div class="messages">
      ${messagesHtml}
    </div>
  </div>
</body>
</html>`
}
