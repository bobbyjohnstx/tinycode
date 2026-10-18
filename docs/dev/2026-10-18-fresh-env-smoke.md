# Fresh-env smoke (pre-release)

Automated new-user path for plugins + LSP before a release.

## What it covers

1. `tinycode doctor` on an empty HOME (plugin install tips)
2. `tinycode plugin install <name> --from` with a locally built binary
3. Doctor configured-vs-installed + gopls relevance with `go.mod`
4. Headless `tinycode run` against a mock LLM that forces `lsp_diagnostics`
5. Optional TUI smoke via tmux (`--skip-tui` to omit) — shell-launched like `tui-test.sh`,
   polls for prompt chrome, asserts placeholder clears on type + `history:` after Enter
6. Optional GitHub release plugin install (`--release-install`) — no `--from`, hits real assets

## Run

```bash
./script/test-fresh-env.sh                              # full (incl. TUI)
./script/test-fresh-env.sh --skip-tui                   # headless only (CI default)
./script/test-fresh-env.sh --skip-build                 # reuse dist/fresh-env/* if present
./script/test-fresh-env.sh --skip-tui --release-install # + download from GitHub release
```

Requires **podman or docker**, **go**, and **jq**. The harness cross-compiles
`linux/$arch` binaries into `dist/fresh-env/` (host macOS Mach-O cannot run in
the container).

| Env | Purpose |
|-----|---------|
| `FRESH_ENV_ENGINE` | `podman` or `docker` (auto-detects) |
| `FRESH_ENV_NETWORK=host` | Share host network (CI: mock at `127.0.0.1:18080`) |
| `FRESH_ENV_HOST_GW` | Override host gateway hostname |
| `FRESH_ENV_PLUGIN_RELEASE` | Tag for `--release-install` (default `v2.4.1`) |

Results land in `.fresh-env-results/` (`summary.json`, doctor dumps, NDJSON).

## CI

- GitHub: `.github/workflows/fresh-env.yml` (PR/push path filters + weekly cron)
- Gitea: `fresh-env` job in `.gitea/workflows/ci.yml`

Both run `./script/test-fresh-env.sh --skip-tui --release-install` with
`FRESH_ENV_ENGINE=docker` and `FRESH_ENV_NETWORK=host`.

## Design notes

- Mock LLM (`script/mock-llm`) speaks **SSE** — tinycode always streams.
- Mock binds `0.0.0.0` so host-gateway / host-network can reach it.
- Container uses `bash -c` (not `-lc`) so `/etc/profile` does not wipe
  `PATH` and hide `/go/bin/gopls`.
- Release install runs in an empty `/work/project` so `attemptPluginInstall`
  cannot fall back to building `cmd/plugin-*` from a source checkout.
