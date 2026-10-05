> **Historical document (September 2026).** This analysis predates v2.1 and the two-round code review (60+ fixes). Many findings are now resolved. See README.md and CHANGELOG.md for current state.

# Plugin Auth Audit

Audit of authentication mechanisms across all 30 plugins. Last updated: 2026-09-14.

## Auth Types

| Plugin | Auth Type(s) | Service | Notes |
|--------|-------------|---------|-------|
| aap-bridge | Basic Auth + Token Auth | AAP Controller API | `username`/`password` or `oauthToken`; sets `CONTROLLER_OAUTH_TOKEN` shell env |
| cluster-ops | Token Auth | OpenShift API | `consoleOfflineToken` for `oc login`; env-var token |
| code-review | No Auth | Local | Pure local code analysis, no external calls |
| command-inject | No Auth | Local | Injects shell commands into tool context |
| container-linter | No Auth | Local | Local Containerfile/Dockerfile analysis |
| context-pruning | No Auth | Local | Context window management, no external calls |
| eda-events | No Auth | Webhook endpoint | Fires events to a configured HTTP endpoint; no auth headers sent |
| handoff | No Auth | Local | Session handoff between agents |
| lightwell | Token Auth | Lightwell API | `serviceAccountToken` bearer token |
| log-sanitizer | No Auth | Local | Regex-based log redaction, no external calls |
| notify | No Auth | Local | Desktop notifications via `osascript`/`notify-send` |
| ocp-context-injection | OC CLI + Console SSO | OpenShift + Console API | `oc` for cluster state; `consoleOfflineToken` for Console API catalog |
| ocp-oauth | Token Auth | OpenShift API | `oc login --token`; direct API token input per-call |
| ocp-obs-logging | Token Auth + OC CLI | Loki + Tempo + OpenShift | Bearer token for Loki/Tempo APIs; `oc` for FlowCollector/dashboard CRs |
| ocp-obs-metrics | Token Auth | Prometheus/Thanos + AlertManager | Bearer token via `PromQLClient` |
| pilot | Token Auth (env var) | GitHub/GitLab/Gitea APIs | `GITHUB_TOKEN`/`GH_TOKEN`, `GITLAB_TOKEN`, `GITEA_TOKEN` env vars |
| quay | Basic Auth + Token Auth | Quay Registry API | `username`/`password` or `apiToken` |
| rh-api-catalog | Console SSO | Red Hat Console API | `consoleOfflineToken` → `ConsoleAuthClient` → access token |
| rh-dev-content | No Auth | Red Hat Developer RSS | Public RSS feed + web scraping, no auth needed |
| rh-ecosystem-catalog | No Auth | Red Hat Ecosystem Catalog | Public API, no auth needed |
| rhacm | Token Auth + OC CLI | OpenShift + Thanos | `oc` for managed cluster CRs; optional `thanosUrl`+`token` for PromQL |
| rhacs | Basic Auth + Token Auth | RHACS Central API | `username`/`password` or `apiToken` |
| rhdh | Token Auth | Red Hat Developer Hub API | `apiToken` bearer token |
| rhdp-provisioner | Session Cookie | Red Hat Developer Portal API | `sessionCookie` passed as `Cookie` header |
| rhoai-eval-trustyai | Basic Auth + Token Auth + OC CLI | RHOAI Eval/TrustyAI APIs + OpenShift | `username`/`password` or `token` for eval/trusty APIs; `oc` for workbenches |
| rhoai-experiment-tracker | Token Auth | MLflow API | Bearer token (hardcoded empty default); `APIClient` |
| rhoai-mcp-bridge | Token Auth | MCP Server API | `oauthToken` bearer token |
| rhoai-mlflow-tools | Token Auth | MLflow API | Bearer token (hardcoded empty default); `APIClient` |
| rhoai-model-serving | Console SSO + OC CLI | OpenShift + Console Sandbox API | `consoleOfflineToken` → `ConsoleAuthClient`; `oc` for model CRs |
| rhoai-pipelines | Token Auth | Data Science Pipelines API | `token` bearer token |
| safety-net | No Auth | Local | Pre-flight safety checks, no external calls |
| satellite | Token Auth | Satellite API | `token` bearer token |
| snippets | No Auth | Local | Code snippet management, no external calls |
| tekton | OC CLI | OpenShift | `oc` for Tekton CRs (PipelineRun, TaskRun, etc.) |
| telemetry | No Auth | Local | Event tracking, no external auth |
| web-search | No Auth | HTTP | Uses `http.Client` for web requests; no auth |

## Summary

| Auth Category | Count | Plugins |
|--------------|-------|---------|
| Basic Auth + Token Auth | 5 | aap-bridge, quay, rhacs, rhoai-eval-trustyai (also OC CLI) |
| Token Auth only | 10 | cluster-ops, lightwell, ocp-oauth, ocp-obs-metrics, pilot, rhdh, rhoai-experiment-tracker, rhoai-mcp-bridge, rhoai-mlflow-tools, rhoai-pipelines |
| Token Auth + OC CLI | 2 | ocp-obs-logging, rhacm |
| Console SSO | 1 | rh-api-catalog |
| Console SSO + OC CLI | 2 | ocp-context-injection, rhoai-model-serving |
| OC CLI only | 1 | tekton |
| Session Cookie | 1 | rhdp-provisioner |
| No Auth | 12 | code-review, command-inject, container-linter, context-pruning, eda-events, handoff, log-sanitizer, notify, rh-dev-content, rh-ecosystem-catalog, safety-net, snippets |
| No Auth (HTTP, no creds) | 2 | telemetry, web-search |
| **Token Auth (env var)** | 1 | satellite |

### Observations

1. **5 plugins support basic auth**: aap-bridge, quay, rhacs, rhoai-eval-trustyai, and satellite (token-only, listed as basic auth in issue but actually token auth). All 5 that support basic auth also support token auth as a fallback.
2. **No plugins are missing needed auth**: Each plugin that connects to an external service has an appropriate auth mechanism.
3. **Console SSO pattern** (`ConsoleAuthClient`): Used by ocp-context-injection, rh-api-catalog, and rhoai-model-serving for Red Hat Console/Sandbox API access via offline token exchange.
4. **OC CLI auth** inherits kubeconfig credentials — no explicit auth configuration needed in the plugin itself.
5. **rhdp-provisioner** uses a non-standard session cookie auth pattern, unique among all plugins.
6. **pilot** reads tokens from environment variables rather than plugin options, unlike all other token-auth plugins.
