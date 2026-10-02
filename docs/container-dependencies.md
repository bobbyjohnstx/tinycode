# External Tool Dependencies

External CLI tools that tinycode shells out to at runtime. Required for containerized deployments.

## Core (always needed)

| Tool | Used by | Purpose |
|------|---------|---------|
| `sh` | `internal/tool/shell.go` | Shell tool — all user/agent shell commands |
| `git` | `internal/vcs/git.go`, `internal/project/`, `internal/server/revert.go`, `builtin_code_review` | VCS detection, diff, stash, revert |
| `oc` | `internal/redhat/oc.go`, `ocp-context-injection` | OpenShift CLI — cluster auth, resource queries, apply |

## Platform-specific (optional)

| Tool | Platform | Used by | Purpose |
|------|----------|---------|---------|
| `osascript` | macOS | `builtin_notify` | Desktop notifications |
| `notify-send` | Linux | `builtin_notify` | Desktop notifications |
| `pbcopy` | macOS | `internal/tui/clipboard.go` | Clipboard write |
| `xclip` / `xsel` / `wl-copy` | Linux | `internal/tui/clipboard.go` | Clipboard write |
| `sysctl` | macOS | `internal/provider/gpu_memory.go` | System memory detection |
| `nvidia-smi` | Linux (NVIDIA) | `internal/provider/gpu_memory.go` | GPU memory detection |

## Plugin-specific

| Tool | Plugin | Purpose |
|------|--------|---------|
| `ansible-lint` | `aap-bridge` | Playbook linting |

## Implied by shell tool

The shell tool (`sh -c`) means users and agents can invoke anything available in `$PATH`. Common tools the agent relies on:

- `kubectl` — Kubernetes resource management (many plugins instruct the agent to run kubectl commands)
- `curl` / `wget` — HTTP requests from shell
- `jq` — JSON processing in shell pipelines
- `grep` / `find` / `ls` / `cat` — Standard file operations
- `python3` — Occasional data processing

## LSP and MCP servers

LSP servers (`internal/lsp/client.go`) and MCP servers (`internal/mcp/stdio.go`) are spawned via exec with user-configured commands. These are fully user-defined — the container needs whatever servers the user configures.

## Recommended container base

For an OpenShift-focused container image:

```dockerfile
FROM registry.access.redhat.com/ubi9/ubi-minimal

# Core
RUN microdnf install -y git tar gzip --nodocs && microdnf clean all

# OpenShift CLI
RUN curl -sL https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/openshift-client-linux.tar.gz \
    | tar xz -C /usr/local/bin oc kubectl

# Shell utilities the agent commonly uses
RUN microdnf install -y jq findutils procps-ng --nodocs && microdnf clean all

# Optional: ansible-lint for aap-bridge plugin
# RUN pip3 install ansible-lint

COPY dist/tinycode /usr/local/bin/tinycode
ENTRYPOINT ["tinycode"]
```

For an OpenShell wrapper, the entrypoint would be the OpenShell process that manages tinycode as a subprocess.
