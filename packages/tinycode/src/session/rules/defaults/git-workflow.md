# Git Workflow Rules

## Commit Message Format

```
<type>(<scope>): <description>

<optional body>

<optional trailers>
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`

### Decision-Context Trailers (include when applicable)

> Trailers are optional for trivial commits (typos, formatting, minor renames). Include them when the decision context is non-obvious or shapes future behavior.

```
Constraint:   active constraint that shaped this decision
Rejected:     alternative considered | reason for rejection
Directive:    warning or instruction for future modifiers
Confidence:   high | medium | low
Scope-risk:   narrow | moderate | broad
Not-tested:   edge case or scenario not covered by tests
```

## Pull Request Workflow

When creating PRs:
1. Analyze full commit history (not just latest commit)
2. Use `git diff [base-branch]...HEAD` to see all changes
3. Draft comprehensive PR summary
4. Include test plan with TODOs
5. Push with `-u` flag if new branch

## Feature Implementation Workflow

1. **Plan First** - Use a planning agent or work through the approach before coding
2. **TDD Approach** - Write tests before implementation (see `testing.md` for full TDD workflow)
3. **Code Review** - Run a code-review pass after writing code
4. **Commit** - Follow conventional commits format above

## Branch Naming

- `feature/` - New features
- `fix/` - Bug fixes
- `refactor/` - Code refactoring
- `docs/` - Documentation changes

## [CUSTOMIZE] Project-Specific Git Rules

Add project-specific git workflow here:
- Branch protection rules
- Required reviewers
- CI/CD requirements
