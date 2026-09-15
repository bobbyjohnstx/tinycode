# Plugin Credentials & Configuration

Every tinycode plugin receives configuration via the `options` field in `config.json`. Options flow through the plugin SDK's `RunWithOptions` factory pattern: config file → plugin manager → JSON-RPC initialize → `parseOptions()` in the plugin.

```jsonc
// ~/.config/tinycode/config.json
{
  "plugins": [
    {
      "name": "rhacs",
      "options": {
        "centralUrl": "https://central-stackrox.apps.cluster.example.com",
        "apiToken": "eyJhbGci..."
      }
    }
  ]
}
```

## Quick Reference

### What do I need for each plugin?

| Plugin | Minimum Required | Optional |
|---|---|---|
| **ocp-context-injection** | `oc` logged in | `consoleOfflineToken`, `clientId` |
| **ocp-oauth** | — (per tool call) | `server`, `insecureSkipTlsVerify` |
| **ocp-odf** | `oc` logged in (ODF operator) | — |
| **ocp-obs-logging** | `lokiUrl`, `token` | `tempoUrl` |
| **ocp-obs-metrics** | `prometheusUrl`, `token` | `alertManagerUrl`, `namespace` |
| **rhacs** | `centralUrl`, `apiToken` | — |
| **quay** | `registryUrl` | `apiToken` |
| **rhacm** | `oc` logged in to hub | `thanosUrl`, `token` |
| **rhdh** | `baseUrl` | `apiToken` |
| **rhdp-provisioner** | `sessionCookie` | `rhdpApiUrl` |
| **rh-api-catalog** | `consoleOfflineToken` | `clientId`, `catalogPath` |
| **rh-dev-content** | — | — |
| **rh-ecosystem-catalog** | — | — |
| **ocp-virt** | `oc` logged in (OCP-V operator) | — |
| **tekton** | `oc` logged in | — |
| **aap-bridge** | `controllerUrl`, `oauthToken` | `apiPrefix` |
| **eda-events** | `edaEndpoint` | `events`, `sensitivePatterns` |
| **lightwell** | `serviceAccountToken` | — |
| **satellite** | `satelliteUrl` + (`username`/`password` or `token`) | — |
| **rhoai-eval-trustyai** | `evalApiUrl`, `trustyaiUrl`, `token` | `namespace` |
| **rhoai-experiment-tracker** | `mlflowUrl` | `experimentName` |
| **rhoai-mcp-bridge** | `mcpServerUrl` | `oauthToken` |
| **rhoai-mlflow-tools** | `mlflowUrl` | — |
| **rhoai-model-serving** | `oc` logged in | `namespace`, `routeHost`, `consoleOfflineToken` |
| **rhoai-pipelines** | `pipelinesUrl`, `token` | `namespace` |
| **cluster-ops** | `oc`/`kubectl` available | `clusterId`, `apiUrl`, `consoleOfflineToken`, `insecureSkipTLSVerify` |
| **code-review** | — | — |
| **command-inject** | — | — |
| **container-linter** | — | — |
| **context-pruning** | — | — |
| **handoff** | — | — |
| **log-sanitizer** | — | — |
| **notify** | — | — |
| **pilot** | — | — |
| **safety-net** | — | — |
| **snippets** | — | — |
| **telemetry** | — | — |
| **web-search** | — | — |

---

## Red Hat Plugins

### ocp-context-injection

Gathers cluster context (version, nodes, operators, alerts) on session start.

**External dependency:** OpenShift cluster  
**CLI tools:** `oc` (must be logged in)

| Option | Required | Description |
|---|---|---|
| `consoleOfflineToken` | No | Enables Cost Management data from console.redhat.com |
| `clientId` | No | SSO client ID for token exchange. Use `ocm-cli` when using an OCM CLI token (default: `cloud-services`) |

**Notes:** Runs on `session.start` hook. Alert query execs into the `alertmanager-main-0` pod. No options needed for basic cluster context. See [How to Get Each Credential](#how-to-get-each-credential) for obtaining the offline token.

### ocp-oauth

OAuth login and token management for OpenShift clusters.

**External dependency:** OpenShift API server  
**CLI tools:** `oc` (for `oc login`)

| Option | Required | Description |
|---|---|---|
| `server` | No | Default API URL (e.g. `https://api.mycluster.example.com:6443`). Can be overridden per tool call |
| `insecureSkipTlsVerify` | No | Skip TLS certificate verification |

**Notes:** Credentials (`server`, `token`) are passed per tool call, not via plugin options. Sets `OC_EDITOR=cat` via `shell.env` hook.

### ocp-obs-logging

Loki log queries, Tempo trace queries, and OCP-native observability tools.

**External dependency:** Loki + Tempo on OpenShift, plus `oc` CLI  
**CLI tools:** `oc` (for flow collectors and dashboards)

| Option | Required | Description |
|---|---|---|
| `lokiUrl` | Yes | Loki API endpoint URL (include tenant path for OCP — see notes) |
| `token` | Yes | Bearer token for Loki/Tempo authentication |
| `tempoUrl` | No | Tempo API endpoint URL. Trace tools return "not configured" without it |

**Notes:** Tools degrade gracefully when optional endpoints are unconfigured. On OpenShift, Loki uses tenant-prefixed paths. Set `lokiUrl` to include the tenant prefix, e.g. `https://<loki-route>/api/logs/v1/infrastructure` for infrastructure logs or `https://<loki-route>/api/logs/v1/application` for application logs. Use `oc whoami -t` for the bearer token.

### ocp-obs-metrics

Prometheus/Thanos metrics queries and AlertManager alert management.

**External dependency:** Prometheus/Thanos + AlertManager on OpenShift

| Option | Required | Description |
|---|---|---|
| `prometheusUrl` | Yes | Prometheus or Thanos Querier URL |
| `token` | Yes | Bearer token for Prometheus/AlertManager |
| `alertManagerUrl` | No | Separate AlertManager URL if not co-located with Prometheus |
| `namespace` | No | Default namespace filter for queries |

**Notes:** Fetches alert summary on `session.start` hook.

### rhacs

Red Hat Advanced Cluster Security (StackRox) — image scanning, compliance, violations.

**External dependency:** RHACS Central API

| Option | Required | Description |
|---|---|---|
| `centralUrl` | Yes | RHACS Central URL (e.g. `https://central-stackrox.apps.cluster.example.com`) |
| `apiToken` | Yes | RHACS API token (Admin role recommended) |

Where to get the token: RHACS Central > Platform Configuration > Integrations > API Token

**Notes:** All 7 tools require both options. Returns stub responses when unconfigured.

### quay

Quay container registry — image search, tags, vulnerabilities (Clair), manifests.

**External dependency:** Quay registry API

| Option | Required | Description |
|---|---|---|
| `registryUrl` | Yes | Quay registry URL (e.g. `https://quay.apps.cluster.example.com`) |
| `apiToken` | No | Quay API token. Without it, only public repos are accessible |

Where to get the token: Quay > Account Settings > Generate Encrypted Password / Token

### rhacm

Red Hat Advanced Cluster Management — managed clusters, policies, applications.

**External dependency:** RHACM hub cluster  
**CLI tools:** `oc` (must be logged in to the hub)

| Option | Required | Description |
|---|---|---|
| `thanosUrl` | No | Thanos Querier URL for cross-cluster metrics |
| `token` | No | Bearer token for Thanos authentication |

**Notes:** Reads `ManagedCluster`, `Policy`, and `Application` resources via `oc get`.

### rhdh

Red Hat Developer Hub (Backstage) — catalog entities, APIs, components.

**External dependency:** Developer Hub instance

| Option | Required | Description |
|---|---|---|
| `baseUrl` | Yes | Developer Hub URL (e.g. `https://backstage-developer-hub.apps.cluster.example.com`) |
| `apiToken` | No | Bearer token for authenticated API access. Without it, only public catalog entities accessible |

### rhdp-provisioner

Red Hat Demo Platform — provision demo environments.

**External dependency:** demo.redhat.com API

| Option | Required | Description |
|---|---|---|
| `sessionCookie` | Yes | Browser session cookie from demo.redhat.com (see [Browser Session Cookie](#browser-session-cookie-rhdp)) |
| `rhdpApiUrl` | No | Override API URL (default: `https://catalog.demo.redhat.com/api/v1`) |

**Notes:** RHDP uses an OAuth proxy on an internal OpenShift cluster (`ocp-us-east-1.infra.open.redhat.com`), not Red Hat SSO directly. There is no programmatic token flow — authentication requires a browser-based OAuth code flow. Pass the resulting session cookie via `sessionCookie`. Cookies expire after a few hours.

### rh-api-catalog

Built-in catalog of 25 Red Hat console.redhat.com APIs.

**External dependency:** console.redhat.com APIs

| Option | Required | Description |
|---|---|---|
| `consoleOfflineToken` | Yes | Red Hat SSO offline token for API authentication |
| `clientId` | No | SSO client ID for token exchange. Use `ocm-cli` when using an OCM CLI token (default: `cloud-services`) |
| `catalogPath` | No | Local filesystem path to cached API spec files. Falls back to live API if not found |

See [How to Get Each Credential](#how-to-get-each-credential) for obtaining the offline token via OCM CLI.

### rh-dev-content

Red Hat developer content — articles, tutorials, guides from developers.redhat.com.

**External dependency:** developers.redhat.com (public)

No configuration needed. Scrapes public HTML content covering ~30 topic areas.

### rh-ecosystem-catalog

Red Hat ecosystem catalog — container images, operators, Helm charts.

**External dependency:** Pyxis API at catalog.redhat.com (public)

No configuration needed. Public API, no authentication.

### tekton

OpenShift Pipelines / Tekton — pipelines, runs, tasks.

**External dependency:** OpenShift with Pipelines operator  
**CLI tools:** `oc` (must be logged in)

No plugin options. Uses `oc` CLI session directly.

### ocp-odf

OpenShift Data Foundation — Ceph storage health, pools, PVCs, buckets, storage classes.

**External dependency:** OpenShift with ODF operator  
**CLI tools:** `oc` (must be logged in)

No plugin options. Uses `oc` CLI session directly. Provides 6 tools:
- `odf_status` — StorageCluster phase and version
- `odf_ceph_status` — Ceph health, capacity, OSD counts, mon quorum (uses rook-ceph-tools pod for live data)
- `odf_pools` — CephBlockPools and CephFilesystems with replication/EC config
- `odf_pvcs` — PVCs backed by ODF storage classes, filtered from cluster-wide PVC list
- `odf_buckets` — ObjectBucketClaims (NooBaa/RGW)
- `odf_storage_classes` — ODF-managed StorageClasses (rbd, cephfs, noobaa)

**Notes:** Session start hook gathers StorageCluster phase and Ceph health summary. `odf_ceph_status` attempts to exec into `rook-ceph-tools` deployment for live `ceph status`; falls back to CephCluster CR status if the tools pod is unavailable.

### ocp-virt

OpenShift Virtualization — VM lifecycle, live migration, DataVolumes, templates, networking.

**External dependency:** OpenShift with OpenShift Virtualization (CNV) operator  
**CLI tools:** `oc` (must be logged in)

No plugin options. Uses `oc` CLI session directly. Provides 11 tools:
- `virt_vms` — list VMs with status, CPU, memory
- `virt_describe` — detailed VM info (disks, volumes, networks, guest OS, conditions)
- `virt_start` / `virt_stop` / `virt_restart` — VM lifecycle
- `virt_migrate` — live migration via VirtualMachineInstanceMigration
- `virt_console` — serial console output from virt-launcher pod
- `virt_datavolumes` — DataVolume import/clone status and progress
- `virt_templates` — VM templates and instance types
- `virt_network` — NetworkAttachmentDefinitions (Multus secondary networks)
- `virt_migrations` — migration status tracking

**Notes:** Session start hook gathers a VM summary (total, running, stopped, error counts).

### aap-bridge

Ansible Automation Platform — job templates, inventories, job execution.

**External dependency:** AAP Controller  
**CLI tools:** `ansible-lint` (optional, for `lint_playbook` tool)

| Option | Required | Description |
|---|---|---|
| `controllerUrl` | Yes | AAP Controller or Gateway URL (e.g. `https://controller.example.com`) |
| `oauthToken` | Yes | OAuth token for AAP authentication |
| `apiPrefix` | No | API path prefix (default: `/api/v2`). Set to `/api/controller/v2` when connecting through an AAP 2.x gateway |

**Notes:** AAP 2.x gateway proxies multiple services at different paths (`/api/controller/`, `/api/eda/`, `/api/galaxy/`). When connecting to the gateway instead of the controller directly, set `apiPrefix` to `/api/controller/v2`. The `aap_hub_search` tool targets the controller's collections endpoint, not Automation Hub. Sets `CONTROLLER_HOST` and `CONTROLLER_OAUTH_TOKEN` via `shell.env` hook.

### eda-events

Event-Driven Ansible — event publishing to EDA webhooks.

**External dependency:** EDA Controller

| Option | Required | Description |
|---|---|---|
| `edaEndpoint` | Yes | EDA webhook endpoint URL |
| `events` | No | List of event type strings to subscribe to |
| `sensitivePatterns` | No | List of regex patterns for data redaction (defaults include `^sha256~`, `^sk-`, etc.) |

### lightwell

Lightwell CVE and package search.

**External dependency:** Lightwell service at packages.redhat.com

| Option | Required | Description |
|---|---|---|
| `serviceAccountToken` | Yes | Service account token for Lightwell API |

**Notes:** Base URL hardcoded to `https://packages.redhat.com/lightwell`.

### satellite

Red Hat Satellite — hosts, content views, errata, Lightspeed queries.

**External dependency:** Red Hat Satellite / Foreman API

| Option | Required | Description |
|---|---|---|
| `satelliteUrl` | Yes | Satellite server URL (e.g. `https://satellite.example.com`) |
| `username` | Auth* | Satellite username for basic auth |
| `password` | Auth* | Satellite password for basic auth |
| `token` | Auth* | Bearer token (for Satellite with OAuth configured) |

*Auth: Provide either `username`/`password` (recommended) or `token`.

> **Errata and content views** use Katello API paths (`/katello/api/v2/...`), while hosts use the Foreman API (`/api/v2/hosts`). The `satellite_query` (Lightspeed) tool requires the Lightspeed plugin to be installed on the Satellite server.

### rhoai-eval-trustyai

OpenShift AI model evaluation + TrustyAI bias/fairness metrics.

**External dependency:** OpenShift AI eval service + TrustyAI + `oc` CLI  
**CLI tools:** `oc` (for listing `notebooks.kubeflow.org`)

| Option | Required | Description |
|---|---|---|
| `evalApiUrl` | Yes | Eval service API URL |
| `trustyaiUrl` | Yes | TrustyAI service URL |
| `token` | Yes | Bearer token for both services |
| `namespace` | No | Default namespace for notebook queries |

### rhoai-experiment-tracker

MLflow experiment tracking on OpenShift AI — auto-logs tool call metrics.

**External dependency:** MLflow tracking server

| Option | Required | Description |
|---|---|---|
| `mlflowUrl` | Yes | MLflow tracking server URL |
| `experimentName` | No | Default experiment name (uses "Default" if not set) |

**Notes:** Tracks experiments via `session.start`/`session.end` hooks using `redhat.MLflowClient`.

### rhoai-mcp-bridge

MCP (Model Context Protocol) tool discovery and proxying.

**External dependency:** MCP server on OpenShift AI

| Option | Required | Description |
|---|---|---|
| `mcpServerUrl` | Yes | MCP server URL |
| `oauthToken` | No | OAuth token for authenticated MCP servers |

**Notes:** Discovers MCP tools on init and creates tinycode `ToolDef` wrappers for each.

### rhoai-mlflow-tools

Read-only MLflow experiment/run queries.

**External dependency:** MLflow tracking server

| Option | Required | Description |
|---|---|---|
| `mlflowUrl` | Yes | MLflow tracking server URL |

**Notes:** Uses `redhat.MLflowClient`. Read-only: list experiments, list runs, get run details, compare runs.

### rhoai-model-serving

OpenShift AI model serving — InferenceService, ServingRuntime, inference testing.

**External dependency:** OpenShift AI with model serving  
**CLI tools:** `oc` (must be logged in)

| Option | Required | Description |
|---|---|---|
| `namespace` | No | Default namespace for model serving resources |
| `routeHost` | No | Custom route host for inference endpoint |
| `consoleOfflineToken` | No | Enables console.redhat.com integration |

**Notes:** No required options — uses `oc` CLI session.

### rhoai-pipelines

Kubeflow Pipelines on OpenShift AI — pipeline management and run execution.

**External dependency:** Kubeflow Pipelines v2 API

| Option | Required | Description |
|---|---|---|
| `pipelinesUrl` | Yes | Kubeflow Pipelines API URL |
| `token` | Yes | Bearer token for pipelines API |
| `namespace` | No | Default namespace filter |

---

## General Plugins

### cluster-ops

Basic Kubernetes/OpenShift cluster operations — login, status, info.

**External dependency:** Kubernetes or OpenShift cluster  
**CLI tools:** `oc`, `kubectl`

| Option | Required | Description |
|---|---|---|
| `clusterId` | No | Cluster identifier for display |
| `apiUrl` | No | API server URL for `oc login` |
| `consoleOfflineToken` | No | Token for console.redhat.com auth-based login |
| `insecureSkipTLSVerify` | No | Skip TLS certificate verification |

### code-review

Git diff-based code review.

**CLI tools:** `git`

No configuration needed. Runs `git diff` locally.

### command-inject

Register external scripts as slash commands.

| Env Var | Description |
|---|---|
| `COMMAND_INJECT_DIR` | Directory containing executable scripts. Each becomes a slash command |

No plugin options. Configuration via environment variable only.

### container-linter

Containerfile/Dockerfile static analysis.

No configuration needed. Pure parser via `redhat.ParseContainerfile()`. No external calls.

### context-pruning

Duplicate tool output detection and deduplication.

| Env Var | Description |
|---|---|
| `CONTEXT_PRUNE_THRESHOLD` | Similarity threshold for duplicate detection |

No plugin options. Hook-based (`ToolExecAfter`).

### handoff

Cross-session context transfer via JSON documents.

| Env Var | Description |
|---|---|
| `HANDOFF_DIR` | Directory for handoff files (default: `~/.tinycode/handoff/`) |

No plugin options.

### log-sanitizer

Sensitive data redaction in tool output.

No configuration needed. Hook-based (`ToolExecAfter`). Built-in regex patterns for tokens, passwords, and keys.

### notify

Desktop notifications for session events.

**CLI tools:** `osascript` (macOS) or `notify-send` (Linux)

No configuration needed. Auto-detects platform.

### pilot

Git forge integration — issues, PRs, comments.

**CLI tools:** `git` (for remote URL detection)

| Env Var | Required | Description |
|---|---|---|
| `GITHUB_TOKEN` or `GH_TOKEN` | For GitHub | GitHub API token |
| `GITLAB_TOKEN` | For GitLab | GitLab API token |
| `GITLAB_URL` | No | GitLab instance URL (default: `https://gitlab.com`) |
| `GITEA_TOKEN` | For Gitea | Gitea API token |
| `GITEA_URL` | For Gitea | Gitea instance URL |
| `PILOT_PROVIDER` | No | Override auto-detected provider (`github`, `gitlab`, `gitea`) |

**Notes:** Provider auto-detected from `git remote get-url origin`. Token env var required for the detected provider.

### safety-net

Dangerous operation detection in tool calls.

No configuration needed. Hook-based (`PermissionAsk`). Built-in regex patterns.

### snippets

Code snippet templates.

| Env Var | Description |
|---|---|
| `SNIPPETS_DIR` | Custom template directory (default: `~/.config/tinycode/snippets/`) |

No plugin options. Built-in templates always available.

### telemetry

Tool call metrics tracking.

| Env Var | Description |
|---|---|
| `TELEMETRY_DB` | SQLite database path (default: `~/.tinycode/telemetry.db`) |

No plugin options. Pure Go SQLite (no CGO).

### web-search

DuckDuckGo web search.

No configuration needed. HTTP scrape of `https://html.duckduckgo.com/html/`. No API key required. Needs internet access.

---

## Functional Testing by Server Type

To functionally test all plugins, you need the following live servers. Plugins are grouped by which server they need. The **Tested** column shows whether `script/test-plugins.sh` has live-server tests for each plugin.

### Testing coverage summary

| Server / Product | Plugins | Tested | Untested |
|---|---|---|---|
| OPP cluster (`oc login`) | 12 | 12 | 0 |
| console.redhat.com (OCM token) | 2 | 2 | 0 |
| demo.redhat.com (session cookie) | 1 | 0 | 1 (`rhdp-provisioner` — test exists, needs cookie) |
| OpenShift AI (RHOAI) | 6 | 0 | 6 |
| Red Hat Developer Hub | 1 | 0 | 1 |
| Ansible Automation Platform | 2 | 1 | 1 (`eda-events`) |
| Standalone (Lightwell, Satellite) | 2 | 1 | 1 (`lightwell`) |
| No server needed | 17 | 3 | 14 (local-only, no live dependency) |

### OpenShift Platform Plus cluster (`oc login`)

One OPP demo cluster covers the most plugins. After `oc login`:

| Plugin | Additional requirement | Tested |
|---|---|---|
| `ocp-context-injection` | None | Yes |
| `ocp-oauth` | None | Yes |
| `tekton` | OpenShift Pipelines operator | Yes |
| `cluster-ops` | None | Yes |
| `rhacm` | RHACM operator + managed clusters | Yes |
| `rhoai-model-serving` | OpenShift AI operator | No — needs RHOAI |
| `ocp-obs-metrics` | Derive `prometheusUrl` + `token` from cluster | Yes |
| `ocp-virt` | OpenShift Virtualization operator | Yes |
| `ocp-odf` | ODF operator | Yes |
| `ocp-obs-logging` | Loki + Tempo deployed | Yes |
| `rhacs` | StackRox Central deployed, generate API token | Yes |
| `quay` | Quay deployed, generate API token | Yes |

### console.redhat.com offline token (via OCM CLI)

One offline token from `ocm login` covers:

| Plugin | Notes | Tested |
|---|---|---|
| `rh-api-catalog` | Pass `clientId: "ocm-cli"` | Yes |
| `ocp-context-injection` | Optional — enables Cost Management. Pass `clientId: "ocm-cli"` | Yes (basic context; cost mgmt not tested) |

### demo.redhat.com session cookie

| Plugin | Notes | Tested |
|---|---|---|
| `rhdp-provisioner` | Browser session cookie, expires in hours | No — test exists, needs `RHDP_SESSION_COOKIE` env var |

### OpenShift AI (RHOAI) cluster

Needs an RHOAI-enabled cluster with the full operator stack. An OPP demo with "OpenShift AI" add-on works, or a standalone RHOAI cluster.

| Plugin | What it needs on the cluster | Tested |
|---|---|---|
| `rhoai-eval-trustyai` | Eval service + TrustyAI | No |
| `rhoai-experiment-tracker` | MLflow tracking server | No |
| `rhoai-mcp-bridge` | MCP server endpoint | No |
| `rhoai-mlflow-tools` | MLflow tracking server | No |
| `rhoai-model-serving` | KServe/ModelMesh + deployed model | No |
| `rhoai-pipelines` | Kubeflow Pipelines v2 | No |

### Red Hat Developer Hub

| Plugin | Tested |
|---|---|
| `rhdh` | No |

### Ansible Automation Platform

| Plugin | What it needs | Tested |
|---|---|---|
| `aap-bridge` | AAP Controller URL + OAuth token | Yes |
| `eda-events` | EDA Controller webhook endpoint | No |

### Standalone products

| Plugin | Product | Tested |
|---|---|---|
| `lightwell` | Lightwell service (packages.redhat.com) | No |
| `satellite` | Red Hat Satellite / Foreman | Yes |

### No server needed

| Plugin | Notes | Tested |
|---|---|---|
| `rh-dev-content` | Public website scrape | Yes |
| `rh-ecosystem-catalog` | Public Pyxis API | Yes |
| `web-search` | Public DuckDuckGo | Yes |
| `code-review` | Local git | No |
| `command-inject` | Local scripts | No |
| `container-linter` | Local parser | No |
| `context-pruning` | Local hooks | No |
| `handoff` | Local filesystem | No |
| `log-sanitizer` | Local hooks | No |
| `notify` | OS notifications | No |
| `pilot` | Optional forge API | No |
| `safety-net` | Local hooks | No |
| `snippets` | Local templates | No |
| `telemetry` | Local SQLite | No |

---

## How to Get Each Credential

### Red Hat SSO offline token (via OCM CLI)

The `console.redhat.com/openshift/token` page is deprecated. Use the OCM CLI instead.

**Install:**

```bash
# macOS
brew install ocm-cli
# or download from https://github.com/openshift-online/ocm-cli/releases
```

**Login and extract token:**

```bash
ocm login --token <your-sso-token>  # or interactive: ocm login --use-auth-code
```

The refresh token is stored in the OCM config file:

- macOS: `~/Library/Application Support/ocm/ocm.json`
- Linux: `~/.config/ocm/ocm.json`

Extract it:

```bash
jq -r '.refresh_token' "$HOME/Library/Application Support/ocm/ocm.json"  # macOS
jq -r '.refresh_token' "$HOME/.config/ocm/ocm.json"                      # Linux
```

**Config example** — pass `clientId: "ocm-cli"` so the SSO token exchange uses the correct client:

```jsonc
{
  "plugins": [
    {
      "name": "rh-api-catalog",
      "options": {
        "consoleOfflineToken": "<refresh_token from ocm.json>",
        "clientId": "ocm-cli"
      }
    }
  ]
}
```

### RHACS API token

Generate from the RHACS Central UI:

1. Log into RHACS Central (e.g. `https://central-stackrox.apps.cluster.example.com`)
2. Go to **Platform Configuration > Integrations > Authentication Tokens**
3. Click **Generate Token**
4. Name: anything descriptive, Role: `Admin` (or scoped role for read-only access)
5. Copy the token string

### OpenShift cluster login (`oc` token)

Most plugins that need cluster access use the `oc` CLI session directly:

```bash
oc login https://api.cluster.example.com:6443 -u kubeadmin -p <password>
# or with a token:
oc login --token=sha256~xxx --server=https://api.cluster.example.com:6443
```

For plugins that need the bearer token directly (ocp-obs-metrics, ocp-obs-logging):

```bash
oc whoami -t
```

### Prometheus/Thanos URL from a cluster

Derive the Prometheus URL from the thanos-querier route:

```bash
echo "https://$(oc get route thanos-querier -n openshift-monitoring -o jsonpath='{.spec.host}')"
```

Use the `oc` token (`oc whoami -t`) as the bearer token.

### Quay API token

From the Quay UI:

1. Log into Quay (e.g. `https://quay.apps.cluster.example.com`)
2. Go to **Account Settings** (click your username, top right)
3. Under **Docker CLI Password**, click **Generate Encrypted Password**
4. Use the generated token, or create a robot account for API-only access

### Browser session cookie (RHDP)

RHDP (`demo.redhat.com`) uses OAuth proxy authentication — no programmatic token flow exists. You must extract a session cookie from your browser:

1. Log into https://demo.redhat.com in your browser
2. Open DevTools (F12) > **Network** tab
3. Navigate to any page on demo.redhat.com
4. Click any request to `catalog.demo.redhat.com` or `demo.redhat.com`
5. In the request headers, copy the full `Cookie` header value
6. Pass as `sessionCookie` in plugin options

Cookies expire after a few hours. This is inherently short-lived.

### AAP OAuth token

**From AAP Controller UI:**

1. Log into the AAP Controller UI
2. Go to **Users > (your user) > Tokens**
3. Click **Add**, set scope to `write`
4. Copy the token value

**From AAP 2.x Gateway (CLI):**

```bash
curl -sk -u admin:<password> \
  -X POST https://<gateway-host>/api/gateway/v1/tokens/ \
  -H 'Content-Type: application/json' \
  -d '{"scope":"write"}' | jq -r '.token'
```

When using a gateway token, set `apiPrefix` to `/api/controller/v2` in plugin options.

### Satellite authentication

**Basic auth (recommended):** Use `username` and `password` directly — works with any Satellite/Foreman instance.

```jsonc
"satellite": {
  "options": {
    "satelliteUrl": "https://satellite.example.com",
    "username": "admin",
    "password": "your-password"
  }
}
```

**Personal Access Token (alternative):** From Satellite UI, go to **Administer > Users > (your user) > Personal Access Tokens**, generate a token, then use it as `token` option. Note: Satellite PATs use basic auth internally (not Bearer), so the plugin handles this via the `username`/`password` mechanism with the PAT as the password.

> **Foreman API vs Katello API:** Hosts are at `/api/v2/hosts` (Foreman). Errata and content views are at `/katello/api/v2/errata` and `/katello/api/v2/content_views` (Katello). The plugin routes to the correct paths automatically.
