---
name: team
description: Launch a multi-agent team to work on a task in parallel
---

# Team — Multi-Agent Parallel Execution

Launch multiple tinycode worker sessions to tackle a task in parallel using the oh-my-tiny coordinator.

## When to use

- The task has clearly separable subtasks (different files, different features, different investigations)
- A single agent would benefit from parallelism (e.g., implement N independent modules, run N searches)
- You want to fan out work and collect results

## Worker count guidance

- **2 workers**: default for most tasks; low overhead, good parallelism
- **3–4 workers**: larger fan-outs (e.g., implement 4 independent features)
- **Avoid 5+**: each worker is a full tinycode session; more workers = more context window usage and coordination cost

Each worker has access to the full tinycode tool set (file read/write/edit, shell, grep, glob, LSP, etc.).

---

## Mode 1: HTTP API mode (preferred)

Use this when the tinycode server is already running on port 4096. Workers become tinycode sessions driven via the HTTP API. This is the default and recommended approach.

```bash
node /Users/bjohns/projects/tinycode/node_modules/oh-my-tiny/dist/team/coordinator.js \
  --server-url http://localhost:4096 \
  --team <team-name> \
  --workers <count> \
  --task "<task description>" \
  --cwd <project-directory>
```

### Example

```bash
node /Users/bjohns/projects/tinycode/node_modules/oh-my-tiny/dist/team/coordinator.js \
  --server-url http://localhost:4096 \
  --team refactor-team \
  --workers 3 \
  --task "Refactor the three provider modules (ollama, openai, anthropic) to use the new BaseProvider interface. Each worker takes one module." \
  --cwd /Users/bjohns/projects/tinycode
```

### TASK_DONE protocol

In HTTP API mode, each worker signals completion by including one of these tokens in its final response:

- `TASK_DONE:taskId:success` — worker completed successfully
- `TASK_DONE:taskId:failed` — worker encountered an unrecoverable error

The coordinator monitors these signals to know when all workers are finished and to aggregate results.

---

## Mode 2: tmux mode (visual monitoring)

Use this when you want to watch workers in real time in separate tmux panes. Omit `--server-url`. Requires `tmux` installed.

```bash
node /Users/bjohns/projects/tinycode/node_modules/oh-my-tiny/dist/team/coordinator.js \
  --team <team-name> \
  --workers <count> \
  --task "<task description>" \
  --cwd <project-directory>
```

After launching, attach to the session to observe workers:

```bash
tmux attach -t <team-name>
```

---

## Thin wrapper script

A convenience wrapper at `script/team.ts` injects tinycode defaults automatically:

```bash
bun script/team.ts --team <name> --workers <n> --task "<desc>"
```

The wrapper automatically adds `--server-url http://localhost:4096` and `--cwd <current directory>` unless you override them. You can override `--server-url` via the `TINYCODE_SERVER_URL` environment variable.

---

## How to invoke via the bash tool

Run the coordinator with the bash tool and monitor its stdout for progress and results. The coordinator prints worker output as it arrives. Wait for it to exit (exit code 0 = all workers succeeded, non-zero = at least one failed).

```bash
# Full invocation with all flags
node /Users/bjohns/projects/tinycode/node_modules/oh-my-tiny/dist/team/coordinator.js \
  --server-url http://localhost:4096 \
  --team my-team \
  --workers 2 \
  --task "Investigate and fix the two failing tests in packages/tinycode/test/session/" \
  --cwd /Users/bjohns/projects/tinycode
```

Or use the wrapper:

```bash
bun /Users/bjohns/projects/tinycode/script/team.ts \
  --team my-team \
  --workers 2 \
  --task "Investigate and fix the two failing tests in packages/tinycode/test/session/"
```
