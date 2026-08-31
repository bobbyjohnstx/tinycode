---
name: wiki
description: Guide agents on using the project wiki for knowledge persistence and retrieval
---

# Wiki

Use this skill when you need to store, retrieve, or manage project-level knowledge that should persist across sessions and be discoverable by future agents.

## When to Use

Use this skill when:
- You discover project-specific patterns, conventions, or gotchas worth preserving
- A debugging session uncovers a root cause or workaround that others will hit again
- An architectural decision is made with rationale that should be recorded
- Environment setup quirks or platform-specific issues are identified
- You want to check if existing wiki knowledge is relevant before starting a task
- Onboarding context needs to be captured for future contributors

## When Not to Use

- Scratch notes for the current session only — use the notepad instead
- User preferences or directives — use project memory (`omt_project_memory_*`)
- Direct edits to `CLAUDE.md` or `AGENTS.md` — edit those files directly
- Session-only working state — keep it in the conversation or notepad working memory
- Ephemeral task progress — use notepad working memory or state tools

## Available Tools

- `omt_wiki_query` — search across all wiki pages by keywords, tags, or category
- `omt_wiki_read` — read a specific wiki page by filename or slug
- `omt_wiki_list` — list all wiki pages with summaries
- `omt_wiki_add` — create a single new wiki page directly
- `omt_wiki_ingest` — create or merge knowledge into wiki pages (handles existing page merging)
- `omt_wiki_delete` — remove a wiki page by filename

## Workflow

1. **Query first** — before starting a task, query the wiki for existing knowledge that might be relevant. Use `omt_wiki_query` with keywords from the task description. This avoids re-discovering known issues or contradicting established decisions.
2. **Use findings** — incorporate wiki knowledge into your work. Reference existing decisions, avoid known pitfalls, and follow documented patterns.
3. **Ingest durable findings** — after completing work, ingest any durable findings back into the wiki. Ask yourself: would a future agent benefit from knowing this? If yes, write it to the wiki.

## Categories

Wiki pages are organized by category. Use the most specific category that fits:

- `architecture` — system design, component relationships, data flow
- `decision` — architectural or technical decisions with rationale
- `pattern` — recurring code patterns, idioms, or conventions
- `debugging` — root causes, workarounds, diagnostic procedures
- `environment` — setup quirks, platform-specific issues, toolchain notes
- `session-log` — session summaries or progress logs
- `reference` — general reference material that does not fit other categories
- `convention` — team or project conventions and standards

## Rules

- Prefer `omt_wiki_add` for simple, standalone pages. Use `omt_wiki_ingest` when content should be merged into an existing page or when providing structured metadata (confidence, sources).
- Always include relevant tags for discoverability. Tags should be lowercase, specific terms that someone might search for.
- Set confidence level accurately: `high` for verified facts, `medium` for likely-correct findings, `low` for hypotheses or untested observations.
- Keep pages focused on a single topic. Split broad findings into multiple pages rather than creating one large page.
- Before creating a new page, query the wiki to check if a relevant page already exists. Merge into existing pages when appropriate.
- Include source context (file paths, error messages, reproduction steps) so future agents can verify the information is still current.
