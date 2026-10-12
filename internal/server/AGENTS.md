# AGENTS.md (server)

`SessionManager` stays in `internal/server`. `server.New` constructs it (`NewSessionManager`).

- Busy/idle: `IsBusy` is an active prompt; `processPrompt` publishes `busy` at start and `idle` when the turn finishes.
- `StartPrompt` / `processPrompt` run one `Processor` per prompt.
- `Summarize` wraps `Processor.Compact`.
- Revert stash is `session_tools.go`.
- The SSE bridge is `session_events.go`.
- Persist helpers are `session_ops_helpers.go` (`persistPromptResult`).
- Subagents are `session_subagent.go` (`RunSubagent`).

`Processor.Abort()` in `internal/session` is the loop cancel flag. `SessionManager.Abort` cancels the prompt context and then calls it. Compaction during a turn is `Processor`; `Processor.Compact` is the compact implementation, and `Summarize` wraps it.

`initDependencies` in `cmd/tinycode/config.go` returns only the bus, database, and config. Callers: `tui.go`, `serve.go` (`runServe` and `runWeb`), `acp.go`, `run.go`, and the CLI helpers in `commands.go`.

`Store`, `MessageStore`, and the loop are [internal/session/AGENTS.md](../session/AGENTS.md).
