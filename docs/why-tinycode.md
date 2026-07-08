# Why Tinycode

Built on a foundation of 13,700+ commits of production-tested code, tinycode is a lean, powerful AI coding assistant designed for developers and teams who want to run AI assistance entirely on their own infrastructure—with zero cloud dependencies.

## Sovereign AI: Complete Control, No Vendor Lock-in

tinycode runs entirely on your infrastructure. No data leaves your network. No third-party cloud backends. No feature locks tied to managed services.

This is the core value proposition: **if your LLM stays on your machines, your code stays off the internet.**

Deploy to:
- **Your laptop** — instant, air-gapped local inference with Ollama or vLLM
- **Your private Kubernetes cluster** — declarative deployment via the tinycode-operator, with cross-namespace model discovery and multi-user workspaces
- **Your OpenShift infrastructure** — UBI9 container image (quay.io/bjohns/tinycode-container), Kubernetes operator, and cluster-admin mode out of the box

## Built for Small Models

The industry's coding assistants optimize for large, cloud-hosted models. tinycode optimizes for the models you actually run locally: Llama 3, Qwen, Mistral, and other 3B–13B parameter variants.

**24 compact agent presets** tuned specifically for these models:

- Average system prompt: **~1K tokens** (vs. ~4K for full variants)
- Fits snugly in 4K–32K context windows
- No performance degradation—just focused, purposeful instructions
- Auto-selection based on model size

Runs production-grade coding assistance from a laptop. No GPU cluster required.

## Three Interfaces, One Experience

- **Terminal UI (primary)** — Fast, responsive, zero dependencies. Stays in your terminal where your code is.
- **Web UI** — Browser access from anywhere on your network. Same conversation, agent, and tool capabilities as the TUI.
- **Electron Desktop App** — Native window for macOS, Windows, and Linux. System tray integration. Standalone binaries.

All three share the same API server, so you can switch interfaces mid-conversation without losing context.

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

## 24 Specialized Agents, 9 Built-in Skills

Agents with specific expertise:

- **architect** — Read-only code analysis and guidance
- **debugger** — Root-cause analysis and bug fixing
- **executor** — Focused task implementation
- **test-engineer** — Test strategy and TDD workflows
- **security-reviewer** — OWASP Top 10, CVE detection
- **planner** — Strategic work planning
- Plus **18 more** (designer, tracer, verifier, git-master, cluster-admin, and others)

Skills provide progressive capability disclosure:

- `/debug` — Isolate root causes
- `/trace` — Evidence-driven causal analysis
- `/verify` — Confirm changes work
- `/deepinit` — Generate per-directory AGENTS.md files
- `/mcp-setup` — Configure MCP servers interactively
- `/ai-slop-cleaner` — Remove AI-generated cruft safely
- Plus **3 more**

## IDE Integration (Agent Client Protocol)

Run `tinycode acp --cwd /path/to/project` to start an ACP server for IDE integration. Stdio transport — no network exposure, no auth needed for local use. A reference VS Code extension is included; any ACP-compatible editor (Zed, JetBrains, etc.) can connect using the same protocol.

A reference VS Code extension is included. Build custom IDE integrations with the ACP protocol.

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
- **Input validation** — Zod schemas at all boundaries

## Zero-Config Local LLM Auto-Discovery

Point tinycode at your network and it finds your models:

```bash
# tinycode auto-discovers Ollama at localhost:11434
bun dev

# Or specify a vLLM endpoint
export TINYCODE_VLLM_URL=http://your-vllm-server:8000
bun dev
```

No manual endpoint registration. No config file wrestling. Auto-probes model metadata to extract context limits, reasoning capabilities (`<think>` block parsing), and more.

## Built on Production-Tested Architecture

tinycode inherits its foundation from a mature, proven codebase:

- **Effect framework** — Typed errors, dependency injection, and resource management
- **Hono HTTP routing** — Multi-environment server framework
- **SQLite + Drizzle ORM** — Robust data layer with Effect wrappers
- **SolidJS** — Reactive UI layer, both TUI and web
- **Bun** — Fast runtime and build toolchain (also Node.js compatible)

13,700+ commits of battle-tested patterns and decisions.

## Getting Started

```bash
# Quick install
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/dev/install.sh | sh

# Or npm
npx tinycode-ai

# Run
tinycode                                    # TUI mode
tinycode /path/to/project                   # TUI against a specific project
tinycode serve                              # headless API server
tinycode web                                # server + web UI
tinycode acp --cwd /path/to/project         # Connect from VS Code via ACP

# From source (for development)
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
bun install
bun dev

# Deploy to Kubernetes
kubectl apply -f tinycode-operator/config/samples/tinycode_v1alpha1_basic.yaml

# Pull the container image
podman pull quay.io/bjohns/tinycode-container:latest
```

## The Ecosystem

Three complementary projects work together:

| Project | Role |
|---------|------|
| **tinycode** (this repo) | Core server, TUI, web UI, desktop app, agents, skills, tools, and LLM provider integrations |
| **tinycode-container** | OCI image for Kubernetes and OpenShift deployments—bundles tinycode with oh-my-tiny, tmux, and git |
| **tinycode-operator** | Kubernetes Operator for declarative TinycodeInstance management, RBAC, GitOps, and multi-team scenarios |

## What tinycode Is Not

tinycode is not a cloud-hosted service. It doesn't replace OpenAI or Anthropic APIs for teams without local LLM infrastructure. If your primary use case is seamless cloud provider switching with minimal operational overhead, tinycode may not be the right fit.

But if you're building AI assistance that stays on your infrastructure, runs without vendor lock-in, and scales with your team's Kubernetes clusters, tinycode is purpose-built for that mission.

---

**Ready to build sovereign AI?** Start with `bun install && bun dev`. No account. No cloud dependencies. Just you, your code, and your models.
