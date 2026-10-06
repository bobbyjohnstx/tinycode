# Deployment Guide

tinycode is a standalone Go binary. Deployment is copying the binary to the target machine and running it.

## Quick Start

```bash
# Build from source
make build

# Run the TUI
./dist/tinycode

# Run headless API server
./dist/tinycode serve
```

## Binary Deployment

### Build

```bash
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
make build
```

This produces `dist/tinycode`. Copy it to the target machine:

```bash
scp dist/tinycode user@server:/usr/local/bin/tinycode
```

No runtime dependencies are required -- the binary is self-contained.

### Cross-compilation

Build for a different platform:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=linux GOARCH=arm64 make build
```

### Headless Server (systemd)

For running tinycode as a persistent headless API server:

```ini
# /etc/systemd/system/tinycode.service
[Unit]
Description=tinycode headless API server
After=network.target

[Service]
Type=simple
User=tinycode
ExecStart=/usr/local/bin/tinycode serve
Restart=on-failure
RestartSec=5
Environment=TINYCODE_AUTH_TOKEN=changeme

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tinycode.service
sudo systemctl status tinycode.service
```

Set `TINYCODE_AUTH_TOKEN` before starting -- without it the server auto-generates a token and logs it at startup. Use `TINYCODE_NO_AUTH=1` only for trusted local loopback.

## Container Deployment

Pre-built container images are published to [Quay.io](https://quay.io/repository/bjohns/tinycode-container):

```bash
# Pull the latest image
podman pull quay.io/bjohns/tinycode-container:latest

# Run with Ollama on the host network
podman run -it --network host \
  -e TINYCODE_AUTH_TOKEN=changeme \
  quay.io/bjohns/tinycode-container:latest

# Run with a remote provider endpoint
podman run -it -p 8080:8080 \
  -e TINYCODE_AUTH_TOKEN=changeme \
  quay.io/bjohns/tinycode-container:latest
```

Images are also mirrored to `ghcr.io/bobbyjohnstx/tinycode-container`. Both registries receive identical multi-arch builds (amd64 + arm64).

### Custom Dockerfile

If you need a custom image:

```dockerfile
FROM scratch
COPY dist/tinycode /tinycode
ENTRYPOINT ["/tinycode", "serve"]
```

Build with:

```bash
GOOS=linux GOARCH=amd64 make build
podman build -t my-tinycode .
```

Note: The `FROM scratch` image has no shell, package manager, or CA certificates. If tinycode needs to make HTTPS calls to external providers, use a base image that includes CA certificates (e.g., `FROM alpine:latest` with `apk add --no-cache ca-certificates`).

## OpenShift / Kubernetes Deployment

For production cluster deployments, the recommended approach is the **tinycode-operator** -- it manages `TinycodeInstance` custom resources and handles deployment, storage, routing, and security context automatically.

See the [tinycode-operator repository](https://github.com/bobbyjohnstx/tinycode-operator) and [RHOAI cluster setup guide](https://github.com/bobbyjohnstx/tinycode-operator/blob/main/docs/rhoai-cluster-setup.md) for full documentation.

**Common deployment patterns:**

| Pattern | What it enables | Key spec fields |
|---------|----------------|-----------------|
| **Code assistant** | Edit files in a cloned repo, run tests, commit changes | `spec.git.url` |
| **Cluster operator** | Manage OpenShift resources, debug pods, review logs | `spec.clusterAdmin.enabled` |
| **Both** | Full-stack work: edit code AND deploy to the cluster | `spec.git` + `spec.clusterAdmin` |
| **Team workspace** | Multiple users share a project on RWX storage | `spec.storage.projectsAccessMode: ReadWriteMany` |

## Configuration

tinycode reads configuration from `~/.config/tinycode/config.json` (JSONC format). Key settings for deployment:

| Setting | Description |
|---------|-------------|
| `server.hostname` | Bind address (default: `127.0.0.1`) |
| `enabled_providers` | Whitelist of provider names to discover |
| `disabled_providers` | Blacklist of provider names to skip |

For model and provider configuration details, see [Model Compatibility](model-compatibility.md).
