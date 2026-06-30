# Building tinycode

This guide covers building the tinycode project across three deployments: the core project, the container image, and the operator. It captures real-world gotchas and workarounds discovered during development.

---

## Prerequisites

All builds require:
- **Bun 1.3.14+** (`bun --version`)
- **git**

Platform-specific:
- **Linux/macOS:** podman or docker
- **GitHub access:** Personal access token in `~/.github-token` (for container builds)
- **Operator local testing:** Python 3.11+, helm 3.x

---

## Part 1: Building tinycode (Core)

### Development Mode

Start developing without building a binary:

```bash
# Install dependencies (from repo root)
cd /path/to/tinycode
bun install

# Terminal UI mode (default)
bun dev
bun dev <directory>   # run against a different directory
bun dev .             # run against repo root

# Headless API server (port 4096)
bun dev serve

# Server + web UI (opens in browser)
bun dev web

# Web UI only (requires server running separately)
bun run --cwd packages/app dev
```

### Standalone Binary

Build a self-contained executable for your current platform:

```bash
cd /path/to/tinycode
./packages/tinycode/script/build.ts --single
```

**Output:** `packages/tinycode/dist/tinycode-<platform>-<arch>/bin/tinycode`

**Gotcha:** The `--single` flag builds only for your current platform. To build all platforms (linux x64/arm64, macOS x64/arm64, Windows x64/arm64), omit `--single`:

```bash
# WARNING: This takes 10+ minutes and requires ~5GB disk
./packages/tinycode/script/build.ts
```

**Verify:** Run the binary to check the version:

```bash
./packages/tinycode/dist/tinycode-linux-x64/bin/tinycode --version
```

**Smoke test:** The build script automatically runs a smoke test for binaries matching your platform. If the test fails, the build exits with code 1.

### Web UI Embedding

The standalone binary includes an embedded web UI. The build script handles this automatically:

```bash
# Web UI is built and embedded into the binary during build.ts
# Source: packages/app/ → dist/ → tinycode-web-ui.gen.ts → compiled into binary

# To rebuild the web UI without the binary:
bun run --cwd packages/app build
```

**Gotcha — dist not committed:** `packages/app/dist/` is `.gitignore`d. After pulling code changes:
- Either rebuild the app: `bun run --cwd packages/app build`
- Or run the web UI in dev mode: `bun dev web` (rebuilds automatically)

The standalone binary rebuild will handle this; `bun dev web` requires it pre-built.

### SDK Regeneration

The TypeScript SDK (`packages/sdk/js/`) is auto-generated from the OpenAPI spec. Regenerate it after any API changes (server.ts modifications):

```bash
cd /path/to/tinycode
./packages/sdk/js/script/build.ts
```

This script:
1. Spins up the dev server and calls `bun dev generate`
2. Captures the OpenAPI JSON
3. Uses `@hey-api/openapi-ts` to generate TypeScript client code

**Gotcha — must run from repo root:** The SDK build script references `../../tinycode`, so it expects to be called from the workspace root.

### Tests

Tests must run from a package directory, never from the repo root (enforced by guard):

```bash
# Correct: cd into the package first
cd packages/tinycode
bun test

# Run single test file with custom timeout (migrations can be slow)
bun test --timeout 30000 test/skill/skill.test.ts

# WRONG: Will fail with "do-not-run-tests-from-root"
cd /path/to/tinycode && bun test
```

### Type Checking

Type check a single package (not the whole workspace):

```bash
cd packages/tinycode
bun typecheck
```

### Linting

Lint the entire workspace from the repo root:

```bash
cd /path/to/tinycode
bun run lint   # Uses oxlint
```

---

## Part 2: Building tinycode-container

### Overview

The container bundles:
1. **tinycode binary** (compiled from source)
2. **oh-my-tiny plugin** (native agent plugin)
3. **entrypoint.sh** (startup configuration)
4. **Default agents & skills** (bundled configs)

### Prerequisites

```bash
# Install podman or docker
podman --version

# Create GitHub token file (needed for private oh-my-tiny repo)
cat > ~/.github-token << 'EOF'
ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
EOF
chmod 600 ~/.github-token
```

The token needs access to:
- `bobbyjohnstx/tinycode` (public, but used for CI builds)
- `bobbyjohnstx/oh-my-tiny` (private)

### CI Build (from GitHub)

This build clones tinycode and oh-my-tiny from GitHub at build time. **Your local changes are NOT included unless pushed.** This is intentional for reproducible CI builds.

```bash
cd /private/tmp/tinycode-container

podman build \
  --platform linux/amd64 \
  -f ContainerFile \
  --build-arg OMT_REF=master \
  --secret id=github_token,src=$HOME/.github-token \
  -t quay.io/bjohns/tinycode-container:latest \
  .
```

**Build arguments:**
- `TINYCODE_REF=main` (default) — GitHub branch/commit to clone
- `OMT_REF=master` (default) — oh-my-tiny branch. Note: NOT `main`, but `master`

**Key gotchas:**

1. **Branch drift (CRITICAL):**
   - tinycode uses `dev` as the working branch
   - GitHub default branch is `main`
   - Changes are committed to `dev` but containers build from `main`
   - **Always sync before building:**
     ```bash
     cd /path/to/tinycode
     git push github dev:main
     ```

2. **Podman layer caching:**
   - Podman aggressively caches `RUN git clone` layers
   - If code changes don't appear in the built image, use `--no-cache`:
     ```bash
     podman build --no-cache --platform linux/amd64 -f ContainerFile ...
     ```

3. **ARM Mac building for amd64 clusters:**
   - OpenShift clusters run `linux/amd64`
   - `--platform linux/amd64` on ARM Mac uses QEMU emulation (slower)
   - Always include `--platform linux/amd64` for cluster deployments

4. **GitHub token secrets:**
   - `--secret` is not baked into the image layers
   - `RUN --mount=type=secret,id=github_token` mounts it at build time only
   - Safe to pass; not included in final image

### Local Build (from local source)

For rapid iteration without pushing to GitHub:

```bash
cd /private/tmp/tinycode-container
./build-local.sh
```

What `build-local.sh` does:
1. Creates a temp build context
2. Copies tinycode source (excludes `node_modules`, `dist`, `.git` for size)
3. Copies oh-my-tiny source
4. Copies `ContainerFile.local`, `entrypoint.sh`, and `config/`
5. Runs `podman build -f ContainerFile.local`

**Environment variables (optional):**

```bash
# Use custom source directories
TINYCODE_SRC=/path/to/tinycode \
OH_MY_TINY_SRC=/path/to/oh-my-tiny \
IMAGE_TAG=my-custom-tag:local \
BUILD_PLATFORM=linux/amd64 \
./build-local.sh
```

**Default paths:**
- `TINYCODE_SRC=/Users/bjohns/projects/tinycode`
- `OH_MY_TINY_SRC=/Users/bjohns/projects/oh-my-tiny`
- `IMAGE_TAG=tinycode-container:local`

### Pushing to Registry

```bash
podman push quay.io/bjohns/tinycode-container:latest
```

For multi-platform images, use buildx or separate builds with `--platform`.

### Deploying to a Kubernetes Cluster

After pushing a new image, force the cluster to pull it (Kubernetes caches `:latest`):

```bash
# Step 1: Set imagePullPolicy to Always (one-time)
oc patch deployment tinycode-tinycode -n tinycode-dev \
  --type json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Always"}]'

# Step 2: Delete running pods to trigger a fresh pull
oc delete pod -l app.kubernetes.io/instance=tinycode -n tinycode-dev

# Step 3: Monitor rollout
oc rollout status deployment tinycode-tinycode -n tinycode-dev --timeout=120s
```

Alternatively, use a unique tag instead of `:latest`:

```bash
podman tag tinycode-container:local quay.io/bjohns/tinycode-container:v0.1.2
podman push quay.io/bjohns/tinycode-container:v0.1.2

# Update deployment to use v0.1.2 (no imagePullPolicy workaround needed)
oc set image deployment/tinycode-tinycode \
  tinycode=quay.io/bjohns/tinycode-container:v0.1.2 \
  -n tinycode-dev
```

### Verifying the Build

Check that your code changes made it into the image:

```bash
# Check version and build time
podman run --rm --platform linux/amd64 --entrypoint tinycode \
  quay.io/bjohns/tinycode-container:latest \
  --version

# Check for a specific code change (limited to unminified strings)
podman run --rm --platform linux/amd64 --entrypoint sh \
  quay.io/bjohns/tinycode-container:latest \
  -c 'strings /usr/local/bin/tinycode | grep -i "your-search-term" || echo "not found (may be minified)"'

# Start container and test the API
podman run -d --name tinycode-test -p 4096:4096 \
  quay.io/bjohns/tinycode-container:latest
curl -s http://localhost:4096/global/health | jq .
podman stop tinycode-test && podman rm tinycode-test
```

---

## Part 3: Building tinycode-operator

The operator is a Python Kopf application that runs on OpenShift/Kubernetes to manage tinycode instances via a Helm chart.

### Building the Operator Image

```bash
cd /private/tmp/tinycode-operator

podman build \
  --platform linux/amd64 \
  -f Dockerfile \
  -t quay.io/bjohns/tinycode-operator:latest \
  .
```

**What the build does:**
1. **Stage 1 (builder):** Installs Python 3.11 dependencies from `operator/requirements.txt`
2. **Stage 2 (runtime):** Minimal UBI9 image + helm binary + operator source + Helm chart
3. Architecture is auto-detected: `uname -m` maps `x86_64` → `amd64`, `aarch64` → `arm64`

**Gotchas:**

1. **Rebuild after code changes:**
   - `operator/main.py` is copied into the image at build time
   - Changes require rebuild; `--no-cache` forces a full rebuild:
     ```bash
     podman build --no-cache --platform linux/amd64 -f Dockerfile ...
     ```

2. **Rebuild after Helm chart changes:**
   - `helm-charts/tinycode/` is copied at build time
   - Use `--no-cache` after updating the chart

3. **Platform architecture:**
   - Always include `--platform linux/amd64` for OpenShift clusters
   - The Dockerfile auto-detects architecture and downloads the correct Helm binary

### Pushing to Registry

```bash
podman push quay.io/bjohns/tinycode-operator:latest
```

### Deploying to Cluster

After pushing, update the running deployment:

```bash
# Step 1: Set imagePullPolicy to Always (one-time)
oc patch deployment tinycode-operator-manager -n tinycode-operator-system \
  --type json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Always"}]'

# Step 2: Restart the operator to pull new image
oc rollout restart deployment/tinycode-operator-manager -n tinycode-operator-system

# Step 3: Monitor rollout
oc rollout status deployment/tinycode-operator-manager -n tinycode-operator-system --timeout=120s

# Step 4: Clear values hash on instances to trigger reconciliation
oc annotate tinycodeinstance <instance-name> tinycode.dev/values-hash- -n <namespace>
```

### Local Testing (without a cluster)

```bash
# Syntax check
python -m py_compile operator/main.py

# Unit tests (if available)
pytest operator/ -v

# Helm chart validation
helm lint helm-charts/tinycode/

# Manual operator run (requires kubeconfig + CRDs + kopf 1.41.0+)
# (Not recommended — cluster testing is more reliable)
```

### Helm Chart Structure

The operator ships with a Helm chart at `helm-charts/tinycode/`. The chart is:
- Copied into the image at build time
- Used by the operator to manage tinycode instances
- Installed on the cluster via `TinycodeInstance` custom resources

After chart updates, rebuild the operator image.

---

## Common Gotchas (All Builds)

### Branch Drift

**Problem:** You edit files in `dev`, but container builds pull from `main`.

**Solution:**
```bash
# After committing changes to dev, sync to main before building containers
cd /path/to/tinycode
git push github dev:main

# Verify
git log -n 1 github/main
```

### Podman Cache + Git Clones

**Problem:** You push code to GitHub, build with `podman build`, but the old code is in the image.

**Cause:** Podman caches `RUN git clone` layers. The layer was cached before your push.

**Solution:**
```bash
# After pushing code changes, always use --no-cache
podman build --no-cache --platform linux/amd64 -f ContainerFile ...
```

### imagePullPolicy on Kubernetes

**Problem:** You push a new image with the same tag (`:latest`), but the cluster still runs the old one.

**Cause:** Kubernetes defaults to `imagePullPolicy: IfNotPresent`. It caches `:latest` locally and never re-pulls.

**Solutions:**
1. **Use unique tags:**
   ```bash
   podman tag tinycode-container:local quay.io/bjohns/tinycode-container:v0.1.2
   podman push quay.io/bjohns/tinycode-container:v0.1.2
   oc set image deployment/tinycode-tinycode tinycode=... -n ns
   ```

2. **Force Always pull:**
   ```bash
   oc patch deployment tinycode-tinycode -n ns \
     --type json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Always"}]'
   oc delete pod -l app.kubernetes.io/instance=tinycode -n ns
   ```

### Building for amd64 on ARM Mac

**Problem:** You're on an ARM Mac (M1/M2/M3), but your cluster runs `linux/amd64`.

**Solution:** Use QEMU emulation (slower but correct):
```bash
# This works on ARM Mac and emulates x86_64
podman build --platform linux/amd64 -f ContainerFile ...
```

The build will be slower (QEMU emulation), but the binary is correct for the cluster.

### Test Guard at Repo Root

**Problem:** Running `bun test` from the repo root fails with an error.

**Cause:** `package.json` has a guard to prevent repo-root test runs (turborepo complexity).

**Solution:** Always `cd` into a package first:
```bash
cd packages/tinycode
bun test
```

### Web UI Not Updating in Standalone Binary

**Problem:** You changed code in `packages/app`, rebuilt the binary, but the UI didn't update.

**Cause:** The build script only rebuilds the web UI if it's missing. It doesn't invalidate cache after source changes.

**Solution:**
```bash
# Delete the web UI dist and rebuild
rm -rf packages/app/dist
./packages/tinycode/script/build.ts --single
```

Or use `bun dev web` to test the UI without rebuilding the binary.

---

## Build Artifact Locations

| Artifact                                      | Location                                                   |
| --------------------------------------------- | ---------------------------------------------------------- |
| tinycode binary (single platform)             | `packages/tinycode/dist/tinycode-<os>-<arch>/bin/tinycode` |
| Web UI (embedded)                             | `packages/app/dist/` (built into binary)                  |
| TypeScript SDK (auto-generated)               | `packages/sdk/js/src/v2/gen/`                             |
| Container image (local)                       | `tinycode-container:local` or custom tag                   |
| Container image (registry)                    | `quay.io/bjohns/tinycode-container:latest`                |
| Operator image (registry)                     | `quay.io/bjohns/tinycode-operator:latest`                 |

---

## Troubleshooting

### Build fails with "permission denied" in container

**Cause:** Running `podman` without proper permission (common on some Linux distros).

**Solution:**
```bash
# Add current user to podman group
sudo usermod -aG podman $USER
newgrp podman

# Or use sudo
sudo podman build ...
```

### "git clone" hangs during container build

**Cause:** Network timeout or GitHub authentication issue.

**Solution:**
```bash
# Verify GitHub token is valid
cat ~/.github-token | xargs -I {} curl -H "Authorization: token {}" https://api.github.com/user

# Rebuild with --no-cache and higher timeout
podman build --no-cache --timeout 300 ...
```

### SDK generation fails with "Cannot find bun dev generate"

**Cause:** SDK build script must run from repo root (where it can reach `../../tinycode`).

**Solution:**
```bash
cd /path/to/tinycode
./packages/sdk/js/script/build.ts
```

### Smoke test fails after binary build

**Cause:** Binary is corrupted or incompatible with current platform.

**Solution:**
```bash
# Check if binary can run at all
./packages/tinycode/dist/tinycode-linux-x64/bin/tinycode --help

# If it segfaults, rebuild from scratch without cache
./packages/tinycode/script/build.ts --single
```

---

## Next Steps

- **Deploy container:** See the Kubernetes deployment guides in `docs/`
- **Configure tinycode:** Edit `~/.config/tinycode/config.json`
- **Test locally:** `bun dev` or `bun dev web`
- **Contributing:** Follow the style guide in `AGENTS.md` and test guidelines in `~/.claude/rules/testing.md`
