import { describe, expect, test } from "bun:test"
import { Schema } from "effect"
import path from "node:path"
import { Parameters, resolvePlan, sanitizeSessionName, shellQuote, writePlan } from "@/tool/swarm"
import { tmpdir } from "../fixture/fixture"

const decode = Schema.decodeUnknownSync(Parameters)

describe("tool.swarm", () => {
  test("parameters accept minimal swarm request", () => {
    const result = decode({ task: "review the repo" })
    expect(result.task).toBe("review the repo")
    expect(result.workers).toBeUndefined()
  })

  test("parameters cap worker count", () => {
    expect(() => decode({ task: "x", workers: 9 })).toThrow()
    expect(() => decode({ task: "x", workers: 0 })).toThrow()
  })

  test("sanitizes tmux session names", () => {
    expect(sanitizeSessionName("  tiny swarm: GLM 5.2!  ")).toBe("tiny-swarm-GLM-5-2")
  })

  test("falls back when sanitized tmux session names are empty", () => {
    const plan = resolvePlan({
      params: decode({ task: "review", session_name: "!!!", worker_command: "tinycode" }),
      projectDir: "/tmp/project",
      now: new Date("2026-06-23T01:02:03.000Z"),
    })

    expect(plan.sessionName).toBe("tiny-swarm-20260623010203")
  })

  test("quotes shell strings safely", () => {
    expect(shellQuote("it's ok")).toBe("'it'\\''s ok'")
  })

  test("rejects empty swarm tasks", () => {
    expect(() =>
      resolvePlan({
        params: decode({ task: "   ", worker_command: "tinycode" }),
        projectDir: "/tmp/project",
        now: new Date("2026-06-23T00:00:00.000Z"),
      }),
    ).toThrow("swarm task cannot be empty")
  })

  test("builds shared persistence and supervisor bump plan", () => {
    const prior = process.env.TINYCODE_SWARM_MODEL
    delete process.env.TINYCODE_SWARM_MODEL
    try {
      const plan = resolvePlan({
        params: decode({
          task: "fix the build",
          workers: 2,
          session_name: "demo swarm",
          model: "openrouter/z-ai/glm-5.2",
          worker_command: "tinycode",
          switch_client: false,
          permissive: true,
        }),
        projectDir: "/tmp/project",
        now: new Date("2026-06-23T00:00:00.000Z"),
      })

      expect(plan.sessionName).toBe("demo-swarm")
      expect(plan.sharedDir).toBe("/tmp/project/.tinycode/swarm/demo-swarm")
      expect(plan.workers).toHaveLength(2)
      expect(plan.workers[0]?.prompt).toContain("Shared persistent storage contract")
      expect(plan.workers[0]?.prompt).toContain("/tmp/project/.tinycode/swarm/demo-swarm")
      expect(plan.workers[0]?.script).toContain("--dangerously-skip-permissions")
      expect(plan.workers[0]?.script).toContain("exited.md")
      expect(plan.dashboardScript).toContain("tmux list-panes")
      expect(plan.dashboardScript).toContain("OMX-style split-screen")
      expect(plan.supervisorScript).toContain("tmux send-keys")
      expect(plan.supervisorScript).toContain("$SHARED_DIR/inbox/$worker.md")
      expect(plan.supervisorScript).toContain("worker pane disappeared")
      expect(plan.supervisorScript).toContain("STALE_SECONDS=240")
      expect(plan.switchClient).toBe(false)
    } finally {
      if (prior === undefined) delete process.env.TINYCODE_SWARM_MODEL
      else process.env.TINYCODE_SWARM_MODEL = prior
    }
  })

  test("writes worker files after creating worker directories", async () => {
    await using tmp = await tmpdir()
    const plan = resolvePlan({
      params: decode({
        task: "write plan smoke",
        workers: 3,
        session_name: "write plan smoke",
        worker_command: "tinycode",
        switch_client: false,
      }),
      projectDir: tmp.path,
      now: new Date("2026-06-23T00:00:00.000Z"),
    })

    await writePlan(plan)

    for (const worker of plan.workers) {
      expect(await Bun.file(path.join(plan.sharedDir, "workers", worker.id, "prompt.md")).exists()).toBe(true)
      expect(await Bun.file(path.join(plan.sharedDir, "workers", worker.id, "run.sh")).exists()).toBe(true)
      expect(await Bun.file(path.join(plan.sharedDir, "inbox", `${worker.id}.md`)).exists()).toBe(true)
    }
    expect(await Bun.file(path.join(plan.sharedDir, "dashboard", "run.sh")).exists()).toBe(true)
    expect(await Bun.file(path.join(plan.sharedDir, "supervisor", "run.sh")).exists()).toBe(true)
  })
})
