---
name: review
description: Code review with structured feedback — correctness, style, performance, and actionable suggestions ranked by severity
---

# Review

Structured code review skill for providing thorough, actionable feedback on code changes.

## When to Use

Use this skill when:
- The user says "review", "code review", "review this PR", "check my changes"
- A diff, PR, or set of changes needs structured quality review
- Post-implementation quality check before merge

## When Not to Use

- The user wants to verify functionality works — use `verify`
- The user wants to debug a failure — use `debug`
- The user wants to refactor or fix code — just implement it
- The user wants a security-focused audit — use a security review agent

## Workflow

1. **Scope the review**: Identify the diff or changeset. Use `git diff`, PR contents, or specified files.
2. **Read the changes**: Read all modified files in full context, not just diff hunks.
3. **Analyze each file** for:
   - **Correctness**: Logic errors, edge cases, off-by-one errors, null/nil handling
   - **Style**: Naming, formatting, idiom adherence, consistency with surrounding code
   - **Performance**: Unnecessary allocations, O(n^2) patterns, missing caching
   - **Maintainability**: Complexity, coupling, testability, readability
   - **Completeness**: Missing error handling, untested paths, incomplete migrations
4. **Cross-cutting concerns**: Check for issues spanning multiple files — broken contracts, missing updates to callers, incomplete renames.
5. **Prioritize findings**: Rank by severity (blocking, suggestion, nit).

## Rules
- Read surrounding code for context, not just the diff.
- Provide concrete fix suggestions, not vague advice.
- Separate blocking issues from stylistic preferences.
- Acknowledge what is done well, not just what is wrong.

## Output Contract

For each finding, report:
- **File:line** — exact location
- **Severity** — blocking / suggestion / nit
- **Issue** — what is wrong and why
- **Fix** — concrete suggestion (code snippet if applicable)

End with a **Summary**:
- **Overall assessment**: approve / request changes / needs discussion
- **Blocking issues**: count
- **Suggestions**: count
- **Nits**: count
