# Why Tinycode

Built on a foundation of 13,700+ commits of production-tested code, tinycode is a lean, powerful AI coding assistant designed for developers and teams who want to run AI assistance entirely on their own infrastructure—with zero cloud dependencies.

## Sovereign AI: Complete Control, No Vendor Lock-in

tinycode runs entirely on your infrastructure. No data leaves your network. No third-party cloud backends. No feature locks tied to managed services.

This is the core value proposition: **if your LLM stays on your machines, your code stays off the internet.**

Deploy to:
- **Your laptop** — instant, air-gapped local inference with Ollama or vLLM
- **Your private Kubernetes cluster** — declarative deployment via the tinycode-operator, with cross-namespace model discovery and multi-user workspaces
- **Your OpenShift infrastructure** — UBI9 container image (quay.io/bjohns/tinycode-container) and the Kubernetes operator. The **ops** agent is the cluster and host persona

## Built for Small Models

The industry's coding assistants optimize for large, cloud-hosted models. tinycode optimizes for the models you actually run locally: Llama 3, Qwen, Mistral, and other 3B–13B parameter variants.

**Compact prompts** are selected automatically for models at 8B parameters and under. Each shipped specialist has a shorter prompt with the same permissions as the full version.

Runs production-grade coding assistance from a laptop. No GPU cluster required.

## Two Interfaces, One Experience

- **Terminal UI (primary)** — Fast, responsive, zero dependencies. Stays in your terminal where your code is.
- **Web UI** — Browser access from anywhere on your network via `tinycode web`. Same conversation, agent, and tool capabilities as the TUI.

Both share the same API server, so you can switch interfaces mid-conversation without losing context. There is no desktop shell in this repository.

## Safe Exploration with Plan Mode

Read-only exploration isn't a suggestion—it's a guarantee.

- **Tab to plan mode** — Hard permission enforcement. The LLM can explore your codebase and write only to `.tinycode/plans/*.md`. All other edits are blocked at the tool level.
- **Review before executing** — Plans are versioned files. Review them, then approve with one keystroke to switch to build mode.
- **Permission prompts** gate every tool regardless of mode—safety is structural, not just advisory.

## Smart Context Management

Long conversations with small-context models hit limits fast. tinycode handles it intelligently:

- **Deterministic file tracking** — File paths extracted from tool calls, never lost to summarization
- **Observation masking** — Old tool outputs replaced with placeholders, keeping context where it matters
- **Automatic compaction** — Summarizes older messages when approaching limits
- **Circuit breaker** — Warns after 3+ compactions (a signal to start fresh)

Works especially well with 4K–32K context windows.

## Agents and skills

Tab cycles `build`, `general`, `ops`, `plan`, `architect`, and `code-reviewer`. **ops** is cluster and host administration: read first, then one change. **general** is a plain assistant.

- **architect** — Design and trade-offs. Read-only
- **debugger** — Root cause in application code
- **executor** — Scoped code changes
- **tracer** — Competing explanations and the next probe
- **document-specialist** — External docs and changelogs
- **plan** — Work plans. Edits limited to `plans/*` and `drafts/*`

`/incident`, `/change`, and `/host` are checklists. `/debug`, `/trace`, `/plan`, `/verify`, `/test`, and `/review` ask the matching agent. `code-simplifier`, `qa-tester`, and `scientist` ship disabled.

## IDE Integration (Agent Client Protocol)

Run `tinycode acp --cwd /path/to/project` to start an ACP server for IDE integration. Stdio transport — no network exposure, no auth needed for local use. Any ACP-compatible editor can connect. This repository does not include a VS Code extension.

## Kubernetes-Native Deployment

The **tinycode-operator** reduces cluster deployment to a single declarative resource:

```yaml
apiVersion: tinycode.dev/v1alpha1
kind: TinycodeInstance
metadata:
  name: my-team-tinycode
spec:
  vllm:
    - name: qwen3-model
      url: http://qwen3-30b.qwen3.svc.cluster.local:8000
  model: "vllm-qwen3/qwen3-30b"
  auth:
    passwordSecret: tinycode-password
  storage:
    projectsSize: "20Gi"
```

The operator handles:
- Route/Ingress creation for web access
- Persistent volume provisioning
- Security context binding (OpenShift SCC)
- vLLM auto-discovery with metadata probing
- Cross-namespace model discovery
- Team workspaces with RWX PVCs
- GitOps startup (clone repos into workspace on pod start)
- Cluster-admin mode for infrastructure management

Container image: `quay.io/bjohns/tinycode-container:latest` (UBI9, OpenShift-certified)

## Security Hardened

- **Timing-safe authentication** — No timing-side-channel leaks
- **Config secret redaction** — Sensitive values masked in logs
- **Path traversal protection** — Prevents directory escape attacks
- **Security headers** — XSS and CSRF mitigations
- **Input validation** — Struct validation at all boundaries

## Zero-Config Local LLM Auto-Discovery

Point tinycode at your network and it finds your models:

```bash
# tinycode auto-discovers Ollama at localhost:11434
tinycode

# Or specify a vLLM endpoint
export TINYCODE_VLLM_HOST=http://your-vllm-server:8000
tinycode
```

No manual endpoint registration. No config file wrestling. Auto-probes model metadata to extract context limits, reasoning capabilities (`<think>` block parsing), and more.

## Built on Production-Tested Architecture

tinycode is a ground-up Go rewrite of the original TypeScript codebase (13,700+ commits), built on proven patterns:

- **standard library `net/http`** — Standard Go HTTP server with middleware
- **bubbletea** — Elm-architecture TUI framework (Charmbracelet)
- **modernc.org/sqlite** — Pure-Go SQLite with migrations
- **slog** — Structured logging via Go standard library
- **Single binary** — No runtime dependencies, no package manager

## Getting Started

```bash
# Install — pick one:
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh
brew install bobbyjohnstx/tap/tinycode   # macOS / Linux

# Run
tinycode                                    # TUI mode
tinycode /path/to/project                   # TUI against a specific project
tinycode serve                              # headless API server
tinycode acp --cwd /path/to/project         # Connect from VS Code via ACP

# Build from source
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
make build
./dist/tinycode

# Deploy to Kubernetes
kubectl apply -f tinycode-operator/config/samples/tinycode_v1alpha1_basic.yaml

# Pull the container image
podman pull quay.io/bjohns/tinycode-container:latest
```

## The Ecosystem

Three complementary projects work together:

| Project | Role |
|---------|------|
| **tinycode** (this repo) | Core server, TUI, web UI, agents, skills, tools, and LLM provider integrations |
| **tinycode-container** | OCI image for Kubernetes and OpenShift deployments—bundles tinycode with oh-my-tiny, tmux, and git |
| **tinycode-operator** | Kubernetes Operator for declarative TinycodeInstance management, RBAC, GitOps, and multi-team scenarios |

## What tinycode Is Not

tinycode is not a cloud-hosted service. It doesn't replace OpenAI or Anthropic APIs for teams without local LLM infrastructure. If your primary use case is seamless cloud provider switching with minimal operational overhead, tinycode may not be the right fit.

But if you're building AI assistance that stays on your infrastructure, runs without vendor lock-in, and scales with your team's Kubernetes clusters, tinycode is purpose-built for that mission.

---

**Ready to build sovereign AI?** Start with `make build && ./dist/tinycode`. No account. No cloud dependencies. Just you, your code, and your models.
