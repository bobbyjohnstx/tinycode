---
name: incident
description: Triage a live system failure — impact, evidence, and one next command
---

# Incident

Use this when something is down, degraded, or alerting.

## When not to use

- The user asked for a planned change and nothing is failing. Use the change skill.
- The failure is in this repository's code. Hand it to debugger.

## Workflow

1. State what is broken and who is affected.
2. Record the cluster, namespace, and how you know.
3. Gather read-only evidence. Prefer oc or kubectl get, describe, and logs, or the product plugin.
4. Name the blast radius of the next command.
5. Stop after one next command.

## Output contract

- Impact
- Evidence
- Blast radius
- One next command
