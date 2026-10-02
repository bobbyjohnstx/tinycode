# Tinycode Plugin Ideas — Red Hat Product Integration

**Date:** 2026-08-21
**Context:** Plugin recommendations for [tinycode](https://github.com/bobbyjohnstx/tinycode), a local-LLM coding harness with a plugin SDK (`@tinycode/plugin`). Cross-referenced against Red Hat product portfolio via MASTER_INDEX (15 products, 10 topic indexes, 4 source indexes).
**Reference plugin:** [oh-my-tiny](https://github.com/bjohns/oh-my-tiny) — agent orchestration plugin (state, notepad, wiki, AST, LSP tools)
**Plugin API version:** 1

---

## Plugin API Surface Summary

Plugins can:

- **Register tools** — custom tools the LLM invokes during sessions (`tool` hook)
- **Add LLM providers** — model discovery and auth (`provider` + `auth` hooks)
- **Intercept chat** — modify messages, params, headers before they reach the LLM (`chat.message`, `chat.params`, `chat.headers`)
- **React to session lifecycle** — start, end, switch, model change (`session.*` hooks)
- **Inject shell environment** — env vars for all shell commands (`shell.env`)
- **Intercept tool execution** — before and after any tool runs (`tool.execute.before`, `tool.execute.after`)
- **Modify tool definitions** — change descriptions/params sent to LLM (`tool.definition`)
- **Handle permissions** — auto-allow/deny tool calls (`permission.ask`)
- **Inject system prompt** — add context to every LLM call (`experimental.chat.system.transform`)
- **Customize compaction** — modify context compaction behavior (`experimental.session.compacting`)
- **Post-process text** — modify generated text after completion (`experimental.text.complete`)
- **React to all events** — firehose of every server event (`event`)
- **Clean up** — dispose resources on unload (`dispose`)

---

## Category 1: Provider Plugins

These use the `provider` and `auth` hooks to register Red Hat-hosted LLM endpoints as tinycode providers.

### 1. RHOAI Model Serving Provider

**Red Hat product:** OpenShift AI (RHOAI) 3.5
**Plugin hooks:** `provider`, `auth`, `session.model.change`
**Complexity:** Medium

**What it does:** Registers RHOAI inference endpoints as first-class tinycode LLM providers. Tinycode already supports OpenAI-compatible endpoints natively (Ollama, vLLM, ramalama), but RHOAI adds a layer: model registry, multi-model serving, GPU-aware scheduling, and OpenShift OAuth. A dedicated plugin eliminates the manual URL-and-key configuration.

**Provider hook implementation:**
- Connects to RHOAI's model registry API to discover all deployed models (InferenceService resources)
- Returns each model with metadata: serving runtime (vLLM, Caikit, TGIS), GPU allocation, model format (GGUF, safetensors), quantization level
- Polls for new deployments on a configurable interval (or uses a watch)
- Maps RHOAI model capabilities to tinycode's model capability flags (`toolcall`, `reasoning`, `vision`)

**Auth hook implementation:**
- OpenShift OAuth flow: redirects to the cluster's OAuth server, exchanges code for token
- Token refresh handled automatically via the `auth.loader` pattern
- Prompts for cluster URL and namespace on first use (`auth.methods[].prompts`)
- Stores credentials per-cluster so multiple RHOAI instances can coexist

**Session model change hook:**
- When the user switches to an RHOAI-served model, log the switch to MLFlow (if the experiment tracker plugin is also loaded) for model comparison tracking

**RHOAI 3.5 features this enables:**
- MaaS GA (Model-as-a-Service) — managed model endpoints
- OGX (replaces Llama Stack) — model orchestration
- Model registry — centralized model catalog
- llm-d distributed inference — multi-node serving
- AutoRAG/AutoML — automated model optimization

**Why a plugin vs. manual config:** Manual config requires knowing the inference URL, API key format, and model ID for each deployed model. The plugin discovers all of this automatically and keeps the provider list in sync as models are deployed or removed. It also handles OpenShift OAuth, which is more complex than a static API key.

**Dependencies:** OpenShift OAuth auth (could be built into this plugin or shared — see Plugin 13)

---

### 2. Satellite Lightspeed Provider

**Red Hat product:** Satellite 6.17–6.18
**Plugin hooks:** `provider`, `auth`
**Complexity:** Low

**What it does:** Registers Satellite's Lightspeed endpoint as a tinycode provider. Satellite Lightspeed GA (6.17+) provides an AI assistant specifically trained on RHEL and Satellite knowledge — host management, errata, content views, subscription management, and system remediation.

**Provider hook implementation:**
- Discovers the Lightspeed endpoint from the Satellite server URL
- Registers a single model entry with metadata indicating its specialization (infrastructure/RHEL knowledge)
- Sets appropriate capability flags (text only, no tool-calling, no vision)

**Auth hook implementation:**
- Satellite API uses either basic auth or OAuth via Keycloak (Satellite 6.16+)
- Prompts for Satellite server URL and credentials on first use
- Validates connectivity before completing auth

**Use case:** When tinycode is being used for infrastructure-as-code work (Ansible playbooks targeting RHEL hosts managed by Satellite), the LLM can query Lightspeed for RHEL-specific guidance: "What errata are pending for these hosts?", "What's the recommended content view structure for this environment?"

**Limitation:** Lightspeed is specialized, not general-purpose. It complements rather than replaces the primary coding model. Best used via agent routing — the `plan` agent uses the primary model, but a custom `infra` agent routes to Lightspeed for RHEL/Satellite questions.

---

## Category 2: Tool Plugins

These use the `tool` hook to register tools the LLM can invoke during sessions.

### 3. OpenShift Cluster Operations

**Red Hat products:** OpenShift Container Platform 4.22, OpenShift Virtualization
**Plugin hooks:** `tool`, `shell.env`, `experimental.chat.system.transform`
**Complexity:** Medium-High

**What it does:** Gives the LLM direct visibility into OpenShift clusters. Tools wrap `oc` CLI commands with structured output parsing, so the LLM gets clean JSON instead of raw terminal text.

**Tools:**

| Tool | Description |
|------|-------------|
| `ocp_get_resources` | Get pods, deployments, services, routes, etc. by namespace. Returns structured JSON with status, conditions, and events. |
| `ocp_logs` | Stream or tail pod logs with container selection, since/until filtering, and previous-container support. |
| `ocp_describe` | Describe any resource with events and conditions parsed into structured format. |
| `ocp_events` | Get cluster or namespace events filtered by type (Normal/Warning), reason, or involved object. |
| `ocp_apply` | Apply a YAML manifest (with permission prompt via `context.ask()`). |
| `ocp_status` | Cluster health summary: node status, cluster operators, etcd health, API server status. |
| `ocp_routes` | List routes with TLS status, backends, and health check results. |

**OpenShift Virtualization tools (if OCP Virt is detected):**

| Tool | Description |
|------|-------------|
| `ocp_vm_list` | List VirtualMachines and VirtualMachineInstances with status. |
| `ocp_vm_action` | Start, stop, restart, pause, unpause, or migrate a VM (with permission prompt). |
| `ocp_vm_console` | Get VNC/serial console connection info for a VM. |
| `ocp_vm_snapshot` | Create or restore VM snapshots. |

**Shell.env hook:**
- Injects `KUBECONFIG` from the plugin options or auto-detects from `~/.kube/config`
- Injects `OC_EDITOR=cat` to prevent interactive editors in `oc edit`

**System prompt transform hook:**
- On session start, runs `oc version` and `oc get clusterversion` to detect OCP version
- Injects a one-line cluster context block: `<ocp-context>cluster=mycluster version=4.22.3 nodes=6 namespace=default</ocp-context>`
- If OCP Virt operator is installed, adds that to the context so the LLM knows VM tools are available

**Permission model:**
- Read-only tools (`get`, `logs`, `describe`, `events`, `status`) are auto-allowed via `permission.ask`
- Mutating tools (`apply`, `vm_action`, `vm_snapshot`) always prompt the user

**Why this matters:** The LLM currently has to ask the user to run `oc` commands and paste output. This plugin closes that loop — the LLM reads cluster state directly, diagnoses issues, and proposes fixes with full context.

---

### 4. RHACS Security Scanner

**Red Hat product:** Red Hat Advanced Cluster Security (RHACS) 4.11
**Plugin hooks:** `tool`, `tool.execute.after`
**Complexity:** Medium

**What it does:** Gives the LLM access to RHACS security scanning and policy enforcement. Tools call the RHACS API (Central) or `roxctl` CLI to scan images, check policy violations, and get vulnerability reports.

**Tools:**

| Tool | Description |
|------|-------------|
| `rhacs_image_scan` | Scan a container image for vulnerabilities. Returns CVE list with severity, CVSS score, fixable status, and component. |
| `rhacs_image_check` | Check an image against RHACS deploy-time policies. Returns pass/fail with violated policy names and remediation guidance. |
| `rhacs_deployment_check` | Check a deployment YAML against RHACS policies before applying. Catches privileged containers, host mounts, missing resource limits, etc. |
| `rhacs_violations` | List active policy violations in a namespace or cluster-wide, filtered by severity. |
| `rhacs_risk` | Get the risk score and risk factors for a specific deployment. |
| `rhacs_network_policies` | Generate network policies from observed traffic (runtime) or from manifests (build-time via roxctl). |

**Tool.execute.after hook (optional, configurable):**
- After any `shell` tool execution that runs `docker build`, `podman build`, or `oc apply` with an image reference, automatically triggers `rhacs_image_check` on the image
- Returns a warning annotation if policy violations are found
- Disabled by default; enabled via plugin option `autoScan: true`

**Auth:**
- RHACS Central API token (stored as plugin option `centralUrl` + `apiToken`)
- Or roxctl CLI auth if the binary is available locally

**RHACS 4.11 capabilities this exposes:**
- Runtime process baselines and "Unauthorized Process Execution" policy
- "Kubernetes Actions: Exec into Pod" detection (enabled by default)
- Network policy generation (build-time from manifests, runtime from observed traffic)
- "Environment Variable Contains Secret" detection (enabled by default)
- Deploy-time policy enforcement against SCCs, privileges, host paths

**Use case:** Developer is writing a Dockerfile and Kubernetes manifests in tinycode. Before they push, the LLM runs `rhacs_deployment_check` on the manifests and `rhacs_image_scan` on the built image. Violations are caught in the coding session, not in the CI pipeline 30 minutes later.

---

### 5. AAP MCP Server Bridge

**Red Hat product:** Ansible Automation Platform 2.7
**Plugin hooks:** `tool` (or MCP client configuration), `auth`, `shell.env`
**Complexity:** Low-Medium

**What it does:** Connects tinycode to AAP 2.7's GA MCP server, giving the LLM access to Ansible automation actions. Two implementation paths:

**Path A — MCP bridge (simpler):**
- Plugin configures tinycode's built-in MCP client to connect to AAP's MCP server on init
- AAP's MCP tools appear as native tinycode tools automatically
- Plugin handles auth token injection and connection lifecycle
- On `dispose`, disconnects the MCP client cleanly

**Path B — Native tools (tighter integration):**
- Plugin registers tools that call AAP's REST API directly, bypassing MCP serialization
- More control over tool descriptions, argument validation, and output formatting
- Can add tinycode-specific features (progress reporting, structured metadata)

**Tools (Path B):**

| Tool | Description |
|------|-------------|
| `aap_launch_job` | Launch a job template by name or ID. Returns job ID and status URL. Requires permission prompt. |
| `aap_job_status` | Check the status of a running or completed job. Returns status, stdout, and elapsed time. |
| `aap_job_output` | Get the full stdout/stderr of a completed job. |
| `aap_list_templates` | List available job templates with description, last run status, and launch parameters. |
| `aap_list_inventories` | List inventories with host counts. |
| `aap_hub_search` | Search Automation Hub for certified/validated collections by keyword. |
| `aap_eda_rulebooks` | List Event-Driven Ansible rulebooks and their activation status. |
| `aap_eda_trigger` | Manually trigger an EDA rulebook activation (with permission prompt). |

**Shell.env hook:**
- Injects `CONTROLLER_HOST`, `CONTROLLER_USERNAME`, `CONTROLLER_PASSWORD` (or `CONTROLLER_OAUTH_TOKEN`) for `awx` CLI compatibility
- Allows the LLM to also use `awx` CLI directly for operations the plugin doesn't cover

**Auth hook:**
- AAP Controller OAuth2 token flow
- Prompts for Controller URL on first use
- Token refresh handled automatically

**AAP 2.7 features this enables:**
- Platform Gateway — single auth across all AAP services
- MCP server (GA) — Model Context Protocol for AI agent integration
- Event-Driven Ansible — event-driven automation triggers
- Automation Hub — certified collection discovery
- OIDC workload identity — secretless auth for cloud workloads

**Use case:** Developer is writing an Ansible playbook in tinycode. The LLM can search Automation Hub for relevant collections (`aap_hub_search`), test the playbook by launching it against a dev inventory (`aap_launch_job`), check the result (`aap_job_output`), and iterate — all without leaving the coding session.

---

### 6. Quay Registry Tools

**Red Hat product:** Quay 3.17
**Plugin hooks:** `tool`, `auth`
**Complexity:** Low

**What it does:** Tools for searching, inspecting, and managing container images in a Quay registry. Quay 3.17 includes built-in Clair vulnerability scanning.

**Tools:**

| Tool | Description |
|------|-------------|
| `quay_search` | Search repositories by name or keyword. Returns repo name, description, star count, last modified. |
| `quay_tags` | List tags for a repository with digest, size, last modified, and security scan status. |
| `quay_manifest` | Get the manifest for a specific tag. Returns layers, architecture, OS, and config. |
| `quay_vulnerabilities` | Get Clair vulnerability scan results for a specific tag. Returns CVE list with severity, package, and fix version. |
| `quay_labels` | Get or set labels on a manifest (with permission prompt for set). |

**Auth hook:**
- Quay OAuth2 application token or robot account credentials
- Prompts for Quay registry URL (default: `quay.io`) and auth method

**Use case:** LLM is building a container image. It checks Quay for existing base images (`quay_search`), inspects available tags (`quay_tags`), verifies vulnerability scan status before selecting a base (`quay_vulnerabilities`), and after building, checks the new image's scan results.

**Pairs with:** RHACS scanner (Plugin 4) for policy-level checks, Lightwell dependency checker (Plugin 7) for application-layer dependency scanning.

---

### 7. Lightwell Dependency Checker

**Red Hat product:** Red Hat Lightwell Network (GA July 8, 2026)
**Plugin hooks:** `tool`
**Complexity:** Medium

**What it does:** Tools to check Java and Python dependencies against Lightwell's remediated and validated repositories. Given a project's dependency manifest, reports which dependencies have Lightwell patches available, verifies SLSA provenance, and checks OSV vulnerability data.

**Tools:**

| Tool | Description |
|------|-------------|
| `lightwell_check_deps` | Parse a `pom.xml`, `build.gradle`, `requirements.txt`, or `Pipfile.lock` and check each dependency against Lightwell repos. Returns: dependency name, current version, Lightwell patched version (if available), CVE count remediated, SLSA status. |
| `lightwell_check_package` | Check a single package (groupId:artifactId:version for Java, package==version for Python) against Lightwell. Returns patch availability, .rhlw suffix version, and CVE details. |
| `lightwell_osv` | Query Lightwell's OSV vulnerability data for a specific package. Returns active vulnerabilities with severity, description, and whether a Lightwell patch exists. |
| `lightwell_provenance` | Verify SLSA Level 3 build provenance for a specific Lightwell artifact. Returns attestation status, build pipeline, and signature verification result. |
| `lightwell_config_check` | Analyze a project's build tool config (settings.xml, build.gradle, pip.conf) and report whether it's configured to pull from Lightwell repos. Suggests config changes if not. |

**Auth:**
- Lightwell service account credentials (XXXXXXX|service-account-name format)
- Stored as plugin options; no OAuth flow needed (registry auth via `.netrc` or Maven `settings.xml`)

**How it queries Lightwell:**
- Java: checks `packages.redhat.com/lightwell/java/remediated` and `/validated` Maven repos
- Python: checks `packages.redhat.com/lightwell/network/python/remediated` and `/validated` PyPI repos
- OSV data: downloads from `packages.redhat.com/lightwell/osv/java/remediated`

**Ecosystem coverage (as of Aug 2026):**
- Java (Maven/Gradle) — GA
- Python (pip/Pipenv/Poetry) — GA
- npm, Go, Rust, .NET — NOT covered (npm and Go on roadmap, no timeline)

**Use case:** Developer opens a Java project in tinycode. The LLM runs `lightwell_check_deps` on the `pom.xml`, finds 3 dependencies with known CVEs that have Lightwell patches. It suggests updating the build config to point at the Lightwell repo and shows the specific `.rhlw-00001` versions available. The vulnerability gap drops from 45-90 days to the next `mvn dependency:resolve`.

**Novel value:** No other coding tool has supply chain patch awareness at the dependency level. Snyk and Dependabot find vulnerabilities; Lightwell actually patches them without version upgrades. This plugin surfaces that capability at development time.

---

### 8. RHDH Catalog Query

**Red Hat product:** Red Hat Developer Hub (RHDH) 1.10
**Plugin hooks:** `tool`, `auth`
**Complexity:** Low-Medium

**What it does:** Tools to query RHDH's software catalog API. When the LLM is working on code that calls another service, it can look up that service's API spec, ownership, dependencies, and documentation directly from the catalog.

**Tools:**

| Tool | Description |
|------|-------------|
| `rhdh_catalog_search` | Search the software catalog by name, kind (Component, API, System, Group), or lifecycle stage. |
| `rhdh_catalog_entity` | Get full details for a catalog entity: metadata, spec, relations, and links. |
| `rhdh_api_spec` | Fetch the OpenAPI/AsyncAPI spec for an API entity. Returns the spec document the LLM can use to understand the API contract. |
| `rhdh_techdocs` | Fetch TechDocs content for a component. Returns rendered documentation. |
| `rhdh_dependencies` | Get the dependency graph for a component: what it consumes, what consumes it. |

**Auth hook:**
- RHDH typically sits behind OpenShift OAuth or a custom auth provider
- Plugin reuses OpenShift OAuth token if the OCP plugin (Plugin 3) is also loaded
- Otherwise prompts for RHDH URL and auth token

**RHDH 1.10 features this enables:**
- Software catalog with Backstage-compatible entities
- TechDocs integration
- API documentation rendering
- MCP server support (RHDH 1.10 has MCP — similar bridge opportunity as AAP)
- Ansible plug-ins for RHDH (discovery of Ansible content in the catalog)

**Use case:** Developer is adding a new API client in tinycode. The LLM searches the RHDH catalog for the target service (`rhdh_catalog_search`), pulls its OpenAPI spec (`rhdh_api_spec`), and generates a typed client from the spec — all without the developer leaving their session or knowing where the spec lives.

---

### 9. Tekton Pipeline Runner

**Red Hat product:** OpenShift Pipelines 1.22
**Plugin hooks:** `tool`, `shell.env`
**Complexity:** Medium

**What it does:** Tools for managing Tekton pipelines on OpenShift. Trigger pipeline runs, check status, view logs, and verify supply chain attestations via the Trusted Application Pipeline.

**Tools:**

| Tool | Description |
|------|-------------|
| `tekton_list_pipelines` | List available pipelines in a namespace with last run status and duration. |
| `tekton_start_run` | Start a pipeline run with parameters. Returns PipelineRun name and watch URL. Requires permission prompt. |
| `tekton_run_status` | Check the status of a PipelineRun: task statuses, conditions, and overall result. |
| `tekton_run_logs` | Get logs for a specific task in a PipelineRun. |
| `tekton_list_tasks` | List available Tasks and ClusterTasks. |
| `tekton_enterprise_contract` | Verify Enterprise Contract results for a completed pipeline run. Returns SLSA attestation status, policy violations, and signature verification. |

**Shell.env hook:**
- Injects `KUBECONFIG` (shared with OCP plugin if loaded)
- Injects `TKN_NAMESPACE` for `tkn` CLI compatibility

**Trusted Application Pipeline integration:**
- `tekton_enterprise_contract` checks Enterprise Contract verification results
- SLSA Level 3 attestation verification (Tekton Chains)
- Policy evaluation results from the Enterprise Contract policy engine

**Use case:** Developer finishes a code change in tinycode. The LLM runs the CI pipeline (`tekton_start_run`), monitors progress (`tekton_run_status`), checks for failures (`tekton_run_logs`), and verifies the build meets Enterprise Contract policies (`tekton_enterprise_contract`) — full CI feedback loop without leaving the session.

---

## Category 3: Lifecycle and Hook Plugins

These use session lifecycle hooks, system prompt injection, and event interception.

### 10. RHOAI Experiment Tracker (Existing)

**Red Hat product:** OpenShift AI (RHOAI) 3.5 — MLFlow component
**Plugin hooks:** `tool.execute.after`, `session.end`, `session.model.change`, `event`
**Complexity:** Medium (already built by a friend)

**What it does:** Sends reasoning traces, tool call metrics, token usage, and session metadata to MLFlow in RHOAI. Enables structured experiment tracking when evaluating different local models against the same coding tasks.

**What it tracks:**
- Reasoning traces (thinking/chain-of-thought output from the model)
- Tool call count, types, and success/failure rates per session
- Token usage (input/output) per turn
- Model switches within a session (`session.model.change`)
- Session duration and compaction count
- Final session outcome (task completed, abandoned, errored)

**MLFlow integration:**
- Creates an MLFlow experiment per project
- Each tinycode session becomes an MLFlow run
- Reasoning traces logged as MLFlow artifacts
- Metrics logged as MLFlow metrics with step tracking
- Model ID logged as MLFlow parameter for A/B comparison

**Use case:** You're evaluating whether Qwen 3 32B or Granite Code 34B performs better on your codebase. Run the same task with each model. The experiment tracker logs both sessions to MLFlow. Compare tool call success rates, token efficiency, and task completion in MLFlow's comparison view.

---

### 11. Cluster Context Injection

**Red Hat products:** OpenShift 4.22, RHACS, ACM
**Plugin hooks:** `experimental.chat.system.transform`, `session.start`
**Complexity:** Low

**What it does:** On session start, queries the current OpenShift context and injects cluster metadata into the system prompt. The LLM knows what cluster it's targeting without being told.

**System prompt injection content:**
```
<cluster-context>
cluster: mycluster.example.com
version: 4.22.3
nodes: 6 (3 control-plane, 3 worker)
namespace: my-app
operators: [openshift-virtualization, rhacs-operator, openshift-gitops, openshift-pipelines]
rhacs-status: central healthy, 0 critical violations
acm-managed: true (hub: central-hub.example.com)
</cluster-context>
```

**What it queries (all via `oc` CLI on session start):**
- `oc version` — cluster version
- `oc get nodes` — node count and roles
- `oc config current-context` — active namespace
- `oc get csv -A` — installed operators (ClusterServiceVersions)
- `oc get clusterversion` — update channel and available updates
- Optional: RHACS Central health check, ACM governance compliance summary

**Session.start hook:**
- Runs all queries in parallel on session start
- Caches the result for the session duration (cluster context doesn't change per-turn)
- If `oc` is not available or not logged in, injects a note: `<cluster-context>not connected</cluster-context>`

**Why this is high-value for low effort:**
- ~50 lines of plugin code
- Eliminates the "what cluster am I on?" back-and-forth at the start of every infrastructure session
- The LLM can tailor its suggestions to the actual cluster version, installed operators, and namespace

---

### 12. EDA Session Events

**Red Hat product:** Ansible Automation Platform 2.7 — Event-Driven Ansible
**Plugin hooks:** `session.start`, `session.end`, `tool.execute.after`, `event`
**Complexity:** Medium

**What it does:** Bridges tinycode session events to Event-Driven Ansible (EDA). Coding actions in tinycode trigger operational automation via EDA rulebooks.

**Event mappings:**

| Tinycode Event | EDA Trigger | Example Automation |
|----------------|-------------|-------------------|
| `tool.execute.after` (shell: `docker build`) | `tinycode.image.built` | Trigger RHACS image scan pipeline |
| `tool.execute.after` (edit: `Dockerfile`) | `tinycode.dockerfile.changed` | Trigger container rebuild + scan |
| `tool.execute.after` (edit: `*.yaml` in `k8s/`) | `tinycode.manifest.changed` | Run `oc apply --dry-run` validation |
| `session.end` (with uncommitted git changes) | `tinycode.session.ended.dirty` | Notify Slack channel with change summary |
| `tool.execute.after` (shell: `git push`) | `tinycode.code.pushed` | Trigger full CI pipeline via Tekton |

**Implementation:**
- Plugin sends events to EDA's webhook event source endpoint
- Events include: session ID, project directory, file changed, tool name, tool args (sanitized — no secrets)
- EDA rulebook matches on event type and triggers the corresponding automation
- Plugin ships with an example EDA rulebook (`tinycode-events-rulebook.yml`) showing all supported event patterns

**Configuration:**
- Plugin option `edaEndpoint`: EDA webhook URL
- Plugin option `events`: array of event types to forward (default: all)
- Plugin option `sensitivePatterns`: regex patterns to strip from event payloads before sending

**Use case:** Team has an EDA rulebook that runs RHACS scans on every new container image. Developer builds an image in tinycode, the plugin fires `tinycode.image.built` to EDA, EDA triggers the scan, and the scan result is available in RHACS before the developer even pushes the code.

**Novel value:** No other coding tool bridges developer session activity to operational automation. This closes the gap between "developer changed something" and "the platform reacted."

---

### 13. OpenShift OAuth Auth (Shared)

**Red Hat products:** All OpenShift-hosted products (RHOAI, RHACS, Quay, RHDH, Tekton, ACM)
**Plugin hooks:** `auth`
**Complexity:** Low-Medium

**What it does:** A shared authentication plugin that provides OpenShift OAuth for any Red Hat product plugin. Authenticate once to the cluster, and all RH plugins reuse the token.

**Auth hook implementation:**
- OAuth2 authorization code flow against the OpenShift OAuth server
- Prompts for cluster URL on first use
- Browser-based login (opens the OpenShift console login page)
- Token stored per-cluster with automatic refresh
- Exposes a `getToken(clusterUrl)` function that other plugins call

**Why it's a separate plugin:**
- Without this, each RH plugin (OCP, RHACS, Quay, RHDH, Tekton) would implement its own OAuth flow
- With this, the user authenticates once and every RH plugin works
- Follows the pattern of shared auth in tinycode's plugin ecosystem (similar to how the GitHub Copilot plugin handles GitHub auth)

**Prompts:**
1. "OpenShift cluster URL" (text, placeholder: `https://api.mycluster.example.com:6443`)
2. "Authentication method" (select: "Browser login (OAuth)" / "API token (oc login)")
- If API token: "Paste your token" (text, validate: starts with `sha256~`)

**Token lifecycle:**
- OAuth tokens: auto-refresh via refresh token
- API tokens: no refresh, prompt user when expired
- Multiple clusters supported: tokens keyed by cluster URL

---

## Priority List

Ranked by daily utility, implementation effort, and novelty.

| Rank | Plugin | Value | Effort | Notes |
|------|--------|-------|--------|-------|
| **1** | RHOAI Model Serving Provider | **High** | Medium | Closes the loop: local models (Ollama) + cluster models (RHOAI), same interface. Auto-discovery eliminates manual URL config. |
| **2** | OpenShift Cluster Ops | **High** | Medium-High | Most broadly useful. Every OCP user needs cluster visibility from their coding tool. Large tool surface but each tool is straightforward. |
| **3** | AAP MCP Server Bridge | **High** | Low-Medium | AAP 2.7 already ships an MCP server. Lowest implementation effort for the highest automation surface area. Path A (MCP bridge) could be <100 lines. |
| **4** | RHACS Security Scanner | **High** | Medium | Shifts security scanning left — into the development session, not the CI pipeline. The `tool.execute.after` auto-scan hook is the differentiator. |
| **5** | Cluster Context Injection | **Medium** | **Low** | ~50 lines of code, high leverage. The LLM just knows what cluster it's targeting. Build this first as a quick win. |
| **6** | OpenShift OAuth Auth (Shared) | **Medium** | Low-Medium | Foundational. Build this alongside or before Plugins 1-4 to avoid duplicating auth flows. |
| **7** | Lightwell Dependency Checker | **Medium** | Medium | Novel — no other coding tool has supply chain patch awareness. Differentiator for Java/Python projects. Limited by ecosystem coverage (no npm/Go yet). |
| **8** | EDA Session Events | **Medium** | Medium | Novel bridge between coding and operations. Requires EDA infrastructure on the customer side, which limits initial adoption. |
| **9** | Tekton Pipeline Runner | **Medium** | Medium | Full CI feedback loop in the coding session. Most valuable when paired with OCP plugin (shared KUBECONFIG). |
| **10** | RHDH Catalog Query | **Low-Medium** | Low-Medium | Valuable in large organizations with many services. Limited value for small teams or single-service projects. |
| **11** | Quay Registry Tools | **Low-Medium** | Low | Straightforward API wrapper. Most valuable when paired with RHACS scanner for full image lifecycle. |
| **12** | RHOAI Experiment Tracker | **Medium** | Already built | Your friend built this. Include it in the plugin ecosystem as-is. |
| **13** | Satellite Lightspeed Provider | **Low** | Low | Niche — useful only for infrastructure-as-code work targeting Satellite-managed environments. |

### Recommended build order

1. **Cluster Context Injection** (Plugin 11) — quick win, <50 lines, immediate value
2. **OpenShift OAuth Auth** (Plugin 13) — foundational, unblocks all other OCP-hosted plugins
3. **RHOAI Model Serving Provider** (Plugin 1) — the headline plugin: local + cluster models in one harness
4. **AAP MCP Bridge** (Plugin 5) — low effort, high surface area, AAP already did the hard work
5. **OpenShift Cluster Ops** (Plugin 3) — the workhorse plugin, build incrementally (start with read-only tools)
6. **RHACS Scanner** (Plugin 4) — security scanning in the dev loop
7. **Lightwell Dependency Checker** (Plugin 7) — the differentiator nobody else has
8. Everything else as demand warrants

### Bundling strategy

These plugins naturally group into bundles:

- **`@tinycode/plugin-openshift`** — Plugins 2 (Cluster Ops), 5 (Context Injection), 6 (OAuth Auth). Core OCP experience.
- **`@tinycode/plugin-rhoai`** — Plugins 1 (Model Serving), 10 (Experiment Tracker). AI/ML workflow.
- **`@tinycode/plugin-security`** — Plugins 4 (RHACS), 7 (Lightwell). Security scanning at container and dependency layers.
- **`@tinycode/plugin-automation`** — Plugins 3 (AAP Bridge), 8 (EDA Events). Ansible integration.
- **`@tinycode/plugin-devex`** — Plugins 6 (Quay), 8 (RHDH), 9 (Tekton). Developer experience tools.
