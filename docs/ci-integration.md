# CI Integration (GitHub Actions)

Use `tinycode run` directly in GitHub Actions for CI-driven agent loops. There is no official GitHub Action package — install the binary and invoke `tinycode run` in workflow YAML. That keeps CI local-first: your workflow owns the model, secrets, and permissions.

For full run-mode flags and NDJSON event types, see [Run Mode](user-guide.md#run-mode) in the user guide.

## Prerequisites

- A model tinycode can reach from the runner (cloud API or a reachable OpenAI-compatible endpoint)
- Repository secrets for API keys and the model id (see [Secrets](#secrets))
- A Linux or macOS runner (`ubuntu-latest` is typical)

Install the binary once per job (same installer as local use):

```yaml
- name: Install tinycode
  run: |
    curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh
    echo "$HOME/.local/bin" >> "$GITHUB_PATH"
```

## Basic PR workflow

Review a pull request when it opens or updates. Redirect stdin from `/dev/null` so the job does not hang waiting for input.

```yaml
name: tinycode PR review

on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install tinycode
        run: |
          curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"

      - name: Run tinycode
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
        run: |
          tinycode run --format json \
            --dangerously-skip-permissions \
            -m "${{ secrets.MODEL }}" \
            "Review this PR for issues. Summarize findings clearly." \
            < /dev/null \
            | tee tinycode-out.ndjson
```

Without `--dangerously-skip-permissions`, Ask-level tools (shell, edit, and similar) are auto-rejected in headless mode — fine for read-only prompts, insufficient if the agent must change files or run commands.

## Comment-triggered `/tinycode`

Respond when someone posts a PR comment that starts with `/tinycode`:

```yaml
name: tinycode comment

on:
  issue_comment:
    types: [created]

permissions:
  contents: read
  pull-requests: write
  issues: write

jobs:
  run:
    if: >
      github.event.issue.pull_request &&
      startsWith(github.event.comment.body, '/tinycode')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: refs/pull/${{ github.event.issue.number }}/head

      - name: Install tinycode
        run: |
          curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"

      - name: Run tinycode from comment
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
          PROMPT: ${{ github.event.comment.body }}
        run: |
          # Strip the leading /tinycode token; rest is the prompt
          PROMPT_TEXT="${PROMPT#/tinycode}"
          PROMPT_TEXT="${PROMPT_TEXT# }"
          if [ -z "$PROMPT_TEXT" ]; then
            PROMPT_TEXT="Review this pull request and summarize findings."
          fi

          tinycode run --format json \
            --dangerously-skip-permissions \
            -m "${{ secrets.MODEL }}" \
            "$PROMPT_TEXT" \
            < /dev/null \
            | tee tinycode-out.ndjson

      - name: Post reply from JSON output
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          # Concatenate model text deltas into one comment body
          REPLY=$(jq -r 'select(.type == "text") | .text // empty' tinycode-out.ndjson | tr -d '\n')
          if [ -z "$REPLY" ]; then
            REPLY=$(jq -r 'select(.type == "error") | .message // "tinycode produced no text output"' tinycode-out.ndjson | tail -n1)
          fi
          gh pr comment "${{ github.event.issue.number }}" --body "$REPLY"
```

The workflow calls `tinycode run` only — no custom Action marketplace package.

## Secrets

Store credentials and the model id in GitHub Actions secrets (Settings → Secrets and variables → Actions). Do not hardcode keys in YAML.

| Secret | Purpose |
|--------|---------|
| `OPENROUTER_API_KEY` | Enables OpenRouter discovery/auth (common cloud path) |
| `MODEL` | Model id passed to `-m` (for example `openrouter/anthropic/claude-sonnet-4`) |

Other providers:

| Env / secret | Notes |
|--------------|--------|
| `TINYCODE_OLLAMA_HOST` / `OLLAMA_HOST` | Reachable Ollama base URL (self-hosted runner or network path) |
| `TINYCODE_VLLM_HOST` | vLLM OpenAI-compatible endpoint |
| `TINYCODE_LMSTUDIO_HOST` | LM Studio endpoint |
| `TINYCODE_MAAS_API_KEY` | MaaS auth when using that provider |

Wire secrets into the step `env:` block so tinycode can discover the provider at startup:

```yaml
env:
  OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
```

Model selection:

```bash
tinycode run -m "${{ secrets.MODEL }}" "…"
```

Or pin a literal model in the workflow if it is not sensitive (prefer a secret when the id encodes a paid route).

## `--dangerously-skip-permissions` caveats

In CI, `--dangerously-skip-permissions` auto-approves every tool permission (shell, write, network, and so on). Use it only when you intentionally want unattended tool execution.

Risks:

- The model can edit the workspace and run shell commands without a human gate
- Compromised prompts (for example from untrusted PR content or comments) can drive destructive tool use
- Secrets available to the job process may be readable by tools the agent invokes

Mitigations:

- Prefer read-only prompts without the flag when tools are unnecessary
- Scope `permissions:` tightly (`contents: read` unless the job must push)
- Restrict which events and actors can trigger agent jobs (for example require a collaborator comment)
- Prefer `--safe-mode` when plugins/MCP/user agents are not needed
- Cap cost with `--max-tokens` and `--max-iterations`
- Run on `pull_request` from forks only with secrets policies you understand (fork PRs often cannot access repository secrets)

Default headless behavior without the flag: allowed rules such as `read *` still pass; Ask requests are rejected. Interactive (`-i`) and JSON permission protocols are a poor fit for most Actions jobs.

## JSON output parsing

`--format json` emits NDJSON (one JSON object per line). Useful event types for CI:

| `type` | Use in CI |
|--------|-----------|
| `session` | Capture `sessionID` for logs or follow-up runs |
| `text` | Append `text` deltas to build the assistant reply |
| `tool_begin` / `tool_end` | Optional progress / failure diagnostics |
| `done` | Success when `ok` is true |
| `error` | Failure message |

Example: collect the reply and fail the job if the run did not finish cleanly:

```bash
tinycode run --format json \
  --dangerously-skip-permissions \
  -m "${{ secrets.MODEL }}" \
  "Summarize the diff risk in this checkout." \
  < /dev/null \
  | tee tinycode-out.ndjson

REPLY=$(jq -r 'select(.type == "text") | .text // empty' tinycode-out.ndjson | tr -d '\n')
OK=$(jq -r 'select(.type == "done") | .ok' tinycode-out.ndjson | tail -n1)

echo "$REPLY"
if [ "$OK" != "true" ]; then
  jq -r 'select(.type == "error") | .message // empty' tinycode-out.ndjson >&2
  exit 1
fi
```

Post that reply with `gh pr comment` (as in the comment-triggered example) or `gh api` if you prefer raw REST.

Text format (`--format default`, the default) is fine for logs; prefer `--format json` whenever another step must parse the answer.

## Further reading

- [Run Mode](user-guide.md#run-mode) — flags, permissions, multi-turn NDJSON protocol
- [Install](install.md) — installer env vars and platforms
- [Cheatsheet](cheatsheet.md) — short `tinycode run` examples
