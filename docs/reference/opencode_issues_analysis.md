# What's Wrong with OpenCode

These aren't theoretical complaints. I've been in the code, run it daily, and forked it into tinycode. These are the problems I hit.

---

## Security - the real ones

**The bash sandbox doesn't work.** OpenCode tries to restrict shell commands by parsing them into an AST and checking against user rules. That's fundamentally broken. Variables, aliases, Python subprocess calls, base64 decoding - all bypass it trivially. It's security theater.

**"Always allow" means forever.** When the agent asks permission to run a command and you click "Always," it approves that command prefix globally across all sessions. Say yes to `python3 -c 'print("hello")'` and you've permanently granted the agent permission to run any arbitrary Python script. There's no expiration, no scope limit, no review.

**Path validation only covers the obvious commands.** The system checks file paths for `cat`, `rm`, and a hardcoded list of basics. It completely misses shell redirections (`>`) because those are parsed as AST siblings, not command arguments. Commands like `cd` and `pushd` explicitly bypass permission checks entirely.

**No network sandbox.** The agent has a WebFetch tool and unrestricted network access from bash. If the model hallucinates a `curl | bash` command - or gets prompt-injected into running one - your machine is wide open. There was already a CVE where a default-enabled HTTP server exposed unauthenticated APIs for arbitrary shell commands and file reads with permissive CORS headers.

**Remote model fallback is silent.** OpenCode defaults to a remote model and auto-downloads its URL payload. If your local setup is misconfigured, it silently falls back to the remote model. You're now exposing a local shell to a remote server and you don't know it.

---

## Performance - death by cache invalidation

**The prompt cache gets destroyed constantly.** OpenCode re-reads AGENTS.md and injects the current date on every SSE turn. It prunes context during agent-to-user transitions, discarding tool call results older than 40K tokens. Interrupting the agent invalidates the cache too. The result: long prefill times on every turn because nothing stays cached.

**Compaction is expensive and lossy.** When sessions get long, the system summarizes to save context. The summarization itself is slow, and switching between Plan and Build modes causes additional cache misses if the system prompt overrides aren't mirrored. You pay for the compaction and then pay again on the next turn.

**System prompts are bloated and contradictory.** The defaults are extremely verbose and can't be globally modified - you have to copy them per project. Some instructions conflict: the prompt tells the agent it can't write to directories while also expecting it to write to `.opencode/plans`. The model gets confused because the instructions are confused.

---

## The TUI

**1GB of RAM for a text interface.** That's not a typo.

**Markdown rendering falls apart on long output.** Performance degrades visibly as messages grow. Chain-of-thought output makes it worse.

**Basic input is broken.** Option+Arrow for word skipping doesn't work. Soft-wrapping hides text in multi-line input. Selecting text during a stream auto-scrolls and kills your selection. Ctrl+C terminates the entire session instead of interrupting the current command.

---

## Agent control

**Queued messages get thrown away.** You can type while the agent is streaming, but if you interrupt, your queued message disappears. You have to retype it.

**Subagents are fragile and opaque.** You can't talk to subagents directly. If one hits an error or needs a permission you deny, it dies and takes all its context with it. No recovery, no handoff.

**No "Never" option on permissions.** You get "Yes," "No," or "Always." No way to permanently deny a specific action. When the agent keeps asking to do something you don't want, you're clicking "No" on every attempt.

**Custom tools are redundant.** OpenCode's built-in grep and glob tools are error-prone compared to exact search-replace. Native `grep` or `rg` in bash do the same job better. The custom tools add complexity without adding value.
