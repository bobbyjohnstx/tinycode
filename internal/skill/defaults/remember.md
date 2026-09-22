---
name: remember
description: Triage a session's findings across memory surfaces — classify each item and route it to the right destination (project memory, CLAUDE.md, or session notes)
---

# Remember

Use this skill at a natural session boundary when you need to decide what knowledge is worth persisting beyond this conversation.

## When to Use

Use this skill when:
- The user says "remember", "save this", "keep that", or "what should we keep from this session"
- A session surfaced a durable fact, user preference, project convention, or decision with rationale worth persisting
- You are wrapping up a work session and want to distill what graduates from chat history into memory
- The user wants to clean up stale or conflicting memory entries

## When Not to Use

- The user wants a quick scratch note for this session only
- The user wants to directly edit CLAUDE.md — do that edit directly
- Mid-task capture of working state — wait for a natural review point
- The information is already captured in CLAUDE.md or derivable from the code

## Memory surfaces
- **Project memory** — durable project knowledge (facts, feedback, decisions)
- **CLAUDE.md / AGENTS.md** — durable instructions and conventions when they truly belong there
- **Session notes** — temporary working context for the current conversation only

## Workflow
1. Scan the session for memory candidates: facts established, user corrections or preferences voiced, conventions agreed upon, decisions made with rationale. List each candidate before classifying.
2. Classify each item:
   - durable project fact
   - user preference or feedback
   - operator instruction or convention
   - temporary working note
   - duplicate / stale / conflicting information
3. Propose the best destination for each item.
4. Write or update only the appropriate memory surface.
5. Call out duplicates or conflicts that should be cleaned up.

## Rules
- Do not dump everything into one store.
- Prefer project memory for durable team/project knowledge.
- Keep entries concise and actionable.
- If something is uncertain, mark it as uncertain rather than storing it as fact.
- Do not save things already captured in CLAUDE.md or derivable from the code.

## Output Contract

For each item processed, report:
- **Item:** brief description of the knowledge
- **Destination:** exact surface and path
- **Action:** stored / updated / skipped (with reason) / flagged as conflict
- **Entry written:** quote the actual text stored (or "skipped" if not written)

End with a **Conflicts/duplicates** block if any were found, listing what to clean up.
