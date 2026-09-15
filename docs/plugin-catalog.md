# Plugin Catalog

Complete reference for the 30 plugins and 4 core builtins shipped with tinycode. Plugins are organized by category. For plugin development, SDK usage, and wire protocol details, see [plugin-development.md](plugin-development.md).

## Quick Start

```bash
# Interactive setup — picks model, username, and plugins by role
tinycode init

# Or install plugins manually
tinycode plugin install safety-net
tinycode plugin install ocp-context-injection

# List plugins with category filter
tinycode plugin list
tinycode plugin list --category sre
```

Enable plugins in your tinycode config (`~/.config/tinycode/tinycode.json`):

```json
{
  "plugins": ["safety-net", "ocp-context-injection", "audit-logs"]
}
```

Example configs for common roles are available in `configs/`.

---

## Core Builtins (Always Available)

These features are built into tinycode and require no installation or configuration. They were promoted from plugins to core builtins because every user benefits from them.

| Builtin | Type | Description |
|---------|------|-------------|
| **context-pruning** | Hook (ToolExecAfter) | Detects duplicate tool outputs within a sliding window and annotates them to save context |
| **notify** | Tool | Send desktop notifications (macOS via osascript, Linux via notify-send) |
| **code-review** | Tool | Git diff formatted as a markdown code review block |
| **handoff** | Tool + Hooks | Cross-session context handoff — saves goals, decisions, open tasks, modified files |

**Tools:**

| Builtin | Tool | Description |
|---------|------|-------------|
| notify | `notify` | Send a desktop notification with title and message |
| code-review | `code_review` | Show git diff for review (supports ref, path, staged, context lines) |
| handoff | `handoff_save` | Save session context (goal, decisions, open tasks, files) for the next session |

**Hooks:**

| Builtin | Hook | Description |
|---------|------|-------------|
| context-pruning | `ToolExecAfter` | SHA-256 dedup of tool outputs within a configurable window (default: 20, set `CONTEXT_PRUNE_THRESHOLD`) |
| handoff | `SessionStart` | Loads the most recent handoff state from a previous session |
| handoff | `SessionEnd` | Persists current session state to `~/.tinycode/handoff/` (or `HANDOFF_DIR`) |
| handoff | `Dispose` | Clears in-memory handoff state |

---

## SRE — OpenShift SRE / Platform Admin (10 plugins)

Cluster troubleshooting, observability, and offline diagnostics.

| Plugin | Tools | Description |
|--------|-------|-------------|
| **ocp-context-injection** | 2 | Cluster context injection + authentication |
| **ocp-must-gather** | 12 | Must-gather offline cluster analysis |
| **etcd-diag** | 6 | etcd performance diagnostics from must-gather |
| **ingress-inspect** | 5 | HAProxy/Ingress inspection from must-gather |
| **audit-logs** | 5 | API audit log analysis |
| **insights** | 10 | OpenShift Insights archive analysis |
| **ocp-obs-metrics** | 3 | Prometheus metrics, alerts, silencing |
| **ocp-obs-logging** | 5 | Loki logs, Tempo traces, NetObserv |
| **ocp-virt** | 13 | OpenShift Virtualization VM lifecycle |
| **ocp-odf** | 8 | OpenShift Data Foundation storage health |

### ocp-context-injection

Injects cluster metadata (version, nodes, operators, alerts, cost) into the system prompt on session start. Also provides `oc login` authentication.

| Tool | Description |
|------|-------------|
| `cluster_context` | Gather cluster context (version, nodes, operators, alerts) |
| `oc_login` | Authenticate to an OpenShift cluster with API token |

### ocp-must-gather

Offline cluster analysis from must-gather archives. Uses `pkg/mustgather/` library. For etcd and HAProxy deep-dives, use the specialist plugins (`etcd-diag`, `ingress-inspect`).

| Tool | Description |
|------|-------------|
| `mg_use` | Set the active must-gather directory path |
| `mg_cluster_version` | Cluster version, update channel, upgrade history |
| `mg_nodes` | Node status, roles, conditions, capacity |
| `mg_operators` | ClusterOperator available/degraded/progressing status |
| `mg_certs` | Certificate expiry across cluster secrets |
| `mg_node_logs` | Host-level journal and service logs |
| `mg_ovn` | OVN-Kubernetes diagnostics (EgressIP, NetworkPolicy) |
| `mg_prometheus` | Prometheus metrics from monitoring directory |
| `mg_machine_config` | MachineConfig and MachineConfigPool status |
| `mg_pods` | Pod status, restarts, resource usage |
| `mg_events` | Cluster events by namespace, type, or reason |
| `mg_health` | Aggregated health summary across all checks |

### etcd-diag

Specialist etcd diagnostics from must-gather logs.

| Tool | Description |
|------|-------------|
| `etcd_diag_stats` | Slow write/fsync counts, compaction durations (max/min/median/avg) |
| `etcd_diag_errors` | Error categorization: auth, storage, raft, network |
| `etcd_diag_timeline` | Timeline of leader elections, member changes, compactions, defrags |
| `etcd_diag_compare` | Cross-pod metric correlation to identify node-specific issues |
| `etcd_diag_live` | (Stub) Live Prometheus etcd metrics |
| `etcd_diag_health` | Overall etcd health: OK/WARN/CRITICAL per dimension |

### ingress-inspect

HAProxy/Ingress inspection from must-gather data.

| Tool | Description |
|------|-------------|
| `ingress_controllers` | List IngressController CRs (domain, replicas, endpoint strategy, certs) |
| `ingress_backends` | Parse HAProxy config backends (server counts, mode, balance algorithm) |
| `ingress_route_check` | Cross-reference Routes vs HAProxy config (stale backends, misconfigs) |
| `ingress_config` | HAProxy global/defaults config (timeouts, maxconn, SSL) |
| `ingress_health` | Overall ingress health (controllers, HAProxy, router pods, certs) |

### audit-logs

API audit log analysis — stream-parses JSON logs without loading full files.

| Tool | Description |
|------|-------------|
| `audit_top` | Top-N callers by user, verb, resource, or namespace |
| `audit_search` | Search events by user, verb, resource, namespace, status code |
| `audit_timeline` | Event volume over time, bucketed by interval |
| `audit_anomalies` | Detect failed auth spikes, mass deletions, privilege escalation |
| `audit_health` | Health summary: auth failures, deletion rates, event volume |

### insights

OpenShift Insights archive analysis.

| Tool | Description |
|------|-------------|
| `insights_use` | Extract and validate an Insights archive (.tar.gz) |
| `insights_summary` | Cluster summary: version, platform, node count, operator health |
| `insights_nodes` | Nodes with roles, status, capacity |
| `insights_operators` | ClusterOperator status (available/degraded/progressing) |
| `insights_memory` | Container memory metrics, OOM-kill candidates |
| `insights_etcd` | etcd operator status, member health, known issues |
| `insights_storage` | PV status, capacity, claims, storage class distribution |
| `insights_alerts` | Active alerts grouped by severity |
| `insights_uid_overlap` | UID range conflicts across namespaces |
| `insights_health` | Aggregated health summary |

### ocp-obs-metrics

| Tool | Description |
|------|-------------|
| `obs_promql` | Run PromQL instant or range query |
| `obs_alerts` | List active alerts filtered by severity and namespace |
| `obs_alert_silence` | Silence an alert with confirmation |

### ocp-obs-logging

| Tool | Description |
|------|-------------|
| `obs_logs` | Query Loki logs with LogQL or namespace/pod/severity filters |
| `obs_traces` | Search Tempo traces by service, operation, duration |
| `obs_trace_detail` | Full span tree for a trace ID |
| `obs_flow_collectors` | List FlowCollector resources (Network Observability) |
| `obs_dashboards` | List available Grafana dashboards |

### ocp-virt

OpenShift Virtualization VM lifecycle management.

| Tool | Description |
|------|-------------|
| `virt_vms` | List VMs with status, readiness, CPU, memory |
| `virt_describe` | Detailed VM info (CPU, memory, disks, volumes, networks, OS, conditions) |
| `virt_start` | Start a stopped VM |
| `virt_stop` | Stop a running VM |
| `virt_restart` | Restart a VM (delete VMI, controller recreates) |
| `virt_migrate` | Live migrate a VM to another node |
| `virt_console` | Recent serial console output from virt-launcher pod logs |
| `virt_datavolumes` | DataVolume import/clone/upload status and progress |
| `virt_templates` | Available VM templates and instance types |
| `virt_network` | NetworkAttachmentDefinitions for secondary networks |
| `virt_migrations` | VirtualMachineInstanceMigration status |
| `virt_node_capacity` | Node roles, CPU, memory capacity vs allocatable |
| `virt_health` | Service health: KubeVirt CRD, HyperConverged operator |

### ocp-odf

OpenShift Data Foundation storage health.

| Tool | Description |
|------|-------------|
| `odf_status` | StorageCluster status (phase, version) |
| `odf_ceph_status` | Ceph cluster health, capacity, OSD counts, mon quorum |
| `odf_pools` | CephBlockPools and CephFilesystems with replication config |
| `odf_pvcs` | PVCs backed by ODF storage classes |
| `odf_buckets` | ObjectBucketClaims and backing store status |
| `odf_storage_classes` | ODF StorageClasses (Ceph RBD, CephFS, NooBaa) |
| `odf_node_resources` | Node CPU, memory, ephemeral-storage capacity vs allocatable |
| `odf_health` | Service health: ODF operator, Ceph cluster status |

---

## Security — Security / Compliance (5 plugins)

Image scanning, policy enforcement, supply chain verification, log sanitization.

| Plugin | Tools | Description |
|--------|-------|-------------|
| **rhacs** | 7 | ACS security scanning, policy checks, compliance |
| **lightwell** | 6 | Package security, SLSA provenance, OSV vulnerabilities |
| **container-linter** | 3 | Containerfile linting, bootc, UBI suggestions |
| **log-sanitizer** | 0 (hook) | Redacts secrets from tool outputs |
| **safety-net** | 0 (hook) | Blocks destructive shell commands |

### rhacs

| Tool | Description |
|------|-------------|
| `rhacs_image_scan` | Scan container image for CVEs |
| `rhacs_image_check` | Check image against deploy-time policies |
| `rhacs_deployment_check` | Validate deployment YAML against security policies |
| `rhacs_violations` | List active policy violations |
| `rhacs_risk` | Risk score and factors for a deployment |
| `rhacs_compliance_scan` | Trigger a compliance scan |
| `rhacs_compliance_status` | Compliance results by standard (CIS, NIST, PCI) |

### lightwell

| Tool | Description |
|------|-------------|
| `lightwell_check_package` | Check a package against Lightwell repos |
| `lightwell_check_deps` | Scan pom.xml or requirements.txt for patches |
| `lightwell_osv` | Query OSV vulnerability data for a package |
| `lightwell_provenance` | Verify SLSA Level 3 build provenance |
| `lightwell_config_check` | Audit build config for Lightwell repo configuration |
| `lightwell_scan_containerfile` | Scan Containerfile for dependency and base image issues |

### container-linter

| Tool | Description |
|------|-------------|
| `container_lint` | Lint Containerfile against Red Hat best practice rules |
| `bootc_validate` | Validate bootc-compatible image builds |
| `container_base_suggest` | Suggest UBI base image for a use case |

### log-sanitizer

Hook-only plugin. Intercepts every tool output and applies regex-based redaction: PEM key blocks, API key prefixes (OpenAI, GitHub, AWS, Slack), bearer tokens, and high-entropy catch-all for 40+ character mixed-class strings. Matches are replaced with `[REDACTED:<type>]`. Zero configuration.

### safety-net

Hook-only plugin. Intercepts `permission.ask` for bash-type permissions and blocks: filesystem destructive (`rm -rf /`, `mkfs`, fork bombs), Kubernetes/OCP destructive (`kubectl delete namespace`, `helm uninstall` in kube-system), and git destructive (`git push --force main`). Scoped paths like `./build` are allowed.

---

## AI/ML — AI/ML / Data Science (3 plugins)

Model serving, experiment tracking, pipelines, and evaluation on RHOAI.

| Plugin | Tools | Description |
|--------|-------|-------------|
| **rhoai-mlflow** | 10 | MLflow experiments, model registry, session metrics |
| **rhoai-pipelines** | 4 | Data Science Pipelines (Kubeflow) |
| **rhoai-serving** | 14 | Model serving, evaluation, TrustyAI, workbenches, sandbox |

### rhoai-mlflow

Merged from `rhoai-mlflow-tools` + `rhoai-experiment-tracker`. Includes 4 session lifecycle hooks.

| Tool | Description |
|------|-------------|
| `mlflow_experiments` | List MLflow experiments |
| `mlflow_runs` | List runs for an experiment with optional filter |
| `mlflow_compare` | Compare multiple runs side by side (metrics, parameters) |
| `mlflow_artifacts` | List artifacts for a run |
| `mlflow_model_registry` | List registered models |
| `mlflow_model_version` | Detailed info for a specific model version |
| `mlflow_promote` | Transition model version stage (Staging, Production, Archived) |
| `mlflow_log_metric` | Log a metric to a run |
| `experiment_last_session` | Last tracked experiment session (metrics, parameters) |
| `mlflow_setup` | Instructions for deploying MLflow on OpenShift |

### rhoai-pipelines

| Tool | Description |
|------|-------------|
| `rhoai_pipeline_list` | List Data Science Pipelines |
| `rhoai_pipeline_run` | Trigger a pipeline run (with confirmation) |
| `rhoai_pipeline_status` | Check status of a pipeline run |
| `rhoai_pipeline_create` | Create a pipeline from a workflow definition |

### rhoai-serving

Merged from `rhoai-eval-trustyai` + `rhoai-model-serving`. Combined health tool.

| Tool | Description |
|------|-------------|
| `rhoai_list_models` | List deployed inference services (models) |
| `rhoai_model_status` | Detailed model status (pods, GPU allocation) |
| `rhoai_list_runtimes` | Available serving runtimes (vLLM, Caikit, TGIS) |
| `rhoai_sandbox_status` | Developer Sandbox environment status |
| `rhoai_sandbox_provision` | Provision a Developer Sandbox |
| `rhoai_eval_run` | Start model evaluation (lm-eval, ragas, garak, guidellm) |
| `rhoai_eval_status` | Check evaluation status and results |
| `rhoai_eval_compare` | Compare multiple evaluation runs |
| `rhoai_trusty_metrics` | TrustyAI fairness and drift metrics for a model |
| `rhoai_trusty_alerts` | Active TrustyAI alerts for drift and bias |
| `rhoai_workbench_list` | List RHOAI workbenches (Jupyter notebooks) |
| `rhoai_serving_health` | Connectivity check to all dependent services |

---

## Platform — Platform / Infrastructure (5 plugins)

Fleet management, CI/CD, automation, and infrastructure management.

| Plugin | Tools | Description |
|--------|-------|-------------|
| **rhacm** | 7 | ACM multi-cluster management |
| **tekton** | 6 | Tekton pipelines |
| **aap-bridge** | 7 | Ansible Automation Platform |
| **satellite** | 11 | Red Hat Satellite host/content management |
| **rhdp-provisioner** | 4 | Developer Platform demo environments |

### rhacm

| Tool | Description |
|------|-------------|
| `acm_clusters` | List managed clusters with status, version, provider |
| `acm_cluster_detail` | Detailed cluster info with addon status |
| `acm_policies` | Governance policies with compliance status |
| `acm_violations` | Active policy violations across fleet |
| `acm_applications` | ACM-managed ArgoCD applications |
| `acm_app_deploy` | Deploy ApplicationSet (with confirmation) |
| `acm_observability` | Federated PromQL via ACM Thanos |

### tekton

| Tool | Description |
|------|-------------|
| `tekton_list_pipelines` | List pipelines with task details |
| `tekton_list_runs` | PipelineRuns with status and duration |
| `tekton_run_status` | Detailed PipelineRun status |
| `tekton_run_logs` | Logs for a task in a PipelineRun |
| `tekton_list_tasks` | Available Tasks and ClusterTasks |
| `tekton_start_run` | Start a pipeline run (with confirmation) |

### aap-bridge

| Tool | Description |
|------|-------------|
| `aap_list_templates` | Job templates with last run status |
| `aap_launch_job` | Launch a job template (with confirmation) |
| `aap_job_status` | Running/completed job status |
| `aap_job_output` | Full stdout/stderr of a completed job |
| `aap_list_inventories` | Inventories with host counts |
| `aap_hub_search` | Search Automation Hub for certified collections |
| `aap_lint_playbook` | Lint an Ansible playbook |

### satellite

| Tool | Description |
|------|-------------|
| `satellite_health_check` | Probe connectivity on ports 443, 9090, 23443 |
| `satellite_hosts` | Search managed hosts by name, OS, environment |
| `satellite_host_facts` | System facts for a host (CPU, memory, OS, networking) |
| `satellite_errata` | Search errata by ID, title, type, severity |
| `satellite_content_views` | Content views with name, composite flag, publish date |
| `satellite_services` | Service health (database, cache, candlepin, pulp) |
| `satellite_tasks` | Foreman tasks with optional search filter |
| `satellite_proxies` | Smart proxies (capsules) with registered features |
| `satellite_repositories` | Repositories in org or content view |
| `satellite_rex_run` | Run a shell command on a host via REX |
| `satellite_rex_result` | Get REX job status and output |

### rhdp-provisioner

| Tool | Description |
|------|-------------|
| `rhdp_search` | Search RHDP demo catalog |
| `rhdp_provision` | Provision a demo environment (with confirmation) |
| `rhdp_status` | Check provisioning status |
| `rhdp_list_active` | List active demo environments with expiration |

---

## Developer (5 plugins)

Registry, API discovery, content, and developer hub integration.

| Plugin | Tools | Description |
|--------|-------|-------------|
| **quay** | 6 | Quay container registry |
| **rhdh** | 5 | Red Hat Developer Hub catalog |
| **rh-api-catalog** | 3 | Red Hat API catalog |
| **rh-dev-content** | 3 | Developer content (articles, RSS) |
| **rh-ecosystem-catalog** | 3 | Certified containers and operators (Pyxis) |

### quay

| Tool | Description |
|------|-------------|
| `quay_search` | Search repositories by name or keyword |
| `quay_tags` | List tags with digest, size, security scan status |
| `quay_manifest` | Inspect manifest layers, architecture, config |
| `quay_vulnerabilities` | Clair CVE scan results sorted by severity |
| `quay_labels` | Get labels on a manifest |
| `quay_health` | Quay registry connectivity check |

### rhdh

| Tool | Description |
|------|-------------|
| `rhdh_catalog_search` | Search by name, kind, or lifecycle stage |
| `rhdh_catalog_entity` | Full entity details with metadata and relations |
| `rhdh_api_spec` | Fetch OpenAPI/AsyncAPI spec for an API entity |
| `rhdh_techdocs` | Rendered TechDocs content for a component |
| `rhdh_dependencies` | Dependency graph (consumesApi, providesApi, dependsOn) |

### rh-api-catalog

| Tool | Description |
|------|-------------|
| `rh_api_list` | Browse available console.redhat.com APIs |
| `rh_api_spec` | Fetch OpenAPI spec for an API |
| `rh_api_endpoints` | List endpoints with methods, paths, parameters |

### rh-dev-content

| Tool | Description |
|------|-------------|
| `rh_dev_search` | Browse articles by topic |
| `rh_dev_article` | Read full content of an article by URL |
| `rh_dev_recent` | Recent articles from the RSS feed |

### rh-ecosystem-catalog

| Tool | Description |
|------|-------------|
| `ecosystem_search` | Search certified container images via Pyxis |
| `ecosystem_operator` | Search certified operators by package name |
| `ecosystem_browse` | Browse recent certified images or operator bundles |

---

## Essential (2 plugins)

| Plugin | Tools | Description |
|--------|-------|-------------|
| **pilot** | 4 | Multi-platform issue management (Gitea, GitHub, GitLab) |
| **telemetry** | 2 | Tool call analytics with local SQLite |

### pilot

| Tool | Description |
|------|-------------|
| `pilot_issues_list` | List issues with optional state and label filters |
| `pilot_issue_create` | Create a new issue with title, body, labels |
| `pilot_issue_update` | Update an existing issue (title, body, state, labels) |
| `pilot_issue_comment` | Add a comment to an issue |

### telemetry

| Tool | Description |
|------|-------------|
| `telemetry_report` | Aggregate summary: sessions, tool calls, top tools |
| `telemetry_query` | Query tool call records by name and recency |

---

## Configuration

Most plugins work out of the box. Plugins that connect to external APIs accept options in your tinycode config or environment variables.

### Red Hat Plugins

| Plugin | Required Options | Optional Options |
|--------|-----------------|------------------|
| ocp-context-injection | — | `apiUrl`, `clusterId`, `insecureSkipTls` |
| ocp-obs-metrics | `prometheusUrl` | `alertManagerUrl`, `token`, `namespace` |
| ocp-obs-logging | — | `lokiUrl`, `tempoUrl`, `token` |
| rhacs | `centralUrl` | `apiToken` |
| lightwell | — | `serviceAccountToken` |
| aap-bridge | `controllerUrl` | `oauthToken` |
| rhacm | — | `hubUrl`, `thanosUrl`, `token` |
| rhoai-mlflow | `mlflowUrl` | — |
| rhoai-pipelines | `pipelinesUrl` | `namespace`, `token` |
| rhoai-serving | — | `evalApiUrl`, `trustyaiUrl`, `namespace`, `token`, `routeHost`, `consoleOfflineToken` |
| satellite | `satelliteUrl` | `token` |
| rhdp-provisioner | `consoleOfflineToken` | `rhdpApiUrl` |

### General Plugins

| Plugin | Environment Variables | Notes |
|--------|----------------------|-------|
| pilot | `GITEA_TOKEN`, `GITHUB_TOKEN` or `GH_TOKEN`, `GITLAB_TOKEN` (one required) | Auto-detects platform from git remote. Override with `PILOT_PROVIDER`. See also `GITEA_URL`, `GITLAB_URL`. |
| telemetry | `TELEMETRY_DB` (optional) | Default: `~/.tinycode/telemetry.db` |

Plugins not listed above require no configuration.

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

Plugins are standalone Go binaries using the `pkg/plugin/` SDK. See [plugin-development.md](plugin-development.md) for the full SDK reference, wire protocol, testing patterns, and examples. See [plugin-sdk-design.md](plugin-sdk-design.md) for design rationale.
