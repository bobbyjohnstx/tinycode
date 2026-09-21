package command

import "strings"

// SwarmPrefix is the instruction preamble prepended to /swarm prompts.
const SwarmPrefix = `You are in SWARM mode. You MUST delegate ALL work to subagents using the "task" tool. Do NOT use bash, read, write, edit, or any other tool directly — ONLY the task tool.

STEP 1: Analyze the user's task and split it into 2-4 independent subtasks.
STEP 2: Call the "task" tool once for EACH subtask. Make ALL task calls in a SINGLE response so they run in parallel.
STEP 3: After all tasks return results, write a final synthesis report.

TASK TOOL FORMAT — each call must include:
  "description": short label (e.g. "Batch 1: files A-G")
  "prompt": detailed instructions for the subagent — tell it exactly what to do and what to return
  "subagent_type": "executor" (for running commands) or "explore" (for reading/searching)

CONSTRAINTS:
- Do NOT set "background": true — use foreground mode
- Do NOT do the work yourself — you are the coordinator, subagents do the work
- Do NOT call bash or any file tool — ONLY the task tool
- Make MULTIPLE task calls in ONE response to run them in parallel
- Each subagent has its own tools (bash, read, etc.) and will do the actual work

EXAMPLE — if the user says "run tests on 3 packages":
Call task 3 times in one response:
  task(description="Test pkg/a", prompt="Run go test ./pkg/a/... and report results", subagent_type="executor")
  task(description="Test pkg/b", prompt="Run go test ./pkg/b/... and report results", subagent_type="executor")
  task(description="Test pkg/c", prompt="Run go test ./pkg/c/... and report results", subagent_type="executor")

USER TASK:
`

// WorkLoopPrefix is the instruction preamble prepended to /work-loop prompts.
const WorkLoopPrefix = `You are in WORK-LOOP mode. Iterate on the user's task until it is complete or you are blocked.

PROTOCOL:
1. Understand: Read the task. Check relevant files.
2. Plan: Identify the single most impactful next action. State it in one sentence.
3. Act: Execute the action using available tools.
4. Verify: Confirm the action worked (run tests, read the file, check output).
5. Assess:
   - If the task is complete → write a brief summary and STOP.
   - If more work remains → return to step 2.
   - If blocked (same action failed 3 times) → explain the blocker and STOP.

Do NOT ask for confirmation between iterations. Keep going until done or blocked.

USER TASK:
`

// ExpandResult holds the expanded prompt text and any flags signaled by the
// slash command (e.g. /swarm implies auto-approve).
type ExpandResult struct {
	Text        string
	AutoApprove bool
}

// ExpandSlashCommand detects /swarm and /work-loop prefixes and prepends
// instruction text so the LLM knows how to execute the command.
func ExpandSlashCommand(text string) ExpandResult {
	trimmed := strings.TrimSpace(text)

	if strings.HasPrefix(trimmed, "/swarm ") {
		userTask := strings.TrimSpace(strings.TrimPrefix(trimmed, "/swarm"))
		return ExpandResult{Text: SwarmPrefix + userTask, AutoApprove: true}
	}
	if strings.HasPrefix(trimmed, "/work-loop ") {
		userTask := strings.TrimSpace(strings.TrimPrefix(trimmed, "/work-loop"))
		return ExpandResult{Text: WorkLoopPrefix + userTask}
	}
	return ExpandResult{Text: text}
}
