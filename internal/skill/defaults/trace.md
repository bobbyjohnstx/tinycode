---
name: trace
description: Evidence-driven causal tracing with competing hypotheses, ranked evidence, and discriminating probes
---

# Trace

Use this skill for ambiguous, causal, evidence-heavy questions where the goal is to explain why an observed result happened, not to jump directly into fixing or rewriting code.

## When to Use

Use this skill when:
- Two or more genuinely different explanations could account for the observation AND you cannot yet determine which is correct from available evidence
- The problem is causal: you need to explain why an observed result happened, not just fix it
- The failure is ambiguous enough that a single-lane diagnosis would miss real alternatives
- The analysis benefits from parallel evidence-gathering across competing hypotheses

## When Not to Use

- The root cause is already known and the task is to implement the fix
- The goal is to confirm a change works — use `verify`
- A single most-likely cause exists and competing hypotheses add no value — use `debug`
- The failure is a straightforward error message with a single obvious cause

## Core tracing contract

Always preserve these distinctions:

1. **Observation** — what was actually observed
2. **Hypotheses** — competing explanations
3. **Evidence For** — what supports each explanation
4. **Evidence Against / Gaps** — what contradicts it or is still missing
5. **Current Best Explanation** — the leading explanation right now
6. **Critical Unknown** — the missing fact keeping the top explanations apart
7. **Discriminating Probe** — the highest-value next step to collapse uncertainty

## Evidence strength hierarchy

From strongest to weakest:

1. Controlled reproductions / direct experiments / uniquely discriminating artifacts
2. Primary source artifacts with tight provenance (trace events, logs, metrics, configs, git history, file:line behavior)
3. Multiple independent sources converging on the same explanation
4. Single-source code-path or behavioral inference
5. Weak circumstantial clues (timing, naming, stack order)
6. Intuition / analogy / speculation

## Workflow

1. Restate the observed result precisely
2. Generate multiple deliberately different candidate hypotheses
3. Assign default lanes:
   - **Code-path / implementation cause**
   - **Config / environment / orchestration cause**
   - **Measurement / artifact / assumption mismatch cause**
4. For each lane, gather evidence for AND against
5. Run a rebuttal round between the leading hypothesis and strongest alternative
6. Detect convergence or genuine separation
7. Synthesize into ranked findings

## Rules

- Every serious trace must try to falsify its own favorite explanation
- Down-rank hypotheses when direct evidence contradicts them, they survive only by adding unverified assumptions, or they make no distinctive prediction
- Do not collapse into a generic fix-it coding loop
- Preserve a ranked shortlist even if one explanation is currently dominant
- Do not claim convergence just because multiple lanes use similar language — require the same root causal mechanism or independent evidence streams

## Output Contract

### Observed Result
[What happened]

### Ranked Hypotheses
| Rank | Hypothesis | Confidence | Evidence Strength |
|------|------------|------------|-------------------|
| 1 | ... | High / Medium / Low | Strong / Moderate / Weak |

### Evidence Summary by Hypothesis
- Hypothesis 1: ...
- Hypothesis 2: ...

### Evidence Against / Missing Evidence
- Hypothesis 1: ...
- Hypothesis 2: ...

### Rebuttal Round
- Best rebuttal to leader: ...
- Why leader held / failed: ...

### Most Likely Explanation
[Current best explanation]

### Critical Unknown
[Single missing fact keeping uncertainty open]

### Recommended Discriminating Probe
[Single next probe to collapse uncertainty]
