# Tools Gap Analysis — AI Coding Assistant Tool Patterns

Date: 2026-10-02
Source: Competitive analysis of upstream AI coding assistant tools reference

## Current Tool Inventory (16 tools)

tinycode already implements these tools:

| Tool | ID | Location | Description |
|------|----|----------|-------------|
| Shell | `bash` | `internal/tool/shell.go` | Shell command execution with output cap, secret detection, destructive-command guard |
| Read | `read` | `internal/tool/read.go` | File reading with line numbers, size cap |
| Edit | `edit` | `internal/tool/edit.go` | String replacement with Levenshtein fuzzy matching |
| Write | `write` | `internal/tool/write.go` | File creation/overwrite |
| Glob | `glob` | `internal/tool/glob.go` | File pattern matching |
| Grep | `grep` | `internal/tool/grep.go` | Content search |
| WebFetch | `webfetch` | `internal/tool/webfetch.go` | URL fetching with SSRF protection |
| WebSearch | `websearch` | via MCP | Web search |
| LSP Diagnostics | `diagnostics` | `internal/tool/diagnostics.go` | Post-edit linting (go vet, ruff, tsc) |
| LSP Hover | `lsp_hover` | `internal/lsp/tools.go` | Type information at position |
| LSP Definition | `lsp_definition` | `internal/lsp/tools.go` | Jump to symbol definition |
| LSP References | `lsp_references` | `internal/lsp/tools.go` | Find all references |
| LSP Symbols | `lsp_symbols` | `internal/lsp/tools.go` | List symbols in file |
| Apply Patch | `apply_patch` | `internal/tool/apply_patch.go` | Unified diff application |
| Task | `task` | `internal/tool/task.go` | Subagent spawning (foreground + background) |
| Question | `question` | `internal/tool/question.go` | Interactive user questions |
| Skill | `skill` | via command system | Skill execution |
| TodoWrite | `todowrite` | `internal/tool/todowrite.go` | Task checklist |

## Tools to Add (5 identified)

### 1. Monitor — Background Process Watcher (HIGH priority)

**What it does**: Runs a command in the background and feeds each output line back to the model as events. The model can react to log entries, file changes, or polled status mid-conversation without pausing work.

**Use cases**:
- Tail a server log and alert on errors: `monitor tail -f /var/log/app.log`
- Watch test output: `monitor go test -v ./... 2>&1`
- Poll a build: `monitor watch -n 5 make build`
- Watch a directory for changes: `monitor fswatch src/`

**Why it fits local-first**: Entirely local execution. No cloud dependency. Uses `exec.Command` with line-buffered stdout scanning. The model processes events in the existing conversation loop.

**Implementation approach**:
- New tool at `internal/tool/monitor.go`
- Spawns background process via `exec.CommandContext` with line scanner on stdout/stderr
- Each line becomes an event delivered to the session processor
- Timeout: 5 minutes default, 30 minutes max
- Permission: same rules as shell tool (reuse shell permission patterns)
- Stop via `/tasks` or model request
- WebSocket variant deferred (requires more infrastructure)

**Estimated effort**: Medium (new tool + event integration with processor loop)

### 2. ReportFindings — Structured Code Review Output (MEDIUM priority)

**What it does**: Allows the model to report code review findings as a typed list instead of prose. Each finding has: file, line, severity, summary, failure scenario, and optional verdict.

**Use cases**:
- `/review` outputs machine-parseable findings instead of markdown prose
- TUI can render findings as a structured list with severity icons
- Findings can be filtered, sorted, and acted on programmatically

**Why it fits local-first**: Pure data structure — no cloud dependency. The tool just structures the model's output.

**Implementation approach**:
- New tool at `internal/tool/report_findings.go`
- Input: array of finding structs (file, line, severity, summary, failure_scenario)
- Output: stored on the session, rendered by TUI/web UI
- The `/review` skill instructs the model to use this tool instead of prose output
- TUI renders findings with severity-colored icons and file:line links

**Estimated effort**: Low-Medium (tool is simple, rendering is the work)

### 3. PushNotification — Local Desktop Notifications (MEDIUM priority)

**What it does**: Sends a desktop notification so long-running tasks can alert the user without requiring them to watch the terminal.

**Use cases**:
- "Notify me when the build finishes"
- Goal completion notification (when `/goal` succeeds)
- Background subagent completion

**Why it fits local-first**: Uses OS-native notification APIs — `notify-send` (Linux), `osascript -e 'display notification'` (macOS), `powershell` toast (Windows). Zero cloud.

**Implementation approach**:
- New tool at `internal/tool/notify.go`
- Detects OS and uses appropriate notification command
- Input: title, message, optional urgency level
- Also wire into goal completion and subagent completion as automatic notifications
- Permission: no prompt needed (notifications are non-destructive)

**Estimated effort**: Low (simple exec.Command wrapper with OS detection)

### 4. NotebookEdit — Jupyter Cell Editing (MEDIUM priority)

**What it does**: Modifies Jupyter notebook cells by cell ID. Supports replace, insert, and delete modes.

**Use cases**:
- Edit code cells in `.ipynb` files without overwriting the entire notebook
- Insert new cells after existing ones
- Delete cells

**Why it fits local-first**: Pure file manipulation of JSON-structured `.ipynb` files. Model-agnostic.

**Implementation approach**:
- New tool at `internal/tool/notebook_edit.go`
- Parse `.ipynb` as JSON, find cell by `cell_id`
- Three modes: replace (overwrite source), insert (add after target), delete (remove target)
- Validate cell_type on insert (code or markdown)
- Permission: same as edit tool

**Estimated effort**: Medium (JSON parsing + cell manipulation + edge cases)

### 5. Worktree Tools — Explicit Git Worktree Management (MEDIUM priority)

**What it does**: Gives the model explicit tools to create and exit git worktrees, enabling autonomous isolation of risky work.

**Use cases**:
- "I'll try this risky refactor in a worktree" — model creates isolation without user intervention
- Parallel experiments in separate worktrees
- Clean rollback by discarding the worktree

**Why it fits local-first**: Git worktrees are local filesystem operations. No cloud.

**Implementation approach**:
- Two new tools: `enter_worktree` and `exit_worktree`
- `enter_worktree`: creates `.claude/worktrees/<name>/` via `git worktree add`, switches session directory
- `exit_worktree`: returns to original directory, optionally removes worktree
- Reuse existing worktree infrastructure from `/swarm`
- Permission: prompt on enter (changes working directory), no prompt on exit

**Estimated effort**: Medium (git worktree management + session directory switching)

## Tools Skipped

| Tool | Why Skip |
|------|----------|
| SendMessage | Cross-session messaging requires cloud relay infrastructure |
| Workflow | Multi-agent orchestration scripts — `/swarm` covers the use case |
| PowerShell | Windows-specific — `bash` tool handles shell commands cross-platform |
| EndConversation | Abuse protection — not needed for local-first personal tool |
| SendFeedback | Cloud feedback submission |
| Artifact | Cloud-hosted interactive pages |
| ShareOnboardingGuide | Cloud sharing |
| SendUserFile | Cloud file delivery |
| SubagentHandback | Auto-mode subagent result delivery — niche |
| CronCreate/Delete/List | Scheduled tasks within session — `/goal` and `/work-loop` cover the main use case |
| TaskCreate/Get/List/Update | Structured task tracking — `todowrite` covers the basic case |
| EnterPlanMode/ExitPlanMode | Plan mode exists as `/plan` command — tools would add marginal value |

## Reassessment Criteria

A skipped tool should be reconsidered if:
1. User demand — multiple users request the capability
2. Infrastructure change — e.g., tinycode adds cross-session communication, making SendMessage feasible
3. Use case evolution — e.g., PowerShell becomes important for Windows adoption
