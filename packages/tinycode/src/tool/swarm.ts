import { chmod, mkdir, writeFile } from "node:fs/promises"
import path from "node:path"
import { Effect, Schema } from "effect"
import { ChildProcess } from "effect/unstable/process"
import { ChildProcessSpawner } from "effect/unstable/process/ChildProcessSpawner"
import { InstanceState } from "@/effect/instance-state"
import { MessageV2 } from "@/session/message-v2"
import DESCRIPTION from "./swarm.txt"
import * as Tool from "./tool"

const DEFAULT_MODEL = "openrouter/z-ai/glm-5.2"
const DEFAULT_WORKERS = 4
const DEFAULT_STALE_SECONDS = 240
const DEFAULT_POLL_SECONDS = 30
const MAX_WORKERS = 8

export const Parameters = Schema.Struct({
  task: Schema.String.annotate({ description: "The user task for the swarm to solve" }),
  workers: Schema.optional(Schema.Int.check(Schema.isBetween({ minimum: 1, maximum: MAX_WORKERS }))).annotate({
    description: `Number of worker panes to launch. Defaults to ${DEFAULT_WORKERS}; max ${MAX_WORKERS}.`,
  }),
  model: Schema.optional(Schema.String).annotate({
    description: `Worker model in provider/model format. Defaults to ${DEFAULT_MODEL}.`,
  }),
  variant: Schema.optional(Schema.String).annotate({
    description: "Optional model variant/reasoning effort to pass to each worker.",
  }),
  session_name: Schema.optional(Schema.String).annotate({
    description: "Optional tmux session name. Unsafe characters are replaced with '-'.",
  }),
  shared_dir: Schema.optional(Schema.String).annotate({
    description: "Optional shared persistence directory. Relative paths resolve from the project worktree.",
  }),
  stale_seconds: Schema.optional(Schema.Int.check(Schema.isGreaterThan(0))).annotate({
    description: `Seconds without visible pane changes before the supervisor bumps a worker. Defaults to ${DEFAULT_STALE_SECONDS}.`,
  }),
  poll_seconds: Schema.optional(Schema.Int.check(Schema.isGreaterThan(0))).annotate({
    description: `Supervisor polling interval in seconds. Defaults to ${DEFAULT_POLL_SECONDS}.`,
  }),
  worker_command: Schema.optional(Schema.String).annotate({
    description:
      "Command prefix used to launch tinycode workers. Defaults to TINYCODE_SWARM_WORKER_CMD or 'tinycode'. Example: 'tiny --high'.",
  }),
  switch_client: Schema.optional(Schema.Boolean).annotate({
    description:
      "Switch the current tmux client to the swarm split-screen after launch. Defaults to true when running inside tmux unless TINYCODE_SWARM_SWITCH_CLIENT=0.",
  }),
  permissive: Schema.optional(Schema.Boolean).annotate({
    description: "When true, workers pass --dangerously-skip-permissions to tinycode run.",
  }),
})

type Params = Schema.Schema.Type<typeof Parameters>

type Worker = {
  id: string
  focus: string
  prompt: string
  script: string
}

type Plan = {
  sessionName: string
  task: string
  workerCount: number
  model: string
  variant?: string
  sharedDir: string
  projectDir: string
  staleSeconds: number
  pollSeconds: number
  workerCommand: string
  switchClient: boolean
  permissive: boolean
  workers: Worker[]
  supervisorScript: string
  dashboardScript: string
}

type Metadata = {
  sessionName: string
  sharedDir: string
  workerCount: number
  model: string
  supervisor: string
  dashboard: string
  attach: string
  panes: string[]
  switchClient: boolean
  switchedClient: boolean
}

const FOCUS = [
  "architecture, constraints, and decomposition",
  "implementation strategy and focused code changes",
  "tests, verification, and regression risk",
  "review, simplification, and edge cases",
  "documentation, user workflow, and commands",
  "dependency/API behavior and integration boundaries",
  "performance, reliability, and failure modes",
  "final synthesis and conflict resolution",
]

export function sanitizeSessionName(input: string) {
  return input
    .trim()
    .replace(/[^A-Za-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80)
}

export function shellQuote(input: string) {
  return "'" + input.replaceAll("'", "'\\''") + "'"
}

function defaultWorkerCommand() {
  if (process.env.TINYCODE_SWARM_WORKER_CMD) return process.env.TINYCODE_SWARM_WORKER_CMD
  const entry = process.argv[1]
  if (entry?.endsWith(path.join("src", "index.ts"))) {
    return `${shellQuote(process.execPath)} --conditions=browser ${shellQuote(entry)}`
  }
  if (entry && path.basename(entry) === "tinycode" && path.isAbsolute(entry)) return shellQuote(entry)
  return "tinycode"
}

function defaultSwitchClient(input: boolean | undefined) {
  if (input !== undefined) return input
  const override = process.env.TINYCODE_SWARM_SWITCH_CLIENT?.toLowerCase()
  if (override !== undefined) return !["0", "false", "no", "off"].includes(override)
  return Boolean(process.env.TMUX)
}

export function resolvePlan(input: {
  params: Params
  currentModel?: string
  currentVariant?: string
  projectDir: string
  now?: Date
}) {
  const now = input.now ?? new Date()
  const timestamp = now
    .toISOString()
    .replace(/[-:TZ.]/g, "")
    .slice(0, 14)
  const sessionName =
    sanitizeSessionName(input.params.session_name || `tiny-swarm-${timestamp}`) || `tiny-swarm-${timestamp}`
  const task = input.params.task.trim()
  if (!task) throw new Error("swarm task cannot be empty")
  const sharedDir = path.resolve(
    input.projectDir,
    input.params.shared_dir ?? path.join(".tinycode", "swarm", sessionName),
  )
  const planBase = {
    sessionName,
    task,
    workerCount: input.params.workers ?? DEFAULT_WORKERS,
    model: input.params.model ?? process.env.TINYCODE_SWARM_MODEL ?? input.currentModel ?? DEFAULT_MODEL,
    variant: input.params.variant ?? input.currentVariant,
    sharedDir,
    projectDir: input.projectDir,
    staleSeconds: input.params.stale_seconds ?? DEFAULT_STALE_SECONDS,
    pollSeconds: input.params.poll_seconds ?? DEFAULT_POLL_SECONDS,
    workerCommand: input.params.worker_command ?? defaultWorkerCommand(),
    switchClient: defaultSwitchClient(input.params.switch_client),
    permissive: input.params.permissive === true,
  }
  const workers = Array.from({ length: planBase.workerCount }, (_, index) => {
    const id = `worker-${String(index + 1).padStart(2, "0")}`
    return {
      id,
      focus: FOCUS[index] ?? `worker lane ${index + 1}`,
      prompt: workerPrompt(planBase, id, FOCUS[index] ?? `worker lane ${index + 1}`),
      script: "",
    }
  })
  const plan = {
    ...planBase,
    workers,
    supervisorScript: "",
    dashboardScript: "",
  }
  return {
    ...plan,
    workers: workers.map((worker) => ({
      ...worker,
      script: workerScript(plan, worker),
    })),
    supervisorScript: supervisorScript(plan),
    dashboardScript: dashboardScript(plan),
  } satisfies Plan
}

function workerPrompt(plan: Omit<Plan, "workers" | "supervisorScript" | "dashboardScript">, id: string, focus: string) {
  const workerDir = path.join(plan.sharedDir, "workers", id)
  return [
    `You are ${id} in tmux swarm ${plan.sessionName}.`,
    `Primary focus: ${focus}.`,
    "",
    "Task:",
    plan.task,
    "",
    "Shared persistent storage contract:",
    `- Shared root: ${plan.sharedDir}`,
    `- Your worker directory: ${workerDir}`,
    `- Your inbox: ${path.join(plan.sharedDir, "inbox", `${id}.md`)}`,
    `- Shared board: ${path.join(plan.sharedDir, "board.md")}`,
    "- Use the shared root for cross-worker coordination. Read the board and your inbox before major steps.",
    "- Before editing files, write a short claim under claims/ so workers avoid conflicting edits.",
    "- Update your worker status/heartbeat files after each meaningful step.",
    "- If a supervisor bump arrives, immediately write your blocker/next action to your status file and continue.",
    "",
    "Required files to write:",
    `1. ${path.join(workerDir, "status.md")} — current state, blocker, and next action.`,
    `2. ${path.join(workerDir, "heartbeat.md")} — timestamped progress notes.`,
    `3. ${path.join(plan.sharedDir, "results", `${id}.md`)} — final findings/result.`,
    `4. ${path.join(workerDir, "done.md")} — write this only when your lane is complete.`,
    `The runner writes ${path.join(workerDir, "exited.md")} automatically if your tinycode process exits.`,
    "",
    "Work autonomously. Coordinate through the shared files. Keep final user-facing text concise, but persist details in the shared storage.",
  ].join("\n")
}

function workerScript(
  plan: Omit<Plan, "workers" | "supervisorScript" | "dashboardScript">,
  worker: Omit<Worker, "script">,
) {
  const workerDir = path.join(plan.sharedDir, "workers", worker.id)
  const promptFile = path.join(workerDir, "prompt.md")
  const title = `${plan.sessionName} ${worker.id}`
  const args = [
    "run",
    "--interactive",
    "--model",
    plan.model,
    "--title",
    title,
    ...(plan.variant ? ["--variant", plan.variant] : []),
    ...(plan.permissive ? ["--dangerously-skip-permissions"] : []),
  ]
    .map(shellQuote)
    .join(" ")

  return [
    "#!/usr/bin/env bash",
    "set -u",
    `SHARED_DIR=${shellQuote(plan.sharedDir)}`,
    `WORKER_ID=${shellQuote(worker.id)}`,
    `WORKER_DIR=${shellQuote(workerDir)}`,
    `PROMPT_FILE=${shellQuote(promptFile)}`,
    `PROJECT_DIR=${shellQuote(plan.projectDir)}`,
    `WORKER_COMMAND=${shellQuote(plan.workerCommand)}`,
    'mkdir -p "$WORKER_DIR" "$SHARED_DIR/results"',
    "date -u '+%Y-%m-%dT%H:%M:%SZ starting' > \"$WORKER_DIR/heartbeat.md\"",
    'printf \'# %s status\\n\\nstarting\\n\' "$WORKER_ID" > "$WORKER_DIR/status.md"',
    'cd "$PROJECT_DIR" || exit 1',
    'echo "[$WORKER_ID] shared persistence: $SHARED_DIR"',
    'echo "[$WORKER_ID] starting tinycode worker pane; mark done.md when finished."',
    `cat "$PROMPT_FILE" | eval "$WORKER_COMMAND ${args}"`,
    "STATUS=$?",
    "date -u '+%Y-%m-%dT%H:%M:%SZ process exited' >> \"$WORKER_DIR/heartbeat.md\"",
    'printf \'worker process exited with status %s\\n\' "$STATUS" >> "$WORKER_DIR/status.md"',
    'printf \'worker process exited with status %s at %s\\n\' "$STATUS" "$(date -u \'+%Y-%m-%dT%H:%M:%SZ\')" > "$WORKER_DIR/exited.md"',
    "exit $STATUS",
    "",
  ].join("\n")
}

function supervisorScript(plan: Omit<Plan, "supervisorScript">) {
  const workerNames = plan.workers.map((worker) => shellQuote(worker.id)).join(" ")
  return [
    "#!/usr/bin/env bash",
    "set -u",
    `SESSION=${shellQuote(plan.sessionName)}`,
    `SHARED_DIR=${shellQuote(plan.sharedDir)}`,
    `STALE_SECONDS=${plan.staleSeconds}`,
    `POLL_SECONDS=${plan.pollSeconds}`,
    `WORKERS=(${workerNames})`,
    'LOG="$SHARED_DIR/supervisor/supervisor.log"',
    'mkdir -p "$SHARED_DIR/supervisor" "$SHARED_DIR/inbox"',
    'echo "$(date -u \'+%Y-%m-%dT%H:%M:%SZ\') supervisor started for $SESSION" | tee -a "$LOG"',
    "while true; do",
    "  all_done=1",
    "  now=$(date +%s)",
    '  for worker in "${WORKERS[@]}"; do',
    '    worker_dir="$SHARED_DIR/workers/$worker"',
    '    done_file="$worker_dir/done.md"',
    '    exited_file="$worker_dir/exited.md"',
    '    heartbeat="$worker_dir/heartbeat.md"',
    '    pane="$worker_dir/pane.log"',
    '    hash_file="$worker_dir/pane.cksum"',
    '    last_bump="$worker_dir/last-bump"',
    '    mkdir -p "$worker_dir"',
    '    target=$(tmux list-panes -t "$SESSION:swarm" -F "#{pane_title} #{pane_id}" 2>/dev/null | awk -v w="$worker" \'$1 == w { print $2; exit }\')',
    '    if [ -z "$target" ] || ! capture=$(tmux capture-pane -pt "$target" -S -200 2>/dev/null); then',
    '      if [ ! -f "$done_file" ] && [ ! -f "$exited_file" ]; then',
    "        printf 'worker pane disappeared at %s\\n' \"$(date -u '+%Y-%m-%dT%H:%M:%SZ')\" > \"$exited_file\"",
    '        echo "$(date -u \'+%Y-%m-%dT%H:%M:%SZ\') marked $worker exited because its tmux pane disappeared" | tee -a "$LOG"',
    "      fi",
    "      continue",
    "    fi",
    '    printf \'%s\\n\' "$capture" > "$pane"',
    "    hash=$(printf '%s' \"$capture\" | cksum | awk '{print $1 \"-\" $2}')",
    '    old_hash=$(cat "$hash_file" 2>/dev/null || true)',
    '    if [ "$hash" != "$old_hash" ]; then',
    '      printf \'%s\' "$hash" > "$hash_file"',
    '      date +%s > "$worker_dir/heartbeat.stamp"',
    "    fi",
    '    if [ -f "$done_file" ]; then',
    "      continue",
    "    fi",
    '    if [ -f "$exited_file" ]; then',
    "      continue",
    "    fi",
    "    all_done=0",
    '    stamp=$(cat "$worker_dir/heartbeat.stamp" 2>/dev/null || stat -f %m "$heartbeat" 2>/dev/null || stat -c %Y "$heartbeat" 2>/dev/null || echo 0)',
    "    age=$((now - stamp))",
    '    bumped_at=$(cat "$last_bump" 2>/dev/null || echo 0)',
    "    since_bump=$((now - bumped_at))",
    '    if [ "$age" -ge "$STALE_SECONDS" ] && [ "$since_bump" -ge "$STALE_SECONDS" ]; then',
    "      note=\"Supervisor bump $(date -u '+%Y-%m-%dT%H:%M:%SZ'): no visible progress for ${age}s. Read your inbox/status files, write your blocker and next action, then continue or mark done.\"",
    '      printf \'\\n## %s\\n%s\\n\' "$(date -u \'+%Y-%m-%dT%H:%M:%SZ\')" "$note" >> "$SHARED_DIR/inbox/$worker.md"',
    '      tmux send-keys -t "$target" "$note" Enter || true',
    '      date +%s > "$last_bump"',
    '      echo "$(date -u \'+%Y-%m-%dT%H:%M:%SZ\') bumped $worker after ${age}s stale" | tee -a "$LOG"',
    "    fi",
    "  done",
    '  if [ "$all_done" -eq 1 ]; then',
    '    echo "$(date -u \'+%Y-%m-%dT%H:%M:%SZ\') all workers done or exited" | tee -a "$LOG"',
    "    printf 'complete %s\\n' \"$(date -u '+%Y-%m-%dT%H:%M:%SZ')\" > \"$SHARED_DIR/supervisor/done\"",
    "    break",
    "  fi",
    '  sleep "$POLL_SECONDS"',
    "done",
    'echo "Supervisor finished. Shared files remain at: $SHARED_DIR"',
    "sleep 5",
    "exit 0",
    "",
  ].join("\n")
}

function dashboardScript(plan: Omit<Plan, "dashboardScript">) {
  const workerNames = plan.workers.map((worker) => shellQuote(worker.id)).join(" ")
  return [
    "#!/usr/bin/env bash",
    "set -u",
    `SESSION=${shellQuote(plan.sessionName)}`,
    `SHARED_DIR=${shellQuote(plan.sharedDir)}`,
    `WORKERS=(${workerNames})`,
    'DONE="$SHARED_DIR/supervisor/done"',
    "while true; do",
    "  clear",
    '  printf "tinycode swarm: %s\\n" "$SESSION"',
    '  printf "shared persistence: %s\\n\\n" "$SHARED_DIR"',
    '  printf "OMX-style split-screen: dashboard + supervisor + one pane per worker.\\n"',
    '  printf "Keys: ctrl-b q shows pane numbers · ctrl-b arrows moves panes · ctrl-b d detaches.\\n\\n"',
    '  echo "Panes:"',
    '  tmux list-panes -t "$SESSION:swarm" -F "  #{pane_index}: #{pane_title} #{pane_current_command}" 2>/dev/null || true',
    '  echo ""',
    '  echo "Workers:"',
    '  for worker in "${WORKERS[@]}"; do',
    '    worker_dir="$SHARED_DIR/workers/$worker"',
    '    state="running"',
    '    if [ -f "$worker_dir/done.md" ]; then',
    '      state="done"',
    '    elif [ -f "$worker_dir/exited.md" ]; then',
    '      state="exited"',
    "    fi",
    '    heartbeat=$(tail -n 1 "$worker_dir/heartbeat.md" 2>/dev/null || echo "no heartbeat yet")',
    '    status=$(grep -v "^#" "$worker_dir/status.md" 2>/dev/null | tail -n 1 || true)',
    '    printf "  %-10s %-8s %s\\n" "$worker" "$state" "$heartbeat"',
    '    if [ -n "$status" ]; then',
    '      printf "    status: %s\\n" "$status"',
    "    fi",
    "  done",
    '  echo ""',
    '  echo "Supervisor log:"',
    '  tail -n 12 "$SHARED_DIR/supervisor/supervisor.log" 2>/dev/null || echo "  waiting for supervisor log..."',
    '  echo ""',
    '  echo "Attach from another shell: tmux attach -t $SESSION"',
    '  if [ -f "$DONE" ]; then',
    '    echo ""',
    '    echo "Swarm complete. Closing dashboard in 10 seconds. Shared files remain at: $SHARED_DIR"',
    "    sleep 10",
    "    exit 0",
    "  fi",
    "  sleep 2",
    "done",
    "",
  ].join("\n")
}

/** @internal exported for race-free filesystem setup tests. */
export async function writePlan(plan: Plan) {
  await mkdir(plan.sharedDir, { recursive: true })
  await Promise.all([
    mkdir(path.join(plan.sharedDir, "workers"), { recursive: true }),
    mkdir(path.join(plan.sharedDir, "inbox"), { recursive: true }),
    mkdir(path.join(plan.sharedDir, "results"), { recursive: true }),
    mkdir(path.join(plan.sharedDir, "claims"), { recursive: true }),
    mkdir(path.join(plan.sharedDir, "supervisor"), { recursive: true }),
    mkdir(path.join(plan.sharedDir, "dashboard"), { recursive: true }),
  ])
  await writeFile(
    path.join(plan.sharedDir, "manifest.json"),
    JSON.stringify(
      {
        sessionName: plan.sessionName,
        task: plan.task,
        model: plan.model,
        variant: plan.variant,
        workers: plan.workers.map((worker) => ({ id: worker.id, focus: worker.focus })),
        sharedDir: plan.sharedDir,
        projectDir: plan.projectDir,
        staleSeconds: plan.staleSeconds,
        pollSeconds: plan.pollSeconds,
        switchClient: plan.switchClient,
        permissive: plan.permissive,
        startedAt: new Date().toISOString(),
      },
      null,
      2,
    ),
  )
  await writeFile(
    path.join(plan.sharedDir, "board.md"),
    [
      `# Swarm ${plan.sessionName}`,
      "",
      "## Task",
      plan.task,
      "",
      "## Coordination",
      "- Workers: write claims before conflicting edits.",
      "- Supervisor: appends stale nudges to inbox/*.md and sends them to worker panes.",
      "- Dashboard: attach or switch to the tmux split-screen to see the OMX-style worker overview.",
      "- Completion: each worker writes workers/<id>/done.md and results/<id>.md; the runner writes workers/<id>/exited.md if the process exits.",
      "",
    ].join("\n"),
  )
  await Promise.all(
    plan.workers.map(async (worker) => {
      const dir = path.join(plan.sharedDir, "workers", worker.id)
      const script = path.join(dir, "run.sh")
      await mkdir(dir, { recursive: true })
      await Promise.all([
        writeFile(path.join(dir, "prompt.md"), worker.prompt),
        writeFile(path.join(plan.sharedDir, "inbox", `${worker.id}.md`), `# Inbox for ${worker.id}\n`),
        writeFile(script, worker.script).then(() => chmod(script, 0o755)),
      ])
    }),
  )
  await writeFile(path.join(plan.sharedDir, "supervisor", "run.sh"), plan.supervisorScript)
  await chmod(path.join(plan.sharedDir, "supervisor", "run.sh"), 0o755)
  await writeFile(path.join(plan.sharedDir, "dashboard", "run.sh"), plan.dashboardScript)
  await chmod(path.join(plan.sharedDir, "dashboard", "run.sh"), 0o755)
}

function currentModel(message: MessageV2.WithParts | undefined): string | undefined {
  if (!message || message.info.role !== "assistant") return undefined
  return `${message.info.providerID}/${message.info.modelID}`
}

function currentVariant(message: MessageV2.WithParts | undefined): string | undefined {
  if (!message || message.info.role !== "assistant") return undefined
  return message.info.variant
}

export const SwarmTool = Tool.define<typeof Parameters, Metadata, ChildProcessSpawner>(
  "swarm",
  Effect.gen(function* () {
    const spawner = yield* ChildProcessSpawner

    const runTmux = Effect.fn("SwarmTool.tmux")(function* (args: string[], cwd: string) {
      const handle = yield* spawner.spawn(ChildProcess.make("tmux", args, { cwd, stdin: "ignore", extendEnv: true }))
      const code = yield* handle.exitCode
      if (code !== 0) throw new Error(`tmux ${args.join(" ")} failed with exit ${code}`)
    })

    const tmuxExit = Effect.fn("SwarmTool.tmuxExit")(function* (args: string[], cwd: string) {
      const handle = yield* spawner.spawn(ChildProcess.make("tmux", args, { cwd, stdin: "ignore", extendEnv: true }))
      return yield* handle.exitCode
    })

    return {
      description: DESCRIPTION,
      parameters: Parameters,
      execute: (params: Params, ctx: Tool.Context<Metadata>) =>
        Effect.scoped(
          Effect.gen(function* () {
            const instance = yield* InstanceState.context
            const message = yield* MessageV2.get({ sessionID: ctx.sessionID, messageID: ctx.messageID }).pipe(
              Effect.catchCause(() => Effect.succeed(undefined)),
            )
            const plan = resolvePlan({
              params,
              currentModel: currentModel(message),
              currentVariant: currentVariant(message),
              projectDir: instance.worktree === "/" ? instance.directory : instance.worktree,
            })

            yield* ctx.ask({
              permission: "swarm",
              patterns: [plan.sessionName, plan.sharedDir],
              always: [plan.sessionName],
              metadata: {
                sessionName: plan.sessionName,
                sharedDir: plan.sharedDir,
                workers: plan.workerCount,
                model: plan.model,
                permissive: plan.permissive,
                switchClient: plan.switchClient,
              },
            })

            yield* runTmux(["-V"], plan.projectDir)
            const exists = yield* tmuxExit(["has-session", "-t", plan.sessionName], plan.projectDir)
            if (exists === 0) throw new Error(`tmux session already exists: ${plan.sessionName}`)

            yield* Effect.promise(() => writePlan(plan))
            yield* runTmux(
              [
                "new-session",
                "-d",
                "-s",
                plan.sessionName,
                "-n",
                "swarm",
                shellQuote(path.join(plan.sharedDir, "dashboard", "run.sh")),
              ],
              plan.projectDir,
            )
            yield* runTmux(["select-pane", "-t", `${plan.sessionName}:swarm`, "-T", "dashboard"], plan.projectDir)
            yield* runTmux(
              [
                "split-window",
                "-h",
                "-t",
                `${plan.sessionName}:swarm`,
                shellQuote(path.join(plan.sharedDir, "supervisor", "run.sh")),
              ],
              plan.projectDir,
            )
            yield* runTmux(["select-pane", "-t", `${plan.sessionName}:swarm`, "-T", "supervisor"], plan.projectDir)
            for (const worker of plan.workers) {
              yield* runTmux(
                [
                  "split-window",
                  "-v",
                  "-t",
                  `${plan.sessionName}:swarm`,
                  shellQuote(path.join(plan.sharedDir, "workers", worker.id, "run.sh")),
                ],
                plan.projectDir,
              )
              yield* runTmux(["select-pane", "-t", `${plan.sessionName}:swarm`, "-T", worker.id], plan.projectDir)
            }
            yield* runTmux(["select-layout", "-t", `${plan.sessionName}:swarm`, "tiled"], plan.projectDir)
            yield* tmuxExit(["select-pane", "-t", `${plan.sessionName}:swarm.0`], plan.projectDir)
            const switchExit = plan.switchClient
              ? yield* tmuxExit(["switch-client", "-t", `${plan.sessionName}:swarm`], plan.projectDir)
              : 1

            const attach = `tmux attach -t ${plan.sessionName}`
            const panes = ["dashboard", "supervisor", ...plan.workers.map((worker) => worker.id)]
            const switchedClient = switchExit === 0
            return {
              title: `swarm ${plan.sessionName}`,
              metadata: {
                sessionName: plan.sessionName,
                sharedDir: plan.sharedDir,
                workerCount: plan.workerCount,
                model: plan.model,
                supervisor: path.join(plan.sharedDir, "supervisor", "run.sh"),
                dashboard: path.join(plan.sharedDir, "dashboard", "run.sh"),
                attach,
                panes,
                switchClient: plan.switchClient,
                switchedClient,
              },
              output: [
                `Swarm launched: ${plan.sessionName}`,
                `Workers: ${plan.workerCount}`,
                `Model: ${plan.model}${plan.variant ? ` (${plan.variant})` : ""}`,
                `Shared persistence: ${plan.sharedDir}`,
                `Tmux panes: ${panes.join(", ")}`,
                `Dashboard: ${path.join(plan.sharedDir, "dashboard", "run.sh")}`,
                `Supervisor: ${path.join(plan.sharedDir, "supervisor", "run.sh")}`,
                `Attach: ${attach}`,
                `Pane list: tmux list-panes -t ${plan.sessionName}:swarm`,
                `Switched current tmux client: ${
                  switchedClient
                    ? "yes"
                    : plan.switchClient
                      ? "no (attach manually if this session was not launched inside tmux)"
                      : "no"
                }`,
                "",
                "Supervisor behavior:",
                `- Captures each worker pane every ${plan.pollSeconds}s`,
                `- Bumps workers after ${plan.staleSeconds}s without visible progress`,
                "- Appends bumps to inbox/<worker>.md and sends them into the worker tmux pane",
                "- Uses one tmux window named 'swarm' with a tiled split-screen layout",
              ].join("\n"),
            }
          }),
        ).pipe(Effect.orDie),
    } satisfies Tool.DefWithoutID<typeof Parameters, Metadata>
  }),
)
