export type ExportMessage = {
  role: string
  agent?: string
  modelID?: string
  parts: ExportPart[]
}

export type ExportPart =
  | { type: "text"; text: string }
  | { type: "reasoning"; text: string }
  | { type: "tool"; tool?: string; title?: string; output?: string; error?: string; status?: string }
  | { type: string; text?: string }

export type ExportSession = {
  id: string
  title?: string
  time?: { created?: number; updated?: number }
}

export function sanitizeFilename(s: string): string {
  let out = s.toLowerCase()
  out = out
    .split("")
    .map((ch) => {
      const code = ch.charCodeAt(0)
      if ((code >= 97 && code <= 122) || (code >= 48 && code <= 57) || ch === "-") return ch
      if (ch === " " || ch === "_") return "-"
      return ""
    })
    .join("")
  while (out.includes("--")) out = out.replaceAll("--", "-")
  out = out.replace(/^-+|-+$/g, "")
  if (out.length > 40) out = out.slice(0, 40)
  return out
}

export function sessionSlug(session: ExportSession): string {
  const fromTitle = sanitizeFilename(session.title ?? "")
  if (fromTitle) return fromTitle
  const id = session.id
  return id.length > 8 ? id.slice(0, 8) : id
}

function formatAssistantHeader(msg: ExportMessage): string {
  let agent = msg.agent || "Build"
  if (agent) agent = agent.charAt(0).toUpperCase() + agent.slice(1)
  const parts = [agent]
  if (msg.modelID) parts.push(msg.modelID)
  return `## Assistant (${parts.join(" · ")})\n\n`
}

function formatPartMarkdown(part: ExportPart): string {
  if (part.type === "text") {
    if (!part.text) return ""
    return `${part.text}\n\n`
  }
  if (part.type === "reasoning") {
    if (!("text" in part) || !part.text) return ""
    return `_Thinking:_\n\n${part.text}\n\n`
  }
  if (part.type === "tool" && "tool" in part) {
    const name = part.tool || part.title || "unknown"
    let result = `**Tool: ${name}**\n\n`
    const output = part.output || part.error
    if (output) {
      const lines = output.split("\n")
      const clipped = lines.length > 50 ? `${lines.slice(0, 50).join("\n")}\n... (${lines.length - 50} more lines)` : output
      result += `**Output:**\n\`\`\`\n${clipped}\n\`\`\`\n\n`
    }
    return result
  }
  if ("text" in part && part.text) return `${part.text}\n\n`
  return ""
}

export function formatTranscript(session: ExportSession, messages: ExportMessage[]): string {
  const title = session.title?.trim() || "Untitled Session"
  const lines: string[] = [`# ${title}`, "", `**Session ID:** ${session.id}`]
  if (session.time?.created) {
    lines.push(`**Created:** ${new Date(session.time.created).toUTCString()}`)
  }
  if (session.time?.updated) {
    lines.push(`**Updated:** ${new Date(session.time.updated).toUTCString()}`)
  }
  lines.push("", "---", "")

  for (const msg of messages) {
    if (msg.role === "user") lines.push("## User", "")
    else if (msg.role === "assistant") lines.push(formatAssistantHeader(msg).trimEnd(), "")
    else lines.push(`## ${msg.role}`, "")

    for (const part of msg.parts) {
      const chunk = formatPartMarkdown(part)
      if (chunk) lines.push(chunk.trimEnd(), "")
    }
    lines.push("---", "")
  }

  return lines.join("\n")
}

function escapeHtml(s: string): string {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
}

export function formatTranscriptHtml(session: ExportSession, messages: ExportMessage[]): string {
  const title = session.title?.trim() || "Untitled Session"
  const body = messages
    .map((msg) => {
      let label = msg.role
      if (msg.role === "user") label = "User"
      if (msg.role === "assistant") {
        let agent = msg.agent || "Build"
        agent = agent.charAt(0).toUpperCase() + agent.slice(1)
        const bits = [agent]
        if (msg.modelID) bits.push(msg.modelID)
        label = `Assistant (${bits.join(" · ")})`
      }
      const content = msg.parts
        .map((part) => formatPartMarkdown(part))
        .join("")
        .trim()
      return `<section class="message ${escapeHtml(msg.role)}"><h2>${escapeHtml(label)}</h2><pre>${escapeHtml(content)}</pre></section>`
    })
    .join("\n")

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>${escapeHtml(title)}</title>
<style>
body{font-family:ui-sans-serif,system-ui,sans-serif;max-width:48rem;margin:2rem auto;padding:0 1rem;line-height:1.5;color:#111}
h1{font-size:1.5rem} h2{font-size:1.1rem;margin-top:1.5rem}
pre{white-space:pre-wrap;word-break:break-word;background:#f6f6f6;padding:0.75rem;border-radius:0.375rem}
.meta{color:#666;font-size:0.875rem}
</style>
</head>
<body>
<h1>${escapeHtml(title)}</h1>
<p class="meta">Session ID: ${escapeHtml(session.id)}</p>
${body}
</body>
</html>
`
}

export function downloadBlob(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  a.rel = "noopener"
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
