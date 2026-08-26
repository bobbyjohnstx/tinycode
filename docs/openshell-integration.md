# OpenShell Integration

[NVIDIA OpenShell](https://docs.nvidia.com/openshell/about/overview) is an open-source (Apache 2.0) sandboxed runtime for AI coding agents. It provides kernel-level isolation via Landlock and seccomp with declarative YAML policies controlling filesystem, network, process, and inference access.

OpenShell does not replace tinycode's agent, tool, or session system — it wraps the entire process in a security sandbox. They are complementary layers.

## Architecture

OpenShell has two components:

- **Gateway** — Control plane that manages policy enforcement, credential injection, and inference routing. Runs outside the sandbox.
- **Supervisor** — Runs inside each sandbox container, enforcing kernel-level restrictions defined in YAML policy files.

Sandboxes are containers (Docker, Podman, or Kubernetes pods) with policies that control what the agent can access. OpenShell uses the `AgentSandbox` CRD (`sandboxes.agents.x-k8s.io`) from the Kubernetes SIG project for cluster deployments.

## Why OpenShell + tinycode

| Concern | Without OpenShell | With OpenShell |
|---------|-------------------|----------------|
| Shell tool execution | Unrestricted (user approval only) | Kernel-level filesystem/network/process isolation |
| LLM inference routing | Direct network to Ollama/vLLM | Routed via `inference.local`, no direct network needed |
| GPU access | Manual device plugin config | `--gpu` flag for NVIDIA passthrough |
| Credential management | Environment variables | Gateway-managed injection |
| Network policy | Manual NetworkPolicy on K8s | Declarative per-sandbox YAML |
| MCP servers | Direct network to MCP endpoints | Routed via `protocol: mcp` in network policy |

## Running tinycode in an OpenShell Sandbox

### Prerequisites

- OpenShell installed ([installation guide](https://docs.nvidia.com/openshell/getting-started/installation))
- A local LLM provider (Ollama, vLLM) running on the host
- tinycode container image (`ghcr.io/bjohns/tinycode-container:latest`)

### Basic Usage

```bash
# Run tinycode in a sandbox with GPU access
openshell sandbox create \
  --image ghcr.io/bjohns/tinycode-container:latest \
  --gpu \
  -- tinycode
```

### With Host-Side Ollama

When Ollama runs on the host, OpenShell routes inference traffic through `host.openshell.internal`:

```bash
# Start a sandbox with inference routing to host Ollama
openshell sandbox create \
  --image ghcr.io/bjohns/tinycode-container:latest \
  --provider ollama --type openai --host host.openshell.internal:11434 \
  -- tinycode
```

Inside the sandbox, configure tinycode to use the routed endpoint:

```json
{
  "model": "ollama/qwen3.5:9b"
}
```

tinycode's auto-discovery will find Ollama at the routed address.

### With vLLM

```bash
openshell sandbox create \
  --image ghcr.io/bjohns/tinycode-container:latest \
  --provider vllm --type openai --host host.openshell.internal:8000 \
  -- tinycode
```

### Custom Policy

Create a policy file to control what tinycode can access:

```yaml
# tinycode-policy.yaml
filesystem:
  read:
    - /workspace
    - /home/tinycode
  write:
    - /workspace
    - /home/tinycode/.config/tinycode
    - /tmp
network:
  allow:
    - inference.local
process:
  allow:
    - git
    - bun
    - node
    - grep
    - find
```

```bash
openshell sandbox create \
  --policy tinycode-policy.yaml \
  --image ghcr.io/bjohns/tinycode-container:latest \
  -- tinycode /workspace
```

## Installing tinycode Plugins in a Sandbox

OpenShell controls what the sandbox can access externally, not what runs inside the container. tinycode plugins (`tinycode plugin install <name>`) work inside a sandbox as long as the policy permits it.

**Option 1: Pre-bake plugins into the container image (recommended)**

Install plugins during the Docker build so no runtime network access is needed:

```dockerfile
# In your Containerfile
RUN tinycode plugin install tinycode-plugin-example
```

This is the most secure approach — the sandbox policy can block all network access except the inference endpoint.

**Option 2: Install plugins at runtime**

Allow npm registry access in the sandbox policy:

```yaml
# tinycode-policy.yaml
filesystem:
  write:
    - /workspace
    - /home/tinycode/.config/tinycode
    - /tmp
network:
  allow:
    - inference.local
    - registry.npmjs.org
process:
  allow:
    - git
    - bun
    - node
    - npm
```

Then install plugins inside the running sandbox:

```bash
tinycode plugin install tinycode-plugin-example
```

**MCP servers:** OpenShell sandboxes can connect to external MCP servers as clients using `protocol: mcp` in the network policy. You can also bundle MCP servers inside the container image — OpenShell does not interfere with processes running inside the container.

## OpenShift Deployment

OpenShell has Helm-based Kubernetes support but **no OpenShift-specific documentation exists yet**. Deploying on OpenShift requires addressing several platform-specific concerns.

### Installing OpenShell on OpenShift

```bash
helm install openshell oci://ghcr.io/nvidia/openshell/helm-chart \
  --namespace openshell-system \
  --create-namespace
```

### OpenShift-Specific Considerations

OpenShell's defaults conflict with OpenShift's `restricted-v2` SCC in several ways:

| Concern | OpenShell Default | OpenShift Requirement | Resolution |
|---------|-------------------|----------------------|------------|
| AppArmor | `Unconfined` | Profile required by restricted-v2 | Create a custom SCC or use `privileged` SCC for the gateway |
| UID | Root sidecar proxy (UID 0) | Arbitrary UID enforcement | Bind gateway ServiceAccount to an SCC that allows UID 0 |
| Seccomp | Custom profile | RuntimeDefault required | Custom SCC with seccomp allowance |
| Networking | Port-forward for access | Route/Ingress preferred | Create an OpenShift Route to the gateway service |
| GPU | `--gpu` passthrough | NVIDIA GPU Operator + device plugin | Install the [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/index.html) on the cluster first |

**SCC configuration for the gateway:**

```bash
# Create a ServiceAccount for OpenShell
oc -n openshell-system create sa openshell-gateway

# Bind to a permissive SCC (gateway needs elevated privileges)
oc adm policy add-scc-to-user anyuid -z openshell-gateway -n openshell-system
```

**Expose the gateway via Route:**

```bash
oc -n openshell-system expose svc/openshell-gateway
```

> **Note:** These instructions are untested and based on OpenShell's Kubernetes requirements mapped to OpenShift equivalents. The OpenShell project does not officially support OpenShift yet. Test thoroughly in a non-production cluster first.

### Operator Integration (Future)

The tinycode-operator could detect OpenShell on the cluster and optionally delegate sandbox management:

1. **Detection** — Check for the `sandboxes.agents.x-k8s.io` CRD or the gateway service
2. **Sandbox creation** — Instead of creating bare Deployments, create `AgentSandbox` custom resources
3. **Inference routing** — Use OpenShell's gateway for vLLM routing instead of direct service discovery
4. **Security context** — Delegate SCC/seccomp configuration to OpenShell policies

This integration is tracked in [issue #90](http://localhost:3000/bjohns/tinycode/issues/90).

## Comparison with tinycode-operator

OpenShell and tinycode-operator serve different purposes:

| Feature | tinycode-operator | OpenShell |
|---------|-------------------|-----------|
| Purpose | Lifecycle management | Security sandbox |
| Deploys tinycode | Yes (CRD-driven) | No (wraps existing containers) |
| vLLM discovery | Auto-probes cluster services | Routes via gateway |
| Storage provisioning | PVC management | Not applicable |
| GitOps mode | Built-in repo cloning | Not applicable |
| Shared workspaces | RWX PVC support | Not applicable |
| Security isolation | SCC binding, NetworkPolicy | Kernel-level Landlock/seccomp |
| GPU management | Device plugin config | `--gpu` passthrough |
| MCP connectivity | Direct network | Gateway-routed with policy |

The operator manages *what* runs; OpenShell manages *how safely* it runs. They can work together.

## Next Steps

- Test tinycode inside an OpenShell sandbox on a local Docker setup
- Validate `inference.local` routing with tinycode's provider auto-discovery
- Test OpenShift deployment with SCC and Route configuration
- Propose tinycode as a supported agent upstream in the OpenShell project
- Evaluate operator integration for OpenShift clusters with OpenShell installed
