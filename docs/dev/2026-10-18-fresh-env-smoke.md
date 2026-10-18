# Fresh-env smoke (pre-release)

Automated new-user path for plugins + LSP before a release.

## What it covers

1. `tinycode doctor` on an empty HOME (plugin install tips)
2. `tinycode plugin install <name> --from` with a locally built binary
3. Doctor configured-vs-installed + gopls relevance with `go.mod`
4. Headless `tinycode run` against a mock LLM that forces `lsp_diagnostics`
5. Optional TUI smoke via tmux (`--skip-tui` to omit)

## Run

```bash
./script/test-fresh-env.sh              # full
./script/test-fresh-env.sh --skip-tui   # headless only
./script/test-fresh-env.sh --skip-build # reuse dist/fresh-env/* if present
```

Requires **podman** and **go**. The harness cross-compiles `linux/$arch`
binaries into `dist/fresh-env/` (host macOS Mach-O cannot run in the container).

Results land in `.fresh-env-results/` (`summary.json`, doctor dumps, NDJSON).

## Design notes

- Mock LLM (`script/mock-llm`) speaks **SSE** — tinycode always streams.
- Mock binds `0.0.0.0` so `host.containers.internal` can reach it.
- Container uses `bash -c` (not `-lc`) so `/etc/profile` does not wipe
  `PATH` and hide `/go/bin/gopls`.
