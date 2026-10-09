---
name: change
description: Plan one system change — the command, the check, and the undo
---

# Change

Use this before a planned update to a cluster, host, or Red Hat product.

## When not to use

- Something is already broken. Use the incident skill.
- The change is a source edit. Hand it to executor.

## Workflow

1. State the cluster, namespace, and object.
2. Write the exact command.
3. State what it changes and what it does not change.
4. State the read-only command that proves it worked.
5. State how to undo it.
6. Run the command only after the permission prompt. Then run the check.

## Output contract

- Command
- Effect
- Check
- Undo
