---
name: host
description: Inspect a machine, local or over ssh, for health, misconfiguration, and exposure
---

# Host

Use this when the subject is a machine. That is the host tinycode is running on, or another host reached with ssh. A cluster node is this skill once you are on the machine. The node object in the API is the incident skill.

## When not to use

- The subject is a cluster object and you are not on the machine. Use the incident skill.
- The user asked for a planned change and the machine is healthy. Use the change skill.
- The exposure is in this repository's source. Hand it to security-reviewer.

## Workflow

1. Name the machine and how you will reach it. Use the local shell for the host tinycode is running on. Use ssh for any other host. Say the target user and host before the first command.
2. If ssh fails, stop. Report whether it was auth, the host key, or reachability. Do not try other accounts or keys.
3. Read first, in one pass: who you are, hostname, OS, load, failed services, disk space and inodes, clock sync, default route, DNS resolution, listening sockets, firewall rules, and SELinux or AppArmor mode.
4. Security pass, still read-only: unexpected listeners, world-writable paths, sudoers, and sshd settings. Report them. Do not fix them in this turn.
5. Do not print key material, password hashes, tokens, or the contents of secret files.
6. One change only after you state the exact command, the read-only check, and the undo. Service, account, firewall, and address or route changes wait for the permission prompt, including over ssh. Then run the check.

## Output contract

- Host (local or ssh target)
- Evidence
- Exposure
- Command, or none
- Check
- Undo
