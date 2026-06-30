import { describe, expect, test } from "bun:test"
import { writeSessionOutput, traceSubagentState, traceFooterOutput } from "@/cli/cmd/run/stream"
import type { FooterApi, FooterOutput, StreamCommit, FooterSubagentState } from "@/cli/cmd/run/types"

describe("stream formatting", () => {
  describe("writeSessionOutput", () => {
    test("forwards commits to footer.append", () => {
      const appended: StreamCommit[] = []
      const footer: FooterApi = {
        append: (commit) => appended.push(commit),
        event: () => {},
      }

      const commits: StreamCommit[] = [
        { text: "Hello", part: undefined },
        { text: "World", part: undefined },
      ]

      writeSessionOutput({ footer }, { commits })

      expect(appended.length).toBe(2)
      expect(appended[0].text).toBe("Hello")
      expect(appended[1].text).toBe("World")
    })

    test("defaults status phase to 'running' when not specified", () => {
      const events: any[] = []
      const footer: FooterApi = {
        append: () => {},
        event: (event) => events.push(event),
      }

      const output: FooterOutput = {
        patch: { status: "Working on it" },
      }

      writeSessionOutput({ footer }, { commits: [], footer: output })

      expect(events.length).toBe(1)
      expect(events[0].type).toBe("stream.patch")
      expect(events[0].patch.status).toBe("Working on it")
      expect(events[0].patch.phase).toBe("running")
    })

    test("preserves explicit phase", () => {
      const events: any[] = []
      const footer: FooterApi = {
        append: () => {},
        event: (event) => events.push(event),
      }

      const output: FooterOutput = {
        patch: { status: "Done", phase: "complete" },
      }

      writeSessionOutput({ footer }, { commits: [], footer: output })

      expect(events[0].patch.phase).toBe("complete")
    })

    test("forwards subagent state to footer.event", () => {
      const events: any[] = []
      const footer: FooterApi = {
        append: () => {},
        event: (event) => events.push(event),
      }

      const subagentState: FooterSubagentState = {
        tabs: ["session1"],
        details: {},
        permissions: [],
        questions: [],
      }

      const output: FooterOutput = {
        subagent: subagentState,
      }

      writeSessionOutput({ footer }, { commits: [], footer: output })

      expect(events.length).toBe(1)
      expect(events[0].type).toBe("stream.subagent")
      expect(events[0].state.tabs).toEqual(["session1"])
    })

    test("forwards view updates to footer.event", () => {
      const events: any[] = []
      const footer: FooterApi = {
        append: () => {},
        event: (event) => events.push(event),
      }

      const output: FooterOutput = {
        view: "diff",
      }

      writeSessionOutput({ footer }, { commits: [], footer: output })

      expect(events.length).toBe(1)
      expect(events[0].type).toBe("stream.view")
      expect(events[0].view).toBe("diff")
    })

    test("writes trace for commits when trace is provided", () => {
      const traces: Array<{ type: string; data: any }> = []
      const trace = {
        write: (type: string, data: unknown) => traces.push({ type, data }),
      }

      const footer: FooterApi = {
        append: () => {},
        event: () => {},
      }

      const commits: StreamCommit[] = [{ text: "Test", part: undefined }]

      writeSessionOutput({ footer, trace }, { commits })

      expect(traces.length).toBe(1)
      expect(traces[0].type).toBe("ui.commit")
    })
  })

  describe("traceSubagentState", () => {
    test("summarizes subagent state for tracing", () => {
      const state: FooterSubagentState = {
        tabs: ["session1", "session2"],
        details: {
          session1: {
            commits: [
              {
                text: "Test commit",
                part: undefined,
              },
            ],
          },
        },
        permissions: [
          {
            id: "perm1",
            sessionID: "session1",
            permission: "read",
            patterns: ["*.ts"],
            tool: "read",
            metadata: {
              input: { path: "/test/file.ts" },
            },
          },
        ],
        questions: [
          {
            id: "q1",
            sessionID: "session1",
            questions: [
              {
                header: "Confirm",
                question: "Proceed?",
                options: ["Yes", "No"],
                multiple: false,
              },
            ],
          },
        ],
      }

      const traced = traceSubagentState(state)

      expect(traced.tabs).toEqual(["session1", "session2"])
      expect(traced.details.session1.commits.length).toBe(1)
      expect(traced.permissions.length).toBe(1)
      expect(traced.permissions[0].metadata?.keys).toEqual(["input"])
      expect(traced.questions.length).toBe(1)
      expect(traced.questions[0].questions[0].options).toBe(2)
    })

    test("truncates long strings in commit text", () => {
      const longText = "a".repeat(200)
      const state: FooterSubagentState = {
        tabs: [],
        details: {
          session1: {
            commits: [
              {
                text: longText,
                part: undefined,
              },
            ],
          },
        },
        permissions: [],
        questions: [],
      }

      const traced = traceSubagentState(state)
      const commit = traced.details.session1.commits[0]

      expect(typeof commit.text).toBe("object")
      if (typeof commit.text === "object" && commit.text !== null) {
        expect((commit.text as any).type).toBe("string")
        expect((commit.text as any).length).toBe(200)
        expect((commit.text as any).preview).toBe("a".repeat(160) + "...")
      }
    })
  })

  describe("traceFooterOutput", () => {
    test("returns input unchanged when no subagent state", () => {
      const footer: FooterOutput = {
        patch: { status: "Running" },
      }

      const traced = traceFooterOutput(footer)
      expect(traced).toEqual(footer)
    })

    test("traces subagent state when present", () => {
      const footer: FooterOutput = {
        subagent: {
          tabs: ["session1"],
          details: {},
          permissions: [],
          questions: [],
        },
      }

      const traced = traceFooterOutput(footer)
      expect(traced?.subagent).toBeDefined()
      expect(traced?.subagent?.tabs).toEqual(["session1"])
    })

    test("handles undefined footer", () => {
      const traced = traceFooterOutput(undefined)
      expect(traced).toBeUndefined()
    })
  })
})
