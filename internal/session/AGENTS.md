# AGENTS.md (session)

`internal/session` owns persistence and the agent loop.

- `Store` (`session.go`) and `MessageStore` (`message.go`) persist sessions and messages.
- The loop is `Processor.Process` / `Processor.ProcessWithID`.
- Compaction during a turn runs on `Processor` (`compact`, from the loop in `processor_loop.go`). `Processor.Compact` is that compact implementation.
- `Processor.Abort()` is the loop cancel flag.

`SessionManager` in `internal/server` is the other side of abort and compaction. `SessionManager.Abort` cancels the prompt context and then calls `Processor.Abort`. `SessionManager.Summarize` wraps `Processor.Compact`. See [internal/server/AGENTS.md](../server/AGENTS.md).
