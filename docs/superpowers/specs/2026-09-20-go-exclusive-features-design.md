# Go-Exclusive Feature Design: "Tinycode Anywhere"

**Status**: DRAFT — brainstorming phase, not approved for implementation
**Date**: 2026-09-20
**Priority focus**: Offline/air-gapped AI (#1), then edge deployment, multi-tenant, plugin ecosystem

## Design Principles

- Design for the hardest case (fully disconnected enterprise), easier cases come free
- Tiered approach for both models and plugins — three levels of integration depth
- Leverage Go's unique strengths: single binary, 40MB RSS, CGo for native inference, cross-compilation, goroutine-per-session

## Target Environments (all four)

1. **Disconnected enterprise** — RHEL/SLES behind firewall, no internet ever, models pre-loaded
2. **Field/edge** — laptops offline, intermittent connectivity, sync when online
3. **Secure dev environments** — OpenShift Dev Spaces, Podman Desktop, air-gapped CI/CD
4. **Shared team servers** — multi-tenant, scales to hundreds of concurrent sessions

## Section 1: Offline-First Model Management

### Tier 1 — Model Cache Sync (build first)

- `tinycode models pull ollama/qwen3:8b` — downloads to `~/.local/share/tinycode/models/`
- Auto-discovery checks local cache before network; if cached model exists and provider unreachable, use cached
- `tinycode models list --cached` shows what's available offline
- Model configs (context window, capabilities, prompt format) cached alongside weights

### Tier 2 — Sidecar Distribution (planned offline)

- `tinycode bundle --model qwen3:8b --plugins ocp,etcd-diag -o tinycode-ocp-airgap.tar.gz`
- Produces: tinycode binary + llama.cpp server binary + model weights + plugin binaries + config
- `tinycode --sidecar` auto-starts bundled inference server, routes requests locally
- Go manages sidecar lifecycle (start/stop/health) via `os/exec`

### Tier 3 — Embedded Inference (true single binary)

- Compile llama.cpp via CGo into tinycode itself
- `tinycode run --embedded "fix the bug"` — in-process inference, no network, no sidecar
- Cross-compile for linux/amd64, linux/arm64, darwin/arm64
- Target: 1-3B quantized models (Q4_K_M), 1-2GB RAM alongside 40MB binary
- Fundamentally impossible in TS — can't statically link C inference engine into Bun

## Section 2: Plugin Ecosystem (three tiers)

### Tier 1 — Offline Plugin Bundles (default)

- Ship tinycode + plugin binaries in a single archive
- `tinycode init` discovers co-located plugins automatically
- JSON-RPC over stdin/stdout (existing architecture)

### Tier 2 — Static Plugin Compilation (enterprise)

- `go build -tags "plugin_ocp,plugin_etcd"` — bake selected plugins into one binary
- No JSON-RPC overhead, no child process spawning
- Customer gets one file with exactly the capabilities they need
- Impossible in TS — plugins must be separate processes

### Tier 3 — WASM Plugin Sandboxing (future)

- Compile plugins to WASM, embed in tinycode, run in sandbox
- True single binary with runtime plugin loading
- Plugins can't escape sandbox — enterprise security compliance
- Go's WASM support is mature

## Section 3: Multi-Tenant (progressive)

### Phase 1 — Shared Team Server

- `tinycode serve --multi-tenant` — one instance serves 5-50 developers
- 40MB per session (vs 300MB for TS) = 50 concurrent sessions in 2GB RAM
- Goroutine-per-session, not process-per-session
- Authentication via OIDC/LDAP

### Phase 2 — Platform Integration

- Sidecar/microservice inside OpenShift Dev Spaces, Podman Desktop, JupyterHub
- Platform manages users; tinycode handles AI sessions
- Scales to hundreds of concurrent sessions

### Phase 3 — Self-Hosted SaaS

- User management, billing/quotas, model routing, audit logging
- Self-hosted GitHub Copilot alternative

## Section 4: Edge Deployment

- Single binary: `curl -L tinycode.io/install | sh` — works on arm64 Pi, s390x mainframes, air-gapped RHEL
- Zero runtime dependencies (no Node.js, no Bun, no Python)
- 40MB binary + 40MB RSS = runs on 512MB RAM devices
- Cross-compilation matrix: linux/darwin/windows × amd64/arm64/riscv64/s390x
- `tinycode init --offline` bootstraps from bundled configs

## What Go Enables That TS Cannot

| Capability | Go | TS/Bun |
|---|---|---|
| Single binary, no runtime | ✅ 40MB | ❌ ~80MB runtime + node_modules |
| Embedded C inference (CGo) | ✅ llama.cpp in-process | ❌ Can't statically link |
| Static plugin compilation | ✅ Build tags | ❌ Must be separate processes |
| 40MB RSS per session | ✅ | ❌ 150-300MB |
| Cross-compile to s390x/riscv64 | ✅ GOOS/GOARCH | ❌ Platform-specific native deps |
| Goroutine-per-session | ✅ Millions of goroutines | ❌ Process-per-session or worker threads |
| Air-gapped, zero-install | ✅ Copy binary, run | ❌ Needs npm/bun install |

## Open Questions

- [ ] Which inference library for Tier 3? llama.cpp (CGo) vs ggml-go (pure Go) vs whisper.cpp patterns
- [ ] Model format: GGUF only, or also support ONNX/safetensors?
- [ ] WASM plugin sandbox: wasmtime-go vs wazero? Security model?
- [ ] Multi-tenant auth: built-in OIDC or delegate to reverse proxy?
- [ ] Model cache: share between users on multi-tenant, or per-user isolation?
- [ ] Bundle signing: how to verify integrity of air-gapped distributions?

## Next Steps

1. Spike: embedded inference with llama.cpp via CGo (feasibility + perf)
2. Design: model cache sync (Tier 1) — spec + implementation plan
3. Design: `tinycode bundle` command (Tier 2)
4. Design: multi-tenant auth layer for `tinycode serve`
