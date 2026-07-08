# Deployment Use Cases

Three primary deployment patterns for tinycode, each with distinct capabilities and constraints.

## 1. Developer Workstation

Run tinycode on your desktop/laptop against local or cloud LLMs.

**When to use**: Daily coding — full filesystem access, local builds, git workflows.

### Setup

```bash
# Install — pick one:
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/dev/install.sh | sh
npx tinycode-ai                          # or: npm install -g tinycode-ai
brew install bobbyjohnstx/tap/tinycode   # macOS / Linux

# Run
tinycode                    # TUI against current directory
tinycode /path/to/project   # TUI against a specific project
tinycode serve              # headless API server
tinycode web                # server + web UI

# Development (from source)
bun install
bun dev                     # TUI against current directory
bun dev /path/to/project    # TUI against a specific project
bun dev serve               # headless API server
bun dev web                 # server + web UI
```

### LLM Configuration

```bash
# Local Ollama (auto-discovered at localhost:11434)
ollama serve

# Local vLLM (auto-discovered at localhost:8000)
vllm serve <model>

# LAN MaaS server
export TINYCODE_MAAS_HOST=https://your-maas-server
export TINYCODE_MAAS_API_KEY=your-key

# Cloud providers via API key
export OPENROUTER_API_KEY=your-key
# or configure in ~/.config/tinycode/config.json
```

### Capabilities

- Full filesystem read/write/edit
- Shell access for builds, tests, git
- LSP integration for code intelligence
- MCP server connections (local and remote)
- Plugin system (`tinycode plug <module>`)

---

## 2. Containerized Developer Workspace

tinycode deployed in a container (OpenShift, Kubernetes, devcontainer) with access to a shared model service and a `/projects` PVC.

**When to use**: Remote development — editing files in a container, code review, document generation, architecture analysis.

### Setup (Operator)

```yaml
apiVersion: tinycode.dev/v1alpha1
kind: TinycodeInstance
metadata:
  name: dev-workspace
spec:
  model: vllm/qwen3-30b
  storage:
    size: 10Gi
```

### Setup (Manual)

```bash
# Inside the container
bun install
bun dev serve --hostname 0.0.0.0

# Connect to shared vLLM service
export TINYCODE_VLLM_URLS=http://vllm-service:8000
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `TINYCODE_VLLM_URLS` | Comma-separated vLLM service URLs |
| `TINYCODE_MAAS_HOST` | MaaS/LiteMaaS endpoint |
| `TINYCODE_MAAS_API_KEY` | MaaS authentication key |
| `TINYCODE_SERVER_PASSWORD` | Server authentication (required for remote access) |

### Capabilities

- File read/write within `/projects`
- Shell access for available runtimes (`node`, `python3`, `go`)
- No host filesystem access
- No system package installation (unless pre-installed in image)
- Git operations if configured via operator

### Limitations

- Cannot install system packages
- Build tools must be in the container image
- No LSP unless language servers are pre-installed

---

## 3. Cluster Operator

tinycode as a natural-language interface for OpenShift/Kubernetes cluster management via `oc`/`kubectl` CLI.

**When to use**: Platform operations — pod diagnostics, log analysis, resource scaling, deployment rollouts.

### Setup (Operator)

```yaml
apiVersion: tinycode.dev/v1alpha1
kind: TinycodeInstance
metadata:
  name: cluster-ops
spec:
  model: vllm/qwen3-30b
  openshift:
    enabled: true    # mounts oc CLI and kubeconfig
```

### Setup (Manual)

```bash
# Ensure oc/kubectl is available and configured
oc whoami  # verify authentication

# Start tinycode with cluster-admin agent
bun dev serve --hostname 0.0.0.0
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `KUBECONFIG` | Path to kubeconfig (default: `~/.kube/config`) |
| `TINYCODE_VLLM_URLS` | Model service endpoint |

### Capabilities

- `oc`/`kubectl` commands for cluster inspection and management
- Log and event correlation across namespaces
- Resource scaling and deployment management
- YAML generation and validation

### Guardrails

- Confirm before destructive operations (`oc delete`, `scale`, `patch`)
- Always read events/logs before recommending changes
- No file writes outside designated config paths

---

## Comparison

| Capability | Workstation | Container Workspace | Cluster Operator |
|-----------|-------------|-------------------|-----------------|
| Filesystem access | Full | `/projects` PVC | Config only |
| Shell access | Full | Available runtimes | `oc`/`kubectl` |
| Git | Yes | If configured | No |
| Build tools | System-installed | Container image | No |
| LSP | Yes | If pre-installed | No |
| MCP servers | Local + remote | Remote only | Remote only |
| Model source | Local/cloud | Shared service | Shared service |
