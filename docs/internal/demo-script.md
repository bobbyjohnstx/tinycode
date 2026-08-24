# tinycode Demo Script

**Total Runtime:** 4m 30s (full) | 2m cut available | **6 Scenes**
**Story:** Zero to productive AI coding without an internet connection.
**Hardware:** Record on M1/M2 MacBook for realistic local inference speed.
**Model:** qwen3.5:9b (scored 14/15 Full Agentic in benchmark)

---

## Production Rules

1. **No cuts during model generation.** The moment you cut, the viewer assumes you're hiding latency or failure. Show the thinking spinner honestly.
2. **No cherry-picked runs.** One imperfect run with visible recovery is more credible than one perfect run that took 15 takes.
3. **No "hello world" tasks.** Work on an existing codebase with real files, not a contrived scaffold.
4. **Show, don't tell.** Every claim has a visible action backing it. Privacy isn't a bullet point — it's a network monitor showing zero traffic.
5. **Design pauses.** Local inference has thinking time. Use those moments to explain what's happening architecturally, or let the viewer read the tool calls.

---

## SCENE 1: Cold Open — Install (20s)

**Mode:** VIDEO
**Setup:** Empty terminal, clean macOS desktop. Wi-Fi icon visible in menu bar.

**Action:**
```bash
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/dev/install.sh | sh
tinycode --version
```

**Shot notes:** Full-screen terminal. Capture curl output, checkmarks as script installs. Brief pause on version output.

**Narration:** "tinycode installs in one line. No sign-up, no API keys, no cloud account. It runs on your machine."

**On-screen text:** `One-line install — no account required`

---

## SCENE 2: First Launch + Airplane Mode (50s)

**Mode:** VIDEO
**Setup:** Ollama running with qwen3.5:9b pulled. Wi-Fi still visible.

**Action:**
1. **Toggle airplane mode** (or visibly turn off Wi-Fi) — the airplane icon appears in the menu bar
2. Run: `tinycode .` against a sample TypeScript project
3. TUI starts. Status bar shows Ollama auto-discovered at localhost:11434
4. Model name `qwen3.5:9b` appears — no configuration file was touched
5. Toast shows "Warming qwen3.5:9b..." → "qwen3.5:9b ready — tool calling supported"
6. Pause 3s on the TUI showing the ready state

**Shot notes:** Zoom on airplane-mode icon. Then zoom on status bar as Ollama is discovered. The juxtaposition is the point — offline, yet fully functional.

**Narration:** "Wi-Fi is off. tinycode auto-discovers your local Ollama instance and the models you've already pulled. No config file. No internet. Let's write some code."

**On-screen text:** `Offline` | `Ollama auto-discovered` | `qwen3.5:9b`

> **Why this works:** The analyst identified airplane mode as "the single most memorable demo moment — no other AI coding tool can do this." The critic agreed: lead with what they can't get elsewhere. Combining install + offline in the first 70s establishes the core value proposition immediately.

---

## SCENE 3: Real Bug Fix — Unbroken (75s)

**Mode:** VIDEO (single unbroken take, no cuts)
**Setup:** TUI with a TypeScript project that has a failing test. Build agent active.

**Action:**
1. User types: `The tests are failing. Fix the bug.`
2. Model thinks (show spinner for 3-5 seconds — honest wait time)
3. Model calls `bash` to run tests — show the tool-approval prompt, user approves
4. Test output streams showing the failure
5. Model calls `read` on the source file
6. Model identifies the bug, calls `edit` to fix it
7. Model calls `bash` to re-run tests — they pass
8. Total elapsed: ~75s of real-time inference

**Shot notes:** Full TUI, no split screen needed. The tool calls appear inline — read, edit, bash are all visible. Do NOT cut during the thinking spinner. Do NOT speed up inference. If the model takes 5s to think, the viewer sees 5s of thinking.

**Narration:** "We ask tinycode to fix a failing test. Watch what happens — it runs the tests, reads the failure, finds the bug in the source, fixes it, and verifies. This is real-time. No speed-up, no editing. A 9-billion parameter model running on this laptop."

**On-screen text:** Overlay token/s counter if available, or model name + parameter count

> **Why this works:** The critic's #1 rule: "Show a real task on a real codebase completing end-to-end without a single cut." This scene IS the demo. Everything else is context.

---

## SCENE 4: Agent Switching Mid-Session (55s)

**Mode:** VIDEO
**Setup:** Same TUI, same session. Bug is fixed.

**Action:**
1. Press `Tab` to switch from `build` to `debugger` — show the agent label change in status bar
2. Type: `Are there other places in this codebase with the same pattern?`
3. Debugger greps the codebase, finds similar patterns, reports with `file:line` citations
4. Press `Tab` to switch to `executor`
5. Type: `Fix them all.`
6. Executor applies minimal diffs, runs tests

**Shot notes:** Close-up on the status bar during Tab transitions. The agent name changing is the visual anchor. Show the debugger's structured output (Root Cause / Fix / Similar Issues format).

**Narration:** "Switch agents on the fly. The debugger traces patterns across the codebase. The executor applies the fixes. Same model, same conversation, different focus. Twenty-four agents available — pick the right tool for the moment."

**On-screen text:** `Tab: build → debugger → executor`

> **Why this works:** The architect identified this as the "multi-agent moment." The key is showing it as a natural workflow, not a feature tour — the user switches agents because the TASK changed, not to show off.

---

## SCENE 5: Plan Mode — Hard Permission Enforcement (40s)

**Mode:** VIDEO
**Setup:** Same session. Switch to plan mode.

**Action:**
1. Type `/plan`
2. Agent switches to plan mode — label changes
3. Ask: `How should we restructure the error handling in this project?`
4. Model reads files freely, explores the codebase
5. Model tries to edit a file — **tool rejects it** with a permission error
6. Model writes a structured plan to `.tinycode/plans/`
7. Exit plan mode, approve, build executes

**Shot notes:** The permission rejection is the surprise moment. Zoom on the error message briefly. Then show the plan file being written to the correct location.

**Narration:** "Plan mode is read-only. Not as a suggestion — as a hard enforcement. The model can explore everything but can't change a single file. It writes a plan instead. When you're ready, approve it and let build execute."

**On-screen text:** `Plan mode: read-only enforced` | `Edit blocked`

> **Why this works:** The architect called this the "unexpected capability moment" — viewers expect read-only to be a prompt instruction that the model can ignore. Showing it as a structural enforcement surprises developers who've been burned by unreliable guardrails.

---

## SCENE 6: Platform Pivot + Privacy Proof (50s)

**Mode:** SPLIT (TUI left, browser right), then VIDEO
**Setup:** Same session running.

**Action:**
1. Run `tinycode web` — browser opens to localhost:4096
2. Split screen: TUI left, web UI right, same conversation visible in both
3. Type a prompt in the web UI — response appears in both interfaces
4. Quick cut to Electron desktop app (2s beauty shot with system tray visible)
5. **Privacy proof:** Show `lsof -i -P | grep -v localhost` returning empty — zero outbound connections
6. Final frame: back to TUI. Toggle session tree with `<leader>b` showing the full session hierarchy

**Shot notes:** The split screen shows platform flexibility. The `lsof` output is the visceral privacy proof — not a claim, evidence.

**Narration:** "Same session in the browser. Same session on the desktop. Four interfaces, one server, your choice. And here's the proof — zero outbound network connections. Your code and your prompts never leave your machine."

**Closing text:** `Your code. Your models. Your machine.`

---

## 2-Minute Cut

For social media / README embed, use:
- Scene 2 (airplane mode + auto-discovery): 20s condensed
- Scene 3 (unbroken bug fix): 60s — the hero moment, do NOT cut this
- Scene 6 (privacy proof only): 15s — just the `lsof` output
- Closing card: 5s

Total: ~100s. This proves three things: it works offline, it does real coding, nothing leaves your machine.

---

## Screenshots (for README, docs, website)

These don't need video — static captures work:

| Shot | What to capture | Where to use |
|------|----------------|--------------|
| TUI overview | Full TUI with conversation, status bar, agent label | README hero image |
| Web UI | Browser at localhost:4096 with conversation | README, docs |
| Desktop app | Electron with system tray visible | README |
| Agent picker | `<leader>a` agent selection overlay | Agent docs |
| Plan file | `.tinycode/plans/` output in an editor | Plan mode docs |
| Config file | 3-line config.json (no secrets) | Getting started guide |
| Session tree | `<leader>b` sidebar showing session hierarchy | Feature docs |
| Context compaction | Token count dropping, conversation continuing | Architecture docs |

---

## Audience Notes

| Audience | What they care about | Scene that wins them |
|----------|---------------------|---------------------|
| Privacy-conscious dev | "Nothing leaves my machine" | Scene 2 (airplane mode) + Scene 6 (lsof proof) |
| Enterprise/regulated | Air-gapped deployment, audit trail | Scene 5 (plan mode enforcement) |
| OSS contributor | Architecture, extensibility, agent system | Scene 4 (agent switching) |
| Claude Code/Cursor evaluator | Can it actually code? | Scene 3 (unbroken bug fix) |

---

## Anti-Patterns Checklist

Before recording, verify:
- [ ] No cuts during model generation
- [ ] No speed-up of inference
- [ ] No "hello world" or scaffold task — use a real project
- [ ] Thinking pauses are visible and not narrated over nervously
- [ ] At least one model mistake + recovery shown (if it happens naturally — don't stage it)
- [ ] Token/s or model size displayed on screen
- [ ] No comparison to Claude Code or Cursor — different category, not cheaper clone
- [ ] `lsof` or network monitor proof included, not just a verbal claim
- [ ] Recorded on consumer hardware (MacBook), not a GPU server
