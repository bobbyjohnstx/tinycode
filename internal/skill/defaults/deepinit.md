---
name: deepinit
description: Deep codebase initialization — generates per-directory AGENTS.md files with parent-linked hierarchy for whole-repo AI documentation coverage
---

# Deep Init

Creates comprehensive, hierarchical AGENTS.md documentation across the entire codebase.

## When to Use

Use this skill when:
- The user says "deepinit", "deep init", "document the whole repo", or "per-directory docs"
- The repo is large, nested, or multi-package and a single root CLAUDE.md is insufficient
- You need agents to navigate documentation hierarchically
- Existing AGENTS.md files need updating while preserving manual annotations

## When Not to Use

- A single root documentation file is all that's needed — use `/init` instead
- The repo is trivial or flat (1-2 directories)
- The user wants to document a single file or answer a one-off question

## AGENTS.md Template

```markdown
<!-- Parent: {relative_path_to_parent}/AGENTS.md -->

# {Directory Name}

## Purpose
{One-paragraph description}

## Key Files
| File | Description |
|------|-------------|
| `file.ts` | Brief description |

## Subdirectories
| Directory | Purpose |
|-----------|---------|
| `subdir/` | What it contains |

## For AI Agents
### Working In This Directory
{Special instructions}

### Testing Requirements
{How to test changes}

<!-- MANUAL: Notes below this line are preserved on regeneration -->
```

## Workflow

1. **Map directory structure**: List all directories, excluding node_modules, .git, dist, build, __pycache__, .venv, coverage.
2. **Create work plan**: Organize by depth level (parent first).
3. **Generate level by level**: Parents before children to ensure parent references resolve.
4. **Compare and update**: If AGENTS.md exists, preserve `<!-- MANUAL -->` sections while updating auto-generated content.
5. **Validate hierarchy**: Confirm all parent references resolve, no orphans, completeness.

## Rules
- Generate parent levels before child levels.
- Preserve `<!-- MANUAL -->` sections on regeneration.
- Skip empty directories (no files, no subdirectories).
- Document only the requested repo or subtree — do not generate files outside scope.

## Output Contract

Report on completion:
- **Directories scanned:** total count
- **Files created:** list of new AGENTS.md paths
- **Files updated:** list of updated paths
- **Files skipped:** directories skipped with reason
- **Validation results:** pass/fail for parent references, orphans, completeness
