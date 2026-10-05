# Building tinycode

## Prerequisites

- **Go 1.27+** (`go version` -- see `go.mod` for exact version)
- **make**
- **git**

## Repository layout

| Path | Role |
|------|------|
| `cmd/tinycode/` | Main binary |
| `cmd/plugin-*/` | Optional plugin binaries (30 plugins) |
| `internal/` | Private application code |
| `pkg/plugin/` | Public plugin SDK |
| `packages/` | **Legacy TypeScript** — used only when building the optional embedded web UI (`make embed-webapp`). Not part of the Go runtime. |

Agent and contributor guidance for Go lives in the repo root: `AGENTS.md` and `CLAUDE.md`. See [spec/README.md](spec/README.md) for a full documentation index.

## Quick Build

```bash
make build
```

Output: `dist/tinycode`

Run it:

```bash
./dist/tinycode              # TUI mode
./dist/tinycode serve        # headless API server
./dist/tinycode web          # server + web UI in browser
./dist/tinycode --version    # print version info
```

## Makefile Targets

| Target | Description |
|---|---|
| `build` | Build for the current platform |
| `build-all` | Cross-compile for all supported platforms |
| `package` | Create release archives for all platforms |
| `test` | Run all tests (`go test ./... -count=1`) |
| `test-race` | Run all tests with the race detector (`-timeout 300s`) |
| `test-verbose` | Run all tests with verbose output |
| `vet` | Run `go vet` |
| `staticcheck` | Run [staticcheck](https://staticcheck.dev/) on `./...` |
| `lint` | Run `go vet` and staticcheck |
| `check` | Run lint + tests |
| `build-plugins` | Build all plugin binaries for the current platform |
| `build-full` | Build tinycode + all plugins (`build` + `build-plugins`) |
| `embed-webapp` | Build SolidJS web app and embed into Go binary |
| `clean` | Remove build artifacts |
| `help` | Show all targets with descriptions |

Cross-compilation and plugin tables below refer to the same **30** `cmd/plugin-*` directories as `make build-plugins`.

## Cross-Compilation

```bash
make build-all
```

Builds for all supported platforms:

| OS | Architecture | Output |
|---|---|---|
| linux | amd64 | `dist/tinycode-linux-amd64` |
| linux | arm64 | `dist/tinycode-linux-arm64` |
| darwin | amd64 | `dist/tinycode-darwin-amd64` |
| darwin | arm64 | `dist/tinycode-darwin-arm64` |
| windows | amd64 | `dist/tinycode-windows-amd64.exe` |

To create release archives (`.tar.gz` for Unix, `.zip` for Windows):

```bash
make package
```

Output goes to `dist/release/`.

## Version Injection

The Makefile injects version, commit, and build date via `-ldflags -X` flags:

```makefile
LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.date=$(DATE)
```

- `VERSION` defaults to `git describe --tags --always --dirty` (or `dev` if no tags)
- `COMMIT` defaults to `git rev-parse --short HEAD`
- `DATE` defaults to the current UTC timestamp

Override at build time:

```bash
make build VERSION=v1.2.3 COMMIT=abc1234 DATE=2026-01-01T00:00:00Z
```

## Embedding the Web App

The SolidJS web UI from `packages/app` can be embedded into the Go binary for single-file distribution. **`packages/` is not required** for TUI, `serve`, `run`, or `acp` — only for this optional embed step.

```bash
make embed-webapp
```

This runs `script/embed-webapp.sh`, which builds the web app (with `TINYCODE_CHANNEL=prod` by default) and places the output in `internal/static/dist/` for Go's `embed.FS` to include at compile time. Requires Node.js and the web app dependencies under `packages/app` (legacy TypeScript tree).

After embedding, `make build` produces a binary that serves the web UI without external files via `tinycode web`. Without embedding, the binary still works -- it just does not serve a web UI unless `TINYCODE_WEB_DIR` points to a directory with built web assets.

**Desktop packaging:** The supported GUI path is `tinycode web` (embedded SolidJS SPA). The Electron shell in `packages/desktop` is experimental and unsupported for the Go product; a Go-native desktop app is not planned. See [spec/16-not-implemented.md](spec/16-not-implemented.md) §16.1.

**Not in Go:** The original TypeScript **tmux swarm** (multi-pane workers) is documented in [spec/16-not-implemented.md](spec/16-not-implemented.md). The Go binary implements goroutine-based `/swarm` and the `task` tool instead.

## Plugin Binaries

Plugins are standalone Go binaries in `cmd/plugin-*/`. Build an individual plugin:

```bash
go build -o dist/plugin-safety-net ./cmd/plugin-safety-net
```

notify, code-review, handoff, and context-pruning are in-process builtins, not `cmd/plugin-*` binaries. The 30 binaries match `internal/plugin/registry.go`:

| Plugin | Description |
|---|---|
| `plugin-aap-bridge` | Ansible Automation Platform bridge (job templates, inventories, lint) |
| `plugin-audit-logs` | API audit log analysis |
| `plugin-container-linter` | Containerfile linting, bootc validation, UBI base image suggestions |
| `plugin-etcd-diag` | etcd diagnostics and snapshot inspection |
| `plugin-ingress-inspect` | HAProxy/Ingress inspection |
| `plugin-insights` | OpenShift Insights archive analysis |
| `plugin-lightwell` | Red Hat Lightwell package security |
| `plugin-log-sanitizer` | Sanitize sensitive data from logs |
| `plugin-ocp-context-injection` | OpenShift cluster context injection |
| `plugin-ocp-must-gather` | Must-gather offline analysis |
| `plugin-ocp-obs-logging` | OpenShift observability logging (Loki, Tempo, NetObserv) |
| `plugin-ocp-obs-metrics` | OpenShift observability metrics (PromQL, alerts, silencing) |
| `plugin-ocp-odf` | OpenShift Data Foundation storage health |
| `plugin-ocp-virt` | OpenShift Virtualization VM lifecycle |
| `plugin-pilot` | Autonomous agent pilot mode |
| `plugin-quay` | Quay container registry |
| `plugin-rh-api-catalog` | Red Hat API catalog |
| `plugin-rh-dev-content` | Red Hat developer content |
| `plugin-rh-ecosystem-catalog` | Red Hat ecosystem catalog (Pyxis) |
| `plugin-rhacm` | Red Hat ACM fleet management |
| `plugin-rhacs` | Red Hat ACS security |
| `plugin-rhdh` | Red Hat Developer Hub |
| `plugin-rhdp-provisioner` | Red Hat Developer Platform provisioner |
| `plugin-rhoai-mlflow` | MLflow experiment tracking and model registry |
| `plugin-rhoai-pipelines` | RHOAI data science pipelines |
| `plugin-rhoai-serving` | RHOAI model serving, evaluation, and TrustyAI |
| `plugin-safety-net` | Pre-execution safety checks for destructive commands |
| `plugin-satellite` | Red Hat Satellite |
| `plugin-tekton` | Tekton pipelines |
| `plugin-telemetry` | Usage telemetry and analytics |

Build all plugins:

```bash
make build-plugins
```

Or build tinycode and all plugins together:

```bash
make build-full
```

To build a single plugin manually:

```bash
for dir in cmd/plugin-*/; do
  name=$(basename "$dir")
  go build -o "dist/$name" "./$dir"
done
```

### Container Plugin Testing

`script/test-plugins-container.sh` cross-compiles all plugins for `linux/amd64`, copies them into a disposable UBI9 podman container, and verifies each binary responds to the JSON-RPC `initialize` handshake. This catches linking issues, missing symbols, and startup crashes without requiring a Linux host.

```bash
# Test all plugins
./script/test-plugins-container.sh

# Test a single plugin
./script/test-plugins-container.sh safety-net
```

## Testing

```bash
# All tests
make test

# Single package
go test ./internal/tool/... -count=1

# Verbose
make test-verbose
```

## Linting

```bash
make lint
```

Runs `go vet ./...` and [staticcheck](https://staticcheck.dev/) on `./...`.

## Clean

```bash
make clean
```

Removes `dist/` and runs `go clean`.
