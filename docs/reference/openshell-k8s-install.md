# Installing OpenShell on Plain Kubernetes

## Prerequisites

- **Kubernetes 1.28+** (user namespaces need 1.33+)
- **Agent Sandbox CRDs** (cluster-scoped, not included in the Helm chart):
  ```bash
  kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/latest/download/manifest.yaml
  ```
- **Default StorageClass** — or set `server.workspaceStorageClass` explicitly, otherwise workspace PVCs hang on Pending
- **cert-manager** (optional, for TLS)

## Install

```bash
kubectl create namespace openshell

helm install openshell oci://ghcr.io/nvidia/openshell/helm-chart \
  --version 0.6.0 -n openshell
```

Pin to a semver release — the `0.0.0-dev` tag tracks `main` and isn't stable.

## Verify

```bash
kubectl get pods -n openshell
kubectl port-forward -n openshell svc/openshell 8081:8081
curl http://localhost:8081/healthz
```

## Key Config Values

| Value | Default | Notes |
|-------|---------|-------|
| `replicaCount` | 1 | >1 requires external PostgreSQL |
| `server.sandboxImage` | `ghcr.io/.../base:latest` | Default sandbox container |
| `server.disableTls` | false | Set true behind a reverse proxy |
| `server.auth.allowUnauthenticatedUsers` | false | Dev-only escape hatch |

## Caveats

- **Experimental** — NVIDIA says "under active development, expect rough edges and breaking changes"
- Default backend is **SQLite** (single replica only). Multi-replica needs PostgreSQL via `server.externalDbSecret`
- AppArmor defaults to `Unconfined` — the supervisor needs this for network namespace mounts
- A pre-install hook Job auto-bootstraps mTLS and JWT secrets
