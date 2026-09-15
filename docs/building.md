# Building tinycode

## Prerequisites

- **Go 1.27+** (`go version` -- see `go.mod` for exact version)
- **make**
- **git**

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
| `test-verbose` | Run all tests with verbose output |
| `vet` | Run `go vet` |
| `lint` | Run `go vet` (add golangci-lint when configured) |
| `check` | Run vet + tests |
| `build-plugins` | Build all 36 plugin binaries for the current platform |
| `build-full` | Build tinycode + all plugins (`build` + `build-plugins`) |
| `embed-webapp` | Build SolidJS web app and embed into Go binary |
| `clean` | Remove build artifacts |
| `help` | Show all targets with descriptions |

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

The SolidJS web UI from `packages/app` can be embedded into the Go binary for single-file distribution:

```bash
make embed-webapp
```

This runs `script/embed-webapp.sh`, which builds the web app and places the output in `internal/static/dist/` for Go's `embed.FS` to include at compile time. Requires the web app dependencies to be installed first (`packages/app` from the legacy TypeScript repo).

After embedding, `make build` produces a binary that serves the web UI without external files. Without embedding, the binary still works -- it just does not serve a web UI unless `TINYCODE_WEB_DIR` points to a directory with built web assets.

## Plugin Binaries

Plugins are standalone Go binaries in `cmd/plugin-*/`. Build an individual plugin:

```bash
go build -o dist/plugin-notify ./cmd/plugin-notify
```

Available plugins:

| Plugin | Description |
|---|---|
| `plugin-cluster-ops` | Kubernetes/OpenShift cluster operations |
| `plugin-code-review` | Git diff for code review |
| `plugin-command-inject` | Custom command injection |
| `plugin-context-pruning` | Context window pruning |
| `plugin-handoff` | Session handoff between agents |
| `plugin-log-sanitizer` | Sanitize sensitive data from logs |
| `plugin-notify` | Desktop notifications |
| `plugin-pilot` | Git forge integration (Gitea, GitHub, GitLab) |
| `plugin-safety-net` | Safety checks before destructive operations |
| `plugin-snippets` | Code snippet management |
| `plugin-telemetry` | Usage telemetry |
| `plugin-web-search` | Web search via Exa API |
| `plugin-aap-bridge` | Ansible Automation Platform bridge |
| `plugin-container-linter` | Container/Dockerfile linting |
| `plugin-eda-events` | Event-Driven Ansible event integration |
| `plugin-lightwell` | Lightwell data pipeline integration |
| `plugin-ocp-context-injection` | OpenShift context injection into sessions |
| `plugin-ocp-oauth` | OpenShift OAuth token management |
| `plugin-ocp-obs-logging` | OpenShift observability logging |
| `plugin-ocp-obs-metrics` | OpenShift observability metrics |
| `plugin-quay` | Quay container registry operations |
| `plugin-rh-api-catalog` | Red Hat API catalog discovery |
| `plugin-rh-dev-content` | Red Hat developer content integration |
| `plugin-rh-ecosystem-catalog` | Red Hat ecosystem catalog lookups |
| `plugin-rhacm` | Red Hat Advanced Cluster Management |
| `plugin-rhacs` | Red Hat Advanced Cluster Security |
| `plugin-rhdh` | Red Hat Developer Hub integration |
| `plugin-rhdp-provisioner` | Red Hat Developer Platform provisioning |
| `plugin-rhoai-eval-trustyai` | RHOAI TrustyAI model evaluation |
| `plugin-rhoai-experiment-tracker` | RHOAI experiment tracking |
| `plugin-rhoai-mcp-bridge` | RHOAI Model Context Protocol bridge |
| `plugin-rhoai-mlflow-tools` | RHOAI MLflow tooling |
| `plugin-rhoai-model-serving` | RHOAI model serving management |
| `plugin-rhoai-pipelines` | RHOAI pipeline orchestration |
| `plugin-satellite` | Red Hat Satellite administration |
| `plugin-tekton` | Tekton pipeline operations |

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
./script/test-plugins-container.sh ocp-oauth
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

Currently runs `go vet ./...`. When golangci-lint is configured, `make lint` will include it.

## Clean

```bash
make clean
```

Removes `dist/` and runs `go clean`.
