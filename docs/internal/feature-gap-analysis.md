# Feature Gap Analysis — AI Coding Assistant Patterns

Date: 2026-10-02
Source: Competitive analysis of upstream AI coding assistant documentation

## Features Adopted (Issues Created)

| # | Feature | Effort | Priority |
|---|---------|--------|----------|
| #338 | `/context` — context window visualization | Low | 1 |
| #339 | `/btw` — side questions without context pollution | Low | 2 |
| #340 | `/branch` — session branching for exploration | Low | 3 |
| #341 | `/export` + `/copy` — clipboard and file export | Low | 4 |
| #342 | `/effort` — reasoning depth control per session | Low | 5 |
| #343 | `/rewind` — checkpointing and rollback | Medium | 6 |
| #344 | `/goal` — autonomous multi-turn execution | Medium | 7 |
| #345 | Lifecycle hooks system | High | 8 |
| #346 | `/batch` — parallel decomposition of large changes | High | 9 |
| #347 | Enhanced `/tc-doctor` with config audit | Low | 10 | *(superseded: Go `tinycode doctor` + TUI `/doctor`; legacy bash `/tc-doctor` obsolete)* |

## Features Skipped

These features were evaluated and intentionally not adopted. They don't fit tinycode's local-first, model-agnostic philosophy or require infrastructure tinycode doesn't have.

### Cloud-Dependent Features

| Feature | Description | Why Skip |
|---------|-------------|----------|
| Remote Control | Control a local session from a web app on another device | Requires cloud relay infrastructure. The local-first model means the session runs where the terminal is. |
| Cloud Sessions / Routines | Run sessions in cloud sandboxes, schedule recurring tasks in the cloud | Core premise of tinycode is local execution. Cloud sessions contradict the "your data stays on your machine" guarantee. |
| Artifacts (published pages) | Create interactive web pages hosted on a cloud platform | Requires hosted rendering platform and user accounts. |
| Slides / Design artifacts | Create presentations and design canvases as hosted artifacts | Same as artifacts — requires cloud canvas service. |
| Deep Research workflow | Fan out web searches, fetch and cross-check sources, synthesize a cited report | Requires web search tool with rate limits and API keys. Could be reconsidered if tinycode adds a web search MCP server. |
| Ultra Review (cloud code review) | Multi-agent code review running in a cloud sandbox | Requires cloud compute sandbox. tinycode's local `/code-review` skill covers this adequately. |
| Voice dictation | Speak prompts instead of typing | Requires cloud speech-to-text API. Could revisit if local whisper.cpp integration becomes practical. |
| Teleport (cloud-to-local) | Pull a cloud session into a local terminal | No cloud sessions to pull from. |

### Platform-Specific Features

| Feature | Description | Why Skip |
|---------|-------------|----------|
| Chrome integration | Control a browser extension from the CLI | Browser extension ecosystem. tinycode's MCP integration can connect to browser tools if needed. |
| Desktop app wrapping | Dedicated desktop application | tinycode already has an Electron desktop app (`packages/desktop`). Not a gap. |
| Slack app integration | Bot that responds in Slack channels | Requires Slack API integration and always-on service. Outside scope. |

### Enterprise / Organizational Features

| Feature | Description | Why Skip |
|---------|-------------|----------|
| Managed settings / org admin | Organization-wide settings pushed to all users | Enterprise cloud feature requiring central management. tinycode's config system (global + project + local) covers individual users. |
| Gateway / proxy configuration | Route API calls through an organizational proxy | Enterprise deployment pattern. Users configure providers directly. |
| Auto mode with classifier rules | ML-based permission classifier for autonomous operation | Requires training data and cloud classifier. tinycode's permission system (ask/allow/deny rules) is sufficient. |

### Marketing / Social Features

| Feature | Description | Why Skip |
|---------|-------------|----------|
| Stickers | Order physical stickers | Marketing. |
| Radio | Lo-fi music stream | Fun but off-mission. |
| Passes | Share free access with friends | Subscription-based feature. |
| Team onboarding guide | Generate onboarding docs from usage history | Cloud-dependent sharing. Could revisit as a local export. |
| Insights report | HTML report analyzing usage patterns | Interesting but low priority. Could be a future skill. |

### Premature Features

| Feature | Description | Why Skip |
|---------|-------------|----------|
| Import from competitor tools | Bring config from other AI coding tools | Niche, high maintenance burden. Revisit when tinycode has enough users migrating from other tools. |
| Plugin marketplace / distribution | Registry for discovering and installing community plugins | Ecosystem too small (#317 already deferred as a separate issue). |
| Agent teams (multi-session coordination) | Multiple sessions messaging each other to coordinate | Experimental, complex. tinycode's subagent system covers the core use case. |
| Focus / fullscreen rendering mode | Alt-screen renderer with collapsed tool output | Nice-to-have TUI enhancement. Could be added later as a view mode toggle. |
| Output styles (concise/verbose/etc) | Predefined response formatting profiles | Low value — users can adjust via agent prompts. Revisit if users request it. |

## Reassessment Criteria

A skipped feature should be reconsidered if:
1. **Local alternative becomes available** — e.g., local speech-to-text (whisper.cpp) makes voice dictation feasible
2. **User demand** — multiple users request the feature in issues or discussions
3. **Infrastructure change** — e.g., tinycode adds a web search MCP server, making deep research feasible
4. **Ecosystem growth** — e.g., 50+ community plugins make a marketplace worthwhile
