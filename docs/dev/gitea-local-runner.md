# Local Gitea Actions runner (Podman-first)

Jobs stay **queued forever** when Gitea is up but no `gitea-runner` daemon is
registered and running. GitHub Actions is separate — green there does not drain
this queue.

## Why it was stuck

Nothing was wrong with the workflow YAML. Homebrew `gitea` was running; there
was simply **no Actions runner** installed (`gitea-runner` / formerly
`act_runner`).

## Stack (this machine)

| Piece | Role |
|-------|------|
| Homebrew `gitea` | Server on `:3000` (`ROOT_URL` = `http://gitea.local:3000/`) |
| Homebrew `gitea-runner` | Claims `.gitea/workflows/*` jobs |
| Podman machine | Container engine — **no Docker Engine / Docker Desktop** |

### Podman, not Docker Engine

`gitea-runner` talks the container API. Point it at the **Podman machine**
socket via `DOCKER_HOST` / `CONTAINER_HOST` (env name is historical). Do **not**
require `brew install docker`.

Label syntax still looks like:

```text
ubuntu-latest:docker://node:20-bookworm
```

The `docker://` prefix is **act_runner vocabulary** for “run this job in a
container image.” Workflows keep `runs-on: ubuntu-latest` — no Docker-branded
runner names.

### Mac + Podman gotchas (fixed in local config)

1. **Do not bind-mount the Mac API sock into Linux job containers**  
   That path is not mountable into the machine VM (`statfs … operation not
   supported`). Use `container.docker_host: "-"` and set `DOCKER_HOST` on the
   runner process instead.

2. **Job containers cannot use `127.0.0.1` for Gitea**  
   Register the runner against `http://gitea.local:3000` and add  
   `container.options: --add-host=gitea.local:host-gateway` so Linux jobs reach
   the host.

## Local config paths

| File | Purpose |
|------|---------|
| `/opt/homebrew/var/gitea/custom/conf/app.ini` | `[actions] ENABLED = true` |
| `/opt/homebrew/etc/gitea-runner/config.yaml` | `docker_host: "-"`, `options: --add-host=…` |
| `/opt/homebrew/var/lib/gitea-runner/.runner` | Registration (instance URL + labels) |
| `~/Library/LaunchAgents/sh.brew.gitea-runner.plist` | `DOCKER_HOST` / `CONTAINER_HOST` → Podman machine sock |

Services:

```bash
brew services list | grep -E 'gitea|runner'
podman machine list   # must be Currently running
```

## Register / restart

```bash
# Ensure Podman machine is up
podman machine start

TOKEN=$(gitea actions generate-runner-token \
  -c /opt/homebrew/var/gitea/custom/conf/app.ini \
  -w /opt/homebrew/var/gitea | tail -1)

rm -f /opt/homebrew/var/lib/gitea-runner/.runner
export DOCKER_HOST=unix://$HOME/.local/share/containers/podman/machine/podman.sock

gitea-runner register --no-interactive \
  --instance "http://gitea.local:3000" \
  --token "$TOKEN" \
  --name "mac-podman" \
  --labels "ubuntu-latest:docker://node:20-bookworm,ubuntu-22.04:docker://node:20-bookworm,podman-host:host" \
  -c /opt/homebrew/etc/gitea-runner/config.yaml

# `podman-host:host` is for jobs that need the Mac Podman engine directly
# (e.g. fresh-env nested containers). Other jobs keep ubuntu-latest.
brew services restart gitea-runner
# Re-apply DOCKER_HOST in the LaunchAgent if brew overwrote the plist, then:
launchctl bootout "gui/$(id -u)/sh.brew.gitea-runner" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/sh.brew.gitea-runner.plist
```

## Sanity checks

```bash
tail -f /opt/homebrew/var/log/gitea-runner.err
# expect: declare successfully, then task started …

cd /path/to/tinycode && tea actions runs list --limit 5

# From a job-like container, host Gitea must be reachable:
podman run --rm --add-host=gitea.local:host-gateway alpine \
  wget -qO- -T3 http://gitea.local:3000/ >/dev/null && echo OK
```
