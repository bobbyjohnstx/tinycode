# Plugin Catalog

Complete reference for all 36 plugins shipped with tinycode. For plugin development, SDK usage, and wire protocol details, see [plugin-development.md](plugin-development.md).

## Quick Start

```bash
# Build and install a single plugin
go build -o ~/.config/tinycode/plugins/safety-net ./cmd/plugin-safety-net

# Build all plugins at once
for dir in cmd/plugin-*/; do
    name=$(basename "$dir")
    go build -o ~/.config/tinycode/plugins/${name#plugin-} ./$dir
done
```

Enable plugins in your tinycode config (`~/.config/tinycode/config.json`):

```json
{
  "plugins": ["safety-net", "web-search", "notify"]
}
```

All plugins that connect to OpenShift-hosted services require `ocp-oauth` — it provides the `oc login` auth hook that every OCP-connected plugin depends on. Authenticate once, and every plugin reuses the token.

---

## Configuration

Most plugins work out of the box. Plugins that connect to external APIs accept options in your tinycode config or environment variables.

### Red Hat Plugins

| Plugin | Required Options | Optional Options |
|--------|-----------------|------------------|
| cluster-ops | — | `consoleOfflineToken`, `clusterId` (enables Insights tools) |
| ocp-obs-metrics | `prometheusUrl` | `alertManagerUrl`, `token`, `namespace` |
| ocp-obs-logging | — | `lokiUrl`, `tempoUrl`, `token` |
| rhacs | `centralUrl` | `apiToken` |
| lightwell | — | `serviceAccountToken` |
| aap-bridge | `controllerUrl` | `oauthToken` |
| rhacm | — | `hubUrl`, `thanosUrl`, `token` |
| rhoai-model-serving | — | `namespace`, `routeHost`, `consoleOfflineToken` |
| rhoai-mcp-bridge | `mcpServerUrl` | `oauthToken` |
| rhoai-mlflow-tools | `mlflowUrl` | — |
| rhoai-pipelines | `pipelinesUrl` | `namespace`, `token` |
| rhoai-eval-trustyai | — | `evalApiUrl`, `trustyaiUrl`, `namespace`, `token` |
| satellite | `satelliteUrl` | `token` |
| rhdp-provisioner | `consoleOfflineToken` | `rhdpApiUrl` |

### General Plugins

| Plugin | Environment Variables | Notes |
|--------|----------------------|-------|
| pilot | `GITEA_TOKEN`, `GITHUB_TOKEN` or `GH_TOKEN`, `GITLAB_TOKEN` (one required) | Auto-detects platform from git remote. Override with `PILOT_PROVIDER`. See also `GITEA_URL`, `GITLAB_URL`. |
| command-inject | `COMMAND_INJECT_DIR` (required) | Path to directory of executable scripts |
| notify | `NTFY_TOPIC` (optional) | Enables push notifications via ntfy.sh |
| telemetry | `TELEMETRY_DB` (optional) | Default: `~/.tinycode/telemetry.db` |
| handoff | `HANDOFF_DIR` (optional) | Default: `~/.tinycode/handoff` |
| snippets | `SNIPPETS_DIR` (optional) | Default: `~/.tinycode/snippets` |
| context-pruning | `CONTEXT_PRUNE_THRESHOLD` (optional) | Messages before outputs are considered stale (default: 20) |

Plugins not listed above require no configuration.

---

## General Plugins (12)

### Security

| Plugin | Type | Description |
|--------|------|-------------|
| **log-sanitizer** | Hook | Redacts secrets and sensitive data from tool outputs before they reach the LLM context |
| **safety-net** | Hook | Blocks destructive shell commands via PermissionAsk |

**Log Sanitizer** intercepts every tool output and applies regex-based redaction rules: PEM private key blocks, API key prefixes (OpenAI, GitHub, AWS, Slack), bearer tokens, and high-entropy catch-all for 40+ character mixed-class strings. Matches are replaced with `[REDACTED:<type>]`. Zero configuration.

**Safety Net** intercepts `permission.ask` for bash-type permissions and blocks three categories: filesystem destructive (`rm -rf /`, `mkfs`, fork bombs), Kubernetes/OCP destructive (`kubectl delete namespace`, `helm uninstall` in kube-system), and git destructive (`git push --force main`). Scoped paths like `./build` are allowed.

### Developer Experience

| Plugin | Type | Description |
|--------|------|-------------|
| **code-review** | Tool | Git diff formatted for AI-assisted code review |
| **command-inject** | Hook | Auto-discovers executable scripts and registers each as a callable tool |
| **context-pruning** | Hook | Deduplicates repeated tool outputs to optimize token usage |
| **handoff** | Tool + Hook | Cross-session context handoff — saves goals, decisions, open tasks |
| **notify** | Tool | Desktop notifications (macOS/Linux) with optional push via ntfy.sh |
| **snippets** | Tool | Kubernetes/OpenShift YAML template library with variable substitution |
| **telemetry** | Tool + Hook | Tool call analytics with local SQLite persistence and reporting |

**Tools:**

| Plugin | Tool | Description |
|--------|------|-------------|
| code-review | `code_review` | Gather a git diff formatted for AI-assisted code review |
| handoff | `handoff_save` | Save session context for handoff to the next session |
| notify | `notify` | Send a desktop notification |
| snippets | `snippet_list` | List available snippet templates |
| snippets | `snippet_expand` | Expand a template with variable substitution |
| telemetry | `telemetry_report` | Aggregate summary: sessions, tool calls, top tools |
| telemetry | `telemetry_query` | Query tool call records by name and recency |

### Automation

| Plugin | Type | Description |
|--------|------|-------------|
| **pilot** | Tool | Multi-platform issue management (Gitea, GitHub, GitLab) |

**Tools:**

| Tool | Description |
|------|-------------|
| `pilot_issues_list` | List issues with optional state and label filters |
| `pilot_issue_create` | Create a new issue with title, body, and labels |
| `pilot_issue_update` | Update an existing issue (title, body, state, labels) |
| `pilot_issue_comment` | Add a comment to an issue |

### Reference

| Plugin | Type | Description |
|--------|------|-------------|
| **web-search** | Tool | Web search via DuckDuckGo with Red Hat KB integration |

**Tools:**

| Tool | Description |
|------|-------------|
| `web_search` | Search the web via DuckDuckGo |
| `rh_kb_search` | Search Red Hat knowledge base (access.redhat.com) |

---

## Red Hat Plugins — OpenShift (4)

All OCP plugins require `ocp-oauth` for authentication.

| Plugin | Type | Description |
|--------|------|-------------|
| **ocp-oauth** | Tool + Hook | Shared OpenShift authentication via `oc login` |
| **ocp-context-injection** | Hook | Injects cluster metadata into the system prompt on session start |
| **ocp-obs-logging** | Tool | Loki logs, Tempo traces, network flows, dashboards |
| **ocp-obs-metrics** | Tool | PromQL queries, alert management, alert silencing |

**Tools (ocp-oauth):**

| Tool | Description |
|------|-------------|
| `oc-login` | Authenticate to an OpenShift cluster with API token |

**Tools (ocp-obs-logging):**

| Tool | Description |
|------|-------------|
| `obs_logs` | Query Loki logs with LogQL or namespace/pod/severity filters |
| `obs_traces` | Search Tempo traces by service, operation, duration |
| `obs_trace_detail` | Full span tree for a trace ID |
| `obs_flow_collectors` | List FlowCollector resources from Network Observability |
| `obs_dashboards` | List available Grafana dashboards |

**Tools (ocp-obs-metrics):**

| Tool | Description |
|------|-------------|
| `obs_promql` | Run PromQL instant or range query |
| `obs_alerts` | List active alerts filtered by severity and namespace |
| `obs_alert_silence` | Silence an alert with confirmation |

---

## Red Hat Plugins — Cluster Operations (1)

| Plugin | Type | Description |
|--------|------|-------------|
| **cluster-ops** | Tool + Hook | Direct cluster visibility — resources, logs, events, health, GitOps, Insights |

**Tools:**

| Tool | Description |
|------|-------------|
| `oc-login` | Authenticate to an OpenShift cluster |
| `oc-status` | Check cluster connection status |
| `cluster-info` | Cluster health: nodes, operators, API server |

---

## Red Hat Plugins — Ansible (2)

| Plugin | Type | Description |
|--------|------|-------------|
| **aap-bridge** | Tool | Ansible Automation Platform — job templates, inventories, Automation Hub, playbook linting |
| **eda-events** | Tool | Event-Driven Ansible bridge — session events to EDA webhooks |

**Tools (aap-bridge):**

| Tool | Description |
|------|-------------|
| `aap_list_templates` | List job templates with last run status |
| `aap_launch_job` | Launch a job template (prompts for confirmation) |
| `aap_job_status` | Check running/completed job status |
| `aap_job_output` | Full stdout/stderr of a completed job |
| `aap_list_inventories` | List inventories with host counts |
| `aap_hub_search` | Search Automation Hub for certified collections |
| `aap_lint_playbook` | Lint an Ansible playbook for best practices |

---

## Red Hat Plugins — RHOAI (6)

| Plugin | Type | Description |
|--------|------|-------------|
| **rhoai-model-serving** | Tool | Discover deployed models, serving runtimes, Developer Sandbox |
| **rhoai-experiment-tracker** | Tool | Track session metrics to MLflow |
| **rhoai-mcp-bridge** | Tool | Bridge to RHOAI MCP server |
| **rhoai-mlflow-tools** | Tool | MLflow experiment and model registry management |
| **rhoai-pipelines** | Tool | Data Science Pipelines (Kubeflow) |
| **rhoai-eval-trustyai** | Tool | Model evaluation, TrustyAI fairness/drift monitoring |

**Tools (rhoai-model-serving):**

| Tool | Description |
|------|-------------|
| `rhoai_list_models` | List deployed models with serving runtime, status, URL |
| `rhoai_model_status` | Detailed status: replicas, GPU allocation, conditions |
| `rhoai_list_runtimes` | Available ServingRuntimes (vLLM, Caikit, TGIS) |
| `rhoai_sandbox_provision` | Provision a Developer Sandbox environment |
| `rhoai_sandbox_status` | Check Developer Sandbox provisioning status |

**Tools (rhoai-mcp-bridge):**

| Tool | Description |
|------|-------------|
| `rhoai_mcp_list` | List tools exposed by the RHOAI MCP endpoint |
| `rhoai_mcp_call` | Call a tool on the RHOAI MCP server by name |

**Tools (rhoai-mlflow-tools):**

| Tool | Description |
|------|-------------|
| `mlflow_experiments` | List MLflow experiments |
| `mlflow_runs` | List runs in an experiment with metrics summary |
| `mlflow_compare` | Compare 2-5 runs side-by-side |
| `mlflow_artifacts` | Browse artifacts attached to a run |
| `mlflow_model_registry` | List registered models |
| `mlflow_model_version` | Detailed info for a specific model version |
| `mlflow_promote` | Transition model version stage (with confirmation) |
| `mlflow_log_metric` | Log a metric value to an MLflow run |

**Tools (rhoai-pipelines):**

| Tool | Description |
|------|-------------|
| `rhoai_pipeline_list` | List Data Science Pipelines |
| `rhoai_pipeline_run` | Trigger a pipeline run (with confirmation) |
| `rhoai_pipeline_status` | Check status of a pipeline run |
| `rhoai_pipeline_create` | Create a pipeline from a workflow definition |

**Tools (rhoai-eval-trustyai):**

| Tool | Description |
|------|-------------|
| `rhoai_eval_run` | Run model evaluation (lm-eval, ragas, garak, guidellm) |
| `rhoai_eval_status` | Check evaluation status and results |
| `rhoai_eval_compare` | Compare results across multiple evaluations |
| `rhoai_trusty_metrics` | TrustyAI fairness and drift metrics for a model |
| `rhoai_trusty_alerts` | Active TrustyAI alerts for drift and bias |

---

## Red Hat Plugins — Platform (12)

| Plugin | Type | Description |
|--------|------|-------------|
| **satellite** | Tool | Satellite AI assistant — RHEL knowledge, host management, errata, content views |
| **quay** | Tool | Quay container registry — search, tags, manifests, Clair vulnerability scans |
| **rhdh** | Tool | Developer Hub catalog — search, entity details, OpenAPI specs, TechDocs, dependencies |
| **tekton** | Tool | Tekton pipelines — list, start runs, check status, view logs |
| **rhacm** | Tool | ACM multi-cluster management — clusters, policies, violations, observability |
| **rhacs** | Tool | ACS security scanning — image scans, policy checks, violations, compliance |
| **rh-api-catalog** | Tool | Red Hat API catalog — browse console.redhat.com APIs, fetch specs |
| **rh-dev-content** | Tool | Developer content browser — articles by topic, full reader, RSS feed |
| **rh-ecosystem-catalog** | Tool | Ecosystem Catalog — certified container images and operators via Pyxis |
| **rhdp-provisioner** | Tool | Developer platform — provision demo environments, check status |
| **container-linter** | Tool | Containerfile linting — Red Hat best practices, UBI checks, bootc validation |
| **lightwell** | Tool | Package security — Lightwell repos, SLSA provenance, OSV vulnerabilities |

**Tools (satellite):**

| Tool | Description |
|------|-------------|
| `satellite_query` | Ask Lightspeed about RHEL/Satellite topics |
| `satellite_hosts` | Search managed hosts by name, OS, environment |
| `satellite_errata` | Search errata by type (security/bugfix/enhancement) |
| `satellite_content_views` | List content views with publish dates |

**Tools (quay):**

| Tool | Description |
|------|-------------|
| `quay_search` | Search repositories by name or keyword |
| `quay_tags` | List tags with digest, size, security scan status |
| `quay_manifest` | Inspect manifest layers, architecture, config |
| `quay_vulnerabilities` | Clair CVE scan results sorted by severity |
| `quay_labels` | Get labels on a manifest |

**Tools (rhdh):**

| Tool | Description |
|------|-------------|
| `rhdh_catalog_search` | Search by name, kind, or lifecycle stage |
| `rhdh_catalog_entity` | Full entity details with metadata and relations |
| `rhdh_api_spec` | Fetch OpenAPI/AsyncAPI spec for an API entity |
| `rhdh_techdocs` | Rendered TechDocs content for a component |
| `rhdh_dependencies` | Dependency graph (consumesApi, providesApi, dependsOn) |

**Tools (tekton):**

| Tool | Description |
|------|-------------|
| `tekton_list_pipelines` | List pipelines in namespace with task details |
| `tekton_list_runs` | List PipelineRuns with status and duration |
| `tekton_run_status` | Detailed status of a specific PipelineRun |
| `tekton_run_logs` | Logs for a task in a PipelineRun |
| `tekton_list_tasks` | Available Tasks and ClusterTasks |
| `tekton_start_run` | Start a pipeline run (with confirmation) |

**Tools (rhacm):**

| Tool | Description |
|------|-------------|
| `acm_clusters` | List managed clusters with status, version, provider |
| `acm_cluster_detail` | Detailed cluster info with addon status |
| `acm_policies` | List governance policies with compliance status |
| `acm_violations` | Active policy violations across fleet |
| `acm_applications` | List ACM-managed ArgoCD applications |
| `acm_app_deploy` | Deploy ApplicationSet (with confirmation) |
| `acm_observability` | Run federated PromQL via ACM Thanos |

**Tools (rhacs):**

| Tool | Description |
|------|-------------|
| `rhacs_image_scan` | Scan container image for CVEs |
| `rhacs_image_check` | Check image against deploy-time policies |
| `rhacs_deployment_check` | Validate deployment YAML against security policies |
| `rhacs_violations` | List active policy violations |
| `rhacs_risk` | Risk score and factors for a deployment |
| `rhacs_compliance_scan` | Trigger a compliance scan |
| `rhacs_compliance_status` | Compliance results by standard (CIS, NIST, PCI) |

**Tools (rh-api-catalog):**

| Tool | Description |
|------|-------------|
| `rh_api_list` | Browse available console.redhat.com APIs |
| `rh_api_spec` | Fetch OpenAPI spec for an API |
| `rh_api_endpoints` | List endpoints with methods, paths, parameters |

**Tools (rh-dev-content):**

| Tool | Description |
|------|-------------|
| `rh_dev_search` | Browse articles by topic |
| `rh_dev_article` | Read full content of an article by URL |
| `rh_dev_recent` | Get recent articles from the RSS feed |

**Tools (rh-ecosystem-catalog):**

| Tool | Description |
|------|-------------|
| `ecosystem_search` | Search certified container images via Pyxis |
| `ecosystem_operator` | Search certified operators by package name |
| `ecosystem_browse` | Browse recent certified images or operator bundles |

**Tools (rhdp-provisioner):**

| Tool | Description |
|------|-------------|
| `rhdp_search` | Search RHDP demo catalog |
| `rhdp_provision` | Provision a demo environment (with confirmation) |
| `rhdp_status` | Check provisioning status |
| `rhdp_list_active` | List active demo environments with expiration |

**Tools (container-linter):**

| Tool | Description |
|------|-------------|
| `container_lint` | Lint Containerfile against Red Hat best practice rules |
| `bootc_validate` | Validate bootc-compatible image builds |
| `container_base_suggest` | Suggest UBI base image for a use case |

**Tools (lightwell):**

| Tool | Description |
|------|-------------|
| `lightwell_check_package` | Check a single package against Lightwell repos |
| `lightwell_check_deps` | Scan pom.xml or requirements.txt for patches |
| `lightwell_osv` | Query OSV vulnerability data for a package |
| `lightwell_provenance` | Verify SLSA Level 3 build provenance |
| `lightwell_config_check` | Audit build config for Lightwell repo configuration |
| `lightwell_scan_containerfile` | Scan Containerfile for dependency and base image issues |

---

## Suggested Bundles

Mix and match plugins by role. Start with the ones marked **core**, add others as needed.

### Essential (Every User)

Safety, search, and notifications — useful regardless of role.

```json
{
  "plugins": [
    "log-sanitizer",
    "safety-net",
    "web-search",
    "notify"
  ]
}
```

### Local LLM Optimization

For users running local models with limited context windows.

```json
{
  "plugins": [
    "context-pruning",
    "handoff",
    "telemetry"
  ]
}
```

### Power User DevEx

Scripting, templates, and code review for daily development.

```json
{
  "plugins": [
    "code-review",
    "command-inject",
    "snippets",
    "pilot"
  ]
}
```

### OpenShift Administrator

Day-to-day cluster management, troubleshooting, and security posture.

```json
{
  "plugins": [
    "ocp-oauth",
    "ocp-context-injection",
    "cluster-ops",
    "rhacs",
    "tekton"
  ]
}
```

### Platform / SRE

Full-stack visibility from cluster health to CI pipelines to automation.

```json
{
  "plugins": [
    "ocp-oauth",
    "ocp-context-injection",
    "cluster-ops",
    "ocp-obs-metrics",
    "ocp-obs-logging",
    "tekton",
    "aap-bridge",
    "eda-events",
    "rhacs"
  ]
}
```

### Application Developer

Build, scan, deploy, and iterate without leaving the editor.

```json
{
  "plugins": [
    "ocp-oauth",
    "ocp-context-injection",
    "cluster-ops",
    "tekton",
    "quay",
    "rhdh",
    "lightwell",
    "code-review"
  ]
}
```

### Security / Governance & Compliance

Audit-focused — image scanning, policy enforcement, supply chain verification, dependency patching.

```json
{
  "plugins": [
    "ocp-oauth",
    "log-sanitizer",
    "safety-net",
    "rhacs",
    "lightwell",
    "container-linter",
    "quay",
    "tekton"
  ]
}
```

### RHEL / Infrastructure (Sysadmin)

For Ansible-driven infrastructure work targeting Satellite-managed environments.

```json
{
  "plugins": [
    "satellite",
    "aap-bridge",
    "eda-events"
  ]
}
```

### AI/ML Engineer

Model serving, experiment tracking, pipelines, and evaluation on RHOAI.

```json
{
  "plugins": [
    "ocp-oauth",
    "rhoai-model-serving",
    "rhoai-experiment-tracker",
    "rhoai-mlflow-tools",
    "rhoai-pipelines",
    "rhoai-eval-trustyai",
    "rhoai-mcp-bridge"
  ]
}
```

### Fleet Manager

Multi-cluster management with observability and access control.

```json
{
  "plugins": [
    "ocp-oauth",
    "ocp-context-injection",
    "cluster-ops",
    "rhacm",
    "ocp-obs-metrics"
  ]
}
```

### Developer Reference

Red Hat developer content, ecosystem catalog, and API discovery.

```json
{
  "plugins": [
    "rh-dev-content",
    "rh-ecosystem-catalog",
    "rh-api-catalog",
    "rhdp-provisioner",
    "web-search"
  ]
}
```

### Incident Response / On-Call

Real-time triage — live metrics, log queries, alert management, and security violations.

```json
{
  "plugins": [
    "ocp-oauth",
    "ocp-context-injection",
    "cluster-ops",
    "ocp-obs-metrics",
    "ocp-obs-logging",
    "rhacs",
    "notify"
  ]
}
```

### Developer Onboarding

New hire ramp-up — explore the catalog, read learning paths, spin up demo environments, discover APIs.

```json
{
  "plugins": [
    "rh-dev-content",
    "rhdh",
    "rh-api-catalog",
    "rhdp-provisioner",
    "rh-ecosystem-catalog",
    "web-search"
  ]
}
```

---

## Shared Library

All Red Hat plugins share `internal/redhat/`, which provides:

| Component | Description |
|-----------|-------------|
| **OcClient** | Typed wrapper around `oc` CLI (get, describe, logs, apply, raw) |
| **APIClient** | HTTP client with token injection, retry on 401/5xx, configurable timeouts |
| **ConsoleAuthClient** | SSO token exchange for console.redhat.com APIs with in-memory caching |
| **PromQLClient** | Prometheus/Thanos query and alert management (instant, range, alerts, silencing) |
| **MlflowClient** | MLflow tracking server operations (experiments, runs, artifacts, registry) |
| **ContainerfileParser** | Multi-stage Containerfile parsing and dependency extraction |
| **HTML Utilities** | HTML tag stripping for web scraping plugins |

See `internal/redhat/` for implementation details and `internal/redhat/*_test.go` for usage examples.

---

## Development

Plugins are standalone Go binaries using the `pkg/plugin/` SDK. See [plugin-development.md](plugin-development.md) for the full SDK reference, wire protocol, testing patterns, and examples. See [plugin-sdk-design.md](plugin-sdk-design.md) for design rationale and the migration guide from the TypeScript plugin system.
