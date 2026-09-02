# Deployment Guide

## Remote Installation

For production deployments on OpenShift or Kubernetes, the recommended approach is the **tinycode-operator** — it manages `TinycodeInstance` custom resources and handles deployment, storage, routing, and security context automatically. See the [tinycode-operator](https://github.com/bobbyjohnstx/tinycode-operator) repository.

For installing directly on a remote server, clone from your Gitea or GitHub remote and run `bun install`. If you need to transfer via zip instead:

```bash
zip -r tinycode.zip . \
  --exclude "*/node_modules/*" \
  --exclude ".git/*" \
  --exclude "*/dist/*"
```

| Exclusion          | Why                                                        |
| ------------------ | ---------------------------------------------------------- |
| `*/node_modules/*` | npm dependencies — restored by `bun install` on the target |
| `.git/*`           | Git history — not needed to run the server                 |
| `*/dist/*`         | Built binaries and web UI assets — regenerated at runtime  |

On the target server after unzipping:

```bash
# Install Bun (if not already installed)
curl -fsSL https://bun.sh/install | bash

# Install dependencies
bun install

# Run headless server, bound to all interfaces for remote access
bun dev serve --hostname 0.0.0.0

# Or set in ~/.config/tinycode/config.json:
# { "server": { "hostname": "0.0.0.0" } }
```

Set `TINYCODE_SERVER_PASSWORD` before starting — without it the server is unsecured. Open port 4096 in the firewall:

```bash
sudo firewall-cmd --add-port=4096/tcp --permanent && sudo firewall-cmd --reload
```

Access the web UI at `http://<server-ip>:4096`.

## Container Images

Pre-built container images are published to [Quay.io](https://quay.io/repository/bjohns/tinycode-container):

```bash
# Pull the latest image
podman pull quay.io/bjohns/tinycode-container:latest

# Run locally with Ollama on the host network
podman run -it --network host \
  -e TINYCODE_SERVER_PASSWORD=changeme \
  quay.io/bjohns/tinycode-container:latest

# Run with a remote vLLM endpoint
podman run -it -p 4096:4096 \
  -e TINYCODE_VLLM_URL=http://your-vllm-server:8000 \
  -e TINYCODE_SERVER_PASSWORD=changeme \
  quay.io/bjohns/tinycode-container:latest
```

Images are also mirrored to `ghcr.io/bobbyjohnstx/tinycode-container`. Both registries receive identical multi-arch builds (amd64 + arm64) on every push to main.

## OpenShift / Kubernetes Deployment

> **Important:** A bare tinycode container on a cluster can only chat with the model — it has no files to read, no code to edit, and no project context. To make it useful, give it something to work on: clone a git repo via GitOps mode, enable cluster-admin mode with `oc` CLI access, or mount a host path with existing code. Without one of these, the container experience is no different from a simple chat UI.

The recommended path for cluster deployments is the **tinycode-operator**. It reduces a full deployment to a single CR:

```yaml
apiVersion: tinycode.dev/v1alpha1
kind: TinycodeInstance
metadata:
  name: my-tinycode
spec:
  # Give tinycode something to work on — at least one of these:
  git:                                    # Clone a repo into /projects on startup
    url: https://github.com/your-org/your-repo.git
  clusterAdmin:                           # Enable oc CLI for cluster management
    enabled: true
    kubeconfigSecretName: my-kubeconfig
  vllm:
    - name: vllm-qwen3
      url: http://qwen3-30b.qwen3.svc.cluster.local:8080
  model: "vllm-qwen3/qwen3-30b"
  auth:
    passwordSecret: tinycode-password
  storage:
    projectsSize: "20Gi"
```

**Common deployment patterns:**

| Pattern | What it enables | Key spec fields |
|---------|----------------|-----------------|
| **Code assistant** | Edit files in a cloned repo, run tests, commit changes | `spec.git.url` |
| **Cluster operator** | Manage OpenShift resources, debug pods, review logs | `spec.clusterAdmin.enabled` |
| **Both** | Full-stack work: edit code AND deploy to the cluster | `spec.git` + `spec.clusterAdmin` |
| **Team workspace** | Multiple users share a project on RWX storage | `spec.storage.projectsAccessMode: ReadWriteMany` + `spec.replicas: 2` |

The operator handles Route creation, PVC provisioning, SCC binding, vLLM model auto-probing, and pod lifecycle. See the [tinycode-operator README](https://github.com/bobbyjohnstx/tinycode-operator) and [RHOAI cluster setup guide](https://github.com/bobbyjohnstx/tinycode-operator/blob/main/docs/rhoai-cluster-setup.md) for full documentation.
