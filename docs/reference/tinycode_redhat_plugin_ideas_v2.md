# Tinycode Plugin Ideas v2 — Enhancements & New Red Hat Integrations

**Date:** 2026-08-25
**Scope:** Enhancements to the 13 plugins in `tinycode_redhat_plugin_ideas.md` + new plugin ideas not covered there
**Sources:** MASTER_INDEX (7,343 product docs, 4,360 developer articles, 911 demo catalog items, 50 console.redhat.com APIs)

---

## Part 1: Enhancements to Existing Plugins

### Plugin 1 Enhancement: RHOAI Model Serving — Add RHDP Sandbox Integration

**Current spec:** Discovers models from RHOAI model registry on a customer's cluster.

**Enhancement:** Add a zero-config path via the Red Hat Developer Sandbox. developers.redhat.com offers a free OpenShift sandbox with RHOAI pre-installed. The plugin could detect when no cluster is configured and offer "Use Developer Sandbox" as a one-click option — creating a sandbox account, provisioning a namespace, and deploying a model serving endpoint automatically.

**Why it matters:** Removes the "I need a cluster first" barrier. A developer trying tinycode for the first time gets an RHOAI-served model without any infrastructure. The sandbox has GPU quotas for inference workloads.

**Developer content available:** 123 learning paths on developers.redhat.com including "Get started consuming GPU-hosted LLMs in Developer Sandbox" — the plugin could link to these directly.

---

### Plugin 3 Enhancement: OpenShift Cluster Ops — Add GitOps Awareness

**Current spec:** Tools for `oc` CLI operations (get, logs, describe, apply, etc.)

**Enhancement:** Add GitOps-aware tools when OpenShift GitOps (ArgoCD) is detected as an installed operator.

**New tools:**

| Tool | Description |
|------|-------------|
| `ocp_gitops_apps` | List ArgoCD Applications with sync status, health status, and last sync time. |
| `ocp_gitops_sync` | Trigger a sync on an ArgoCD Application (with permission prompt). |
| `ocp_gitops_diff` | Show the diff between the desired state (git) and the live cluster state for an Application. |
| `ocp_gitops_history` | Show deployment history for an Application with revision, author, and sync result. |

**Why it matters:** GitOps is the deployment model for most OCP users. When the LLM edits Kubernetes manifests, it should know whether those manifests are managed by ArgoCD and whether a manual `oc apply` would cause drift. `ocp_gitops_diff` is the key tool — it tells the LLM "don't apply this directly, commit it and ArgoCD will sync it."

**System prompt injection enhancement:** When GitOps is detected, inject `<gitops-context>managed-by=argocd apps=12 out-of-sync=2</gitops-context>` so the LLM knows to suggest git commits instead of `oc apply`.

**Product docs available:** OCP GitOps 1.21 docs (50+ pages), 15+ developer articles on ArgoCD patterns.

---

### Plugin 3 Enhancement: OpenShift Cluster Ops — Add Insights/Advisor Integration

**Current spec:** Cluster health via `oc` CLI queries.

**Enhancement:** When Red Hat Insights is connected (most OCP clusters report to console.redhat.com), query the Insights Advisor API for cluster-specific recommendations.

**New tools:**

| Tool | Description |
|------|-------------|
| `ocp_insights_recommendations` | Get active Insights Advisor recommendations for the connected cluster. Returns risk level, description, affected components, and remediation steps. |
| `ocp_insights_cves` | Get CVEs affecting the current cluster version with severity and whether an upgrade fixes them. |

**Why it matters:** Insights Advisor catches configuration issues and known risks that `oc` commands can't surface — things like "your etcd encryption isn't enabled" or "this kernel version has a known performance regression." The LLM gets proactive guidance, not just reactive diagnostics.

**API available:** `console.redhat.com` exposes `insights-advisor` and `ocp-vulnerability` REST APIs (both in the developers.redhat.com API catalog). Auth via Red Hat SSO token.

---

### Plugin 4 Enhancement: RHACS Scanner — Add Compliance Scanning

**Current spec:** Image scanning and policy violation checks.

**Enhancement:** Add compliance profile scanning. RHACS includes compliance operator integration that checks clusters against CIS Benchmarks, NIST 800-53, PCI-DSS, and HIPAA profiles.

**New tools:**

| Tool | Description |
|------|-------------|
| `rhacs_compliance_scan` | Trigger or view results of a compliance scan against a specific profile (CIS, NIST, PCI-DSS). Returns passing/failing controls with remediation. |
| `rhacs_compliance_status` | Summary compliance posture: percentage passing per profile, top failing controls. |

**Why it matters:** When the LLM is writing infrastructure-as-code or modifying cluster configs, compliance context prevents it from suggesting changes that would violate a compliance profile. The `chat.system.transform` hook could inject "this cluster must comply with PCI-DSS" when a compliance profile is active.

---

### Plugin 5 Enhancement: AAP MCP Bridge — Add Playbook Linting

**Current spec:** Launch jobs, check status, search Automation Hub.

**Enhancement:** When the LLM writes or edits an Ansible playbook, automatically lint it using `ansible-lint` and validate it against the team's custom rules. This is a `tool.execute.after` hook on file edits to `.yml`/`.yaml` files in roles/playbooks directories.

**New tool:**

| Tool | Description |
|------|-------------|
| `aap_lint_playbook` | Run `ansible-lint` on a playbook file with the project's configured profile. Returns rule violations with line numbers and fix suggestions. |

**Why it matters:** Local models generate Ansible playbooks with common mistakes — deprecated modules, missing `become`, FQCN violations. Catching these immediately (not after a failed job run) saves a round trip. This is the Ansible equivalent of running `tsc` after editing TypeScript.

---

### Plugin 7 Enhancement: Lightwell — Add Containerfile/Dockerfile Scanning

**Current spec:** Scans `pom.xml` and `requirements.txt` for dependency patches.

**Enhancement:** Also scan `Containerfile`/`Dockerfile` for `pip install` and Maven dependency declarations embedded in build steps. Many Java/Python apps install dependencies directly in the container build rather than via a separate manifest file.

**New behavior:**
- `lightwell_check_deps` accepts a Dockerfile path
- Parses `RUN pip install`, `RUN mvn`, `COPY requirements.txt` references
- Reports Lightwell patch availability for dependencies found in build steps

**Why it matters:** In containerized workflows, the Dockerfile IS the dependency manifest. Missing this means the plugin only catches dependencies declared in build tool configs, not the ones installed at image build time.

---

### Plugin 11 Enhancement: Cluster Context — Add Cost Context

**Current spec:** Injects cluster version, nodes, operators, namespace.

**Enhancement:** Query the console.redhat.com Cost Management API and inject cost context for the active namespace/project.

**System prompt injection addition:**
```
cost-context: namespace=my-app monthly-estimate=$847 top-resource=gpu-worker-0 ($412/mo)
```

**Why it matters:** When the LLM is making resource decisions (replica counts, resource requests/limits, GPU allocation), cost context prevents it from suggesting configurations that would blow the budget. "Add 3 GPU replicas" hits different when the LLM knows the namespace is already at $847/month.

**API available:** `cost-management` API in the developers.redhat.com API catalog.

---

## Part 2: New Plugin Ideas

### 14. Red Hat Developer Content Search

**Plugin hooks:** `tool`, `experimental.chat.system.transform`
**Complexity:** Low-Medium
**Red Hat source:** developers.redhat.com (4,360 pages: 918 articles, 123 learning paths, 106 cheat sheets, 118 e-books)

**What it does:** Gives the LLM searchable access to developers.redhat.com content — articles, learning paths, cheat sheets, and e-books. When the LLM is generating code that involves a Red Hat technology, it can pull the official tutorial or cheat sheet to improve accuracy.

**Tools:**

| Tool | Description |
|------|-------------|
| `rh_dev_search` | Search developers.redhat.com articles, learning paths, and cheat sheets by keyword. Returns title, URL, summary, and content type. |
| `rh_dev_article` | Fetch the full text of a specific developer article by URL or ID. Returns the article content for the LLM to reference. |
| `rh_dev_cheatsheet` | Fetch a cheat sheet by topic (e.g., "podman", "bash", "containers", "quarkus"). Returns the command reference directly. |
| `rh_dev_learning_path` | Get the step-by-step learning path for a topic. Returns ordered module list with descriptions and prerequisites. |

**Implementation options:**

**Option A — Local repo search (no network, fastest):**
- The offline repo already has 4,360 developer articles as TXT files in `4_public_redhat/RH_developers/txt/`
- Plugin indexes these locally with a simple keyword search (title + first paragraph)
- Returns file content directly — no API call, no auth, works offline
- Limitation: content freshness tied to last repo refresh (July 2026)

**Option B — Live API search (current content):**
- developers.redhat.com has a search API
- Returns current content with proper metadata
- Requires network access

**Option A is the better fit for tinycode's "local-first" philosophy.** The 4,360 TXT files are already on disk. A simple index (title → file path) loads in milliseconds. The LLM gets Red Hat developer content without any API call or authentication.

**System prompt transform hook:**
- Detects the project language/framework from the directory (pom.xml → Java/Quarkus, package.json → Node.js, Containerfile → containers)
- Injects a one-liner: `<rh-dev-context>Relevant cheat sheets: podman, containers, buildah. Learning paths: deploy-containers-podman</rh-dev-context>`
- The LLM then knows what reference content is available and can pull it with `rh_dev_cheatsheet`

**Use case:** Developer is writing a Containerfile in tinycode. The LLM calls `rh_dev_cheatsheet("buildah")` and gets the official Red Hat buildah cheat sheet. It uses correct syntax and best practices instead of hallucinating flags. For Quarkus projects, the LLM pulls the Quarkus getting-started guide and generates correct `application.properties` entries.

**Why this improves accuracy:** Local models (7B-34B) frequently hallucinate CLI flags, config properties, and API signatures. Having the actual cheat sheet or tutorial in context grounds the model's output in verified content. This is RAG for coding — but the retrieval is a local file read, not a vector DB query.

---

### 15. Red Hat API Catalog Integration

**Plugin hooks:** `tool`, `auth`
**Complexity:** Medium
**Red Hat source:** developers.redhat.com/api-catalog (50 APIs for console.redhat.com services)

**What it does:** Gives the LLM access to the OpenAPI specs for all 50 console.redhat.com APIs. When the LLM is writing code that integrates with Red Hat cloud services (Insights, Cost Management, Image Builder, Compliance, Inventory, Patch, Vulnerability, Subscriptions, etc.), it can fetch the exact API spec and generate correct client code.

**Tools:**

| Tool | Description |
|------|-------------|
| `rh_api_list` | List all available Red Hat APIs with name, description, and version. |
| `rh_api_spec` | Fetch the OpenAPI spec for a specific Red Hat API. Returns the spec document the LLM uses to generate client code. |
| `rh_api_endpoints` | List endpoints for a specific API with method, path, description, and required parameters. Lighter than the full spec for quick lookups. |

**APIs available (50 in the catalog):**
- `insights-advisor` — cluster recommendations
- `compliance` (v1, v2) — policy compliance
- `cost-management` — cloud spend
- `image-builder` — OS image composition
- `inventory` — host/system inventory
- `patch` — errata/patch management
- `vulnerability` — CVE tracking
- `rhsm-subscriptions` — subscription usage
- `remediations` — Insights remediation playbooks
- `playbook-dispatcher` — run remediation playbooks
- `ansible-automation-controller` — AAP API
- `automation-hub` — certified content API
- And 38 more

**Why this improves accuracy:** When the LLM generates code to call `console.redhat.com/api/cost-management/v1/reports/`, it currently guesses at the endpoint structure. With the actual OpenAPI spec in context, it generates correct paths, parameters, and auth headers. This is the difference between "plausible-looking API code" and "code that actually works."

**Pairs with:** Plugin 3 (OCP Cluster Ops) for Insights recommendations, Plugin 11 (Cluster Context) for cost data injection.

---

### 16. RHDP Demo Environment Provisioner

**Plugin hooks:** `tool`, `auth`
**Complexity:** Medium
**Red Hat source:** Red Hat Demo Platform catalog (911 items: workshops, demos, labs, open environments)

**What it does:** Search and provision demo environments from the Red Hat Demo Platform (RHDP/RHPDS) directly from tinycode. When the LLM needs a cluster to test against, it can find and request the right demo environment.

**Tools:**

| Tool | Description |
|------|-------------|
| `rhdp_search` | Search the demo catalog by keyword, product, or category. Returns environment name, description, duration, and provisioning time. |
| `rhdp_provision` | Request a demo environment (with permission prompt). Returns provisioning status and credentials when ready. |
| `rhdp_status` | Check the status of a provisioned environment. Returns running/provisioning/expired with connection details. |
| `rhdp_list_active` | List all currently active demo environments with remaining time and connection info. |

**Catalog content (911 items):**
- AAP workshops (multi-instance, self-service automation)
- Agentic AI on OpenShift (Llama Stack, 3rd party frameworks)
- GenAIOps enablement (AI501)
- Edge appliances with Image Mode + MicroShift
- And 900+ more across all RH products

**Use case:** Developer is building an AAP integration in tinycode. The LLM searches for an AAP demo environment (`rhdp_search("ansible automation")`), provisions one (`rhdp_provision`), and uses the credentials to test the integration against a live AAP instance. When the session ends, the environment auto-expires.

**Why this matters:** "I don't have a cluster to test against" is the most common blocker for RH product integrations. RHDP environments are free for Red Hat associates and partners. The plugin removes the friction of navigating the demo portal manually.

---

### 17. Containerfile/Image Mode Linter

**Plugin hooks:** `tool`, `tool.execute.after`
**Complexity:** Low-Medium
**Red Hat products:** RHEL Image Mode (bootc), Podman Desktop, Quay

**What it does:** Validates Containerfiles and bootc-compatible image definitions against Red Hat best practices. Catches common mistakes that local models make when generating container build files.

**Tools:**

| Tool | Description |
|------|-------------|
| `container_lint` | Lint a Containerfile/Dockerfile against Red Hat best practices. Returns warnings for: non-UBI base images, running as root, missing labels, `latest` tag usage, unnecessary `RUN` layer chaining, missing `USER` directive, hardcoded secrets. |
| `bootc_validate` | Validate a bootc-compatible image definition. Checks: base image is bootc-compatible, required bootc labels present, filesystem layout correct for image mode, systemd unit files properly structured. |
| `container_base_suggest` | Given a use case (e.g., "Java 21 app", "Python 3.12 ML inference", "Node.js 22"), suggest the correct UBI base image with registry URL and tag. |

**Tool.execute.after hook:**
- When the LLM edits a `Containerfile`, `Dockerfile`, or any file matching `*.containerfile`, automatically runs `container_lint` and appends warnings to the tool output
- Disabled by default; enabled via plugin option `autoLint: true`

**Why this improves accuracy:** Local models generate Containerfiles with `FROM ubuntu:latest` instead of `FROM registry.access.redhat.com/ubi9/ubi-minimal:9.4`. They add `USER root` unnecessarily. They miss required OCP labels. This plugin catches all of that immediately.

**RHEL Image Mode (bootc) value:** bootc is a new paradigm — OS-as-a-container. The rules are different from traditional Dockerfiles (bootc images must use specific base images, have systemd, follow a specific filesystem layout). Local models have zero training data on bootc. This plugin provides the guardrails.

**Developer content available:** RHEL Image Mode product page and getting-started guide on developers.redhat.com. 3 developer articles. Edge appliance demo in RHDP catalog.

---

### 18. Ecosystem Catalog Search

**Plugin hooks:** `tool`
**Complexity:** Low
**Red Hat source:** Ecosystem Catalog (25,952 pages, 265 videos — catalog.redhat.com)

**What it does:** Searches the Red Hat Ecosystem Catalog for certified and validated partner solutions. When the LLM is recommending storage, networking, security, or middleware solutions for OpenShift, it can verify certification status and find the correct operator name.

**Tools:**

| Tool | Description |
|------|-------------|
| `ecosystem_search` | Search the ecosystem catalog by keyword, category (storage, networking, security, AI/ML), or platform (OCP, RHEL, Ansible). Returns partner name, product, certification level, and operator name. |
| `ecosystem_operator` | Get details for a certified operator: supported OCP versions, installation instructions, certified configuration, and support contact. |
| `ecosystem_hardware` | Search for certified hardware (servers, storage arrays, network devices) by vendor or model. Returns certification status and supported RHEL/OCP versions. |

**Implementation:** Local search against the 25,952 JSON pages already in `4_public_redhat/RH_ecosystem/`. No API call needed. Simple keyword index over partner names and product descriptions.

**Use case:** Customer asks "can we use NetApp for persistent storage on OpenShift Virt?" The LLM runs `ecosystem_search("NetApp OpenShift Virtualization storage")`, gets the certified Trident CSI driver entry, and responds with the exact operator name, certified versions, and installation link — all verified against the catalog, not hallucinated.

**Why this improves accuracy:** Partner names, operator names, and version compatibility are the most commonly hallucinated content in RH product conversations. The catalog is ground truth.

---

### 19. Red Hat Learning Path Guide

**Plugin hooks:** `tool`, `experimental.chat.system.transform`
**Complexity:** Low
**Red Hat sources:** developers.redhat.com learning paths (123), Udemy Business (redhat.udemy.com), Red Hat Training

**What it does:** When the LLM detects that the user is learning or exploring (not just coding), it can recommend structured learning paths from developers.redhat.com and Udemy Business.

**Tools:**

| Tool | Description |
|------|-------------|
| `rh_learn_path` | Find learning paths by topic or product. Returns ordered module list with difficulty, duration, and sandbox availability. |
| `rh_learn_module` | Get the content of a specific learning module. Returns step-by-step instructions the user can follow. |
| `rh_learn_suggest` | Given the current project context (language, framework, tools), suggest relevant learning paths the user hasn't completed. |

**System prompt transform hook:**
- Detects when the user's messages indicate learning intent ("how does X work?", "I've never used Y before", "can you explain Z?")
- Injects a note: `<learning-context>User appears to be learning. Relevant paths: [list]. Consider suggesting structured learning resources alongside direct answers.</learning-context>`

**Why this matters for tinycode specifically:** Tinycode targets local LLM users — many of whom are exploring Red Hat technologies for the first time. A learning path recommendation is more valuable than a raw answer because it builds understanding rather than creating dependency on the AI.

---

## Part 3: Enhanced Priority List (All 19 Plugins)

### Tier 1 — Build First (highest daily value, lowest friction)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 1 | **Cluster Context Injection** (11) + Cost + GitOps enhancements | Lifecycle | ~100 lines with enhancements. Immediate value. |
| 2 | **Red Hat Developer Content Search** (14) | Tool | Local file search over 4,360 articles. No auth. Directly improves code accuracy. |
| 3 | **Containerfile/Image Mode Linter** (17) | Tool | Catches the most common local-model mistakes. Auto-lint on edit. |

### Tier 2 — Core Infrastructure (enables everything else)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 4 | **OpenShift OAuth Auth** (13) | Auth | Shared auth. Build before or alongside OCP/RHOAI plugins. |
| 5 | **RHOAI Model Serving** (1) + Sandbox enhancement | Provider | Headline feature. Sandbox path removes "need a cluster" barrier. |
| 6 | **AAP MCP Bridge** (5) + Playbook linting | Tool | AAP already did the MCP work. Linting catches bad playbook generation. |

### Tier 3 — Cluster Operations (the workhorse)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 7 | **OpenShift Cluster Ops** (3) + GitOps + Insights | Tool | Build incrementally: read-only first, GitOps awareness, then Insights. |
| 8 | **RHACS Scanner** (4) + Compliance | Tool | Security scanning + compliance profiles in the dev loop. |
| 9 | **Tekton Pipeline Runner** (9) | Tool | CI feedback loop. Pairs with OCP plugin. |

### Tier 4 — Differentiation (novel, no one else has these)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 10 | **Lightwell Dependency Checker** (7) + Containerfile scanning | Tool | Supply chain patches at dev time. Dockerfile parsing is the enhancement. |
| 11 | **Ecosystem Catalog Search** (18) | Tool | Local search, no auth. Eliminates partner/operator hallucination. |
| 12 | **Red Hat API Catalog** (15) | Tool | OpenAPI specs for 50 console.redhat.com APIs. Correct client code generation. |

### Tier 5 — Specialized (valuable for specific workflows)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 13 | **EDA Session Events** (12) | Lifecycle | Coding-to-operations bridge. Requires EDA infrastructure. |
| 14 | **RHDP Demo Provisioner** (16) | Tool | "I need a cluster to test against" — removes the blocker. |
| 15 | **RHDH Catalog Query** (8) | Tool | Service catalog lookups. Large org value only. |
| 16 | **Quay Registry Tools** (6) | Tool | Image registry. Pairs with RHACS. |
| 17 | **Red Hat Learning Path Guide** (19) | Tool | Learning-mode detection. Niche but good for adoption. |
| 18 | **RHOAI Experiment Tracker** (10) | Lifecycle | Already built. Include as-is. |
| 19 | **Satellite Lightspeed Provider** (2) | Provider | Infra-as-code niche. |

### What moved and why

**Developer Content Search (14) jumped to Tier 1** because it directly improves code accuracy for every Red Hat technology — no auth, no network, local file search over 4,360 articles already on disk. It's the simplest way to reduce local-model hallucination.

**Containerfile Linter (17) is Tier 1** because Containerfile generation is the single most common task where local models make Red Hat-specific mistakes (wrong base image, missing UBI, no labels). Auto-lint-on-edit catches these without the user asking.

**Ecosystem Catalog Search (18) is Tier 4** because it eliminates partner/operator hallucination — a surprisingly common problem when customers ask "does X work with OpenShift?" and the LLM guesses.

### Bundling strategy (updated)

- **`@tinycode/plugin-openshift`** — Plugins 3, 11, 13 (Cluster Ops + Context + OAuth + GitOps + Insights + Cost)
- **`@tinycode/plugin-rhoai`** — Plugins 1, 10 (Model Serving + Experiment Tracker + Sandbox)
- **`@tinycode/plugin-security`** — Plugins 4, 7, 17 (RHACS + Lightwell + Container Linter)
- **`@tinycode/plugin-automation`** — Plugins 5, 12 (AAP Bridge + EDA Events + Playbook Linting)
- **`@tinycode/plugin-devex`** — Plugins 6, 8, 9 (Quay + RHDH + Tekton)
- **`@tinycode/plugin-reference`** — Plugins 14, 15, 18, 19 (Developer Content + API Catalog + Ecosystem + Learning Paths). Local-first, no auth needed.

The reference bundle is the easiest to ship — all local file searches, no APIs, no auth. It could ship before any of the cluster-connected plugins.

---

## Part 4: Major Gaps — RHACM, Observability, MLFlow, RHOAI Deep Features

These were missing from both the original 13 and the v2 additions above. They represent some of the most powerful integration surfaces in the Red Hat stack.

---

### 20. RHACM Multi-Cluster Management

**Plugin hooks:** `tool`, `auth`, `experimental.chat.system.transform`
**Complexity:** Medium-High
**Red Hat product:** Red Hat Advanced Cluster Management (RHACM) 2.14+, Multicluster Engine (MCE)

**What it does:** Gives the LLM fleet-level visibility and management across all managed OpenShift clusters. ACM is the management plane — it knows about every cluster, their health, policy compliance, governance violations, and application deployments. This is the multi-cluster equivalent of Plugin 3 (OCP Cluster Ops), which is single-cluster only.

**Tools:**

| Tool | Description |
|------|-------------|
| `acm_clusters` | List all ManagedClusters with status (Ready/NotReady), version, provider (AWS/bare metal/vSphere), labels, and last heartbeat. |
| `acm_cluster_detail` | Get detailed info for a specific managed cluster: node count, installed operators, addon status, conditions, and compliance summary. |
| `acm_policies` | List governance policies with compliance state per cluster. Returns policy name, template (ConfigurationPolicy, CertificatePolicy, etc.), cluster binding, and compliant/non-compliant/pending status. |
| `acm_violations` | Get active policy violations across the fleet. Returns: cluster, policy, violation detail, severity, remediation action (inform/enforce), and enforcement status. |
| `acm_applications` | List ACM-managed Applications and ApplicationSets with placement status (which clusters they're deployed to) and sync status. |
| `acm_app_deploy` | Deploy an Application or ApplicationSet to target clusters via placement rules (with permission prompt). |
| `acm_observability` | Query ACM's multi-cluster observability (Thanos-backed). Run PromQL queries across ALL managed clusters from a single endpoint. Returns federated metrics. |

**System prompt transform hook:**
```
<acm-context>
hub: central-hub.example.com
managed-clusters: 12 (10 Ready, 1 NotReady, 1 Importing)
governance: 47 policies, 3 non-compliant clusters
non-compliant: [prod-east: "etcd-encryption", staging: "image-policy", dev-gpu: "resource-limits"]
applications: 8 deployed, 1 degraded (payments-api on prod-west)
</acm-context>
```

**Why this is a huge gap:** ACM is the most natural "tinycode integration point" for enterprise OCP users — they don't manage one cluster, they manage 10-100. The LLM needs fleet context to give useful advice. Without it:
- "Scale up the API deployment" → which cluster? The LLM has to ask.
- "Why is prod slow?" → no visibility into whether the issue is on one cluster or three.
- "Are we compliant?" → the LLM can't check governance posture.

**ACM multi-cluster observability value:** ACM ships with Thanos-based federated metrics. `acm_observability` runs a single PromQL query across every managed cluster and returns aggregated results. This is far more powerful than querying Prometheus on one cluster at a time (Plugin 3).

**Pairs with:** Plugin 3 (OCP Cluster Ops) for single-cluster drill-down after ACM identifies the problem cluster, Plugin 11 (Cluster Context) for system prompt enrichment.

---

### 21. Observability Stack — Prometheus, Loki, Tempo, AlertManager

**Plugin hooks:** `tool`, `experimental.chat.system.transform`
**Complexity:** Medium
**Red Hat products:** OpenShift Cluster Monitoring (Prometheus), Cluster Logging (Vector + Loki), Distributed Tracing (OpenTelemetry + Tempo), Cluster Observability Operator (COO), Network Observability

**What it does:** Gives the LLM direct access to the MELT stack (Metrics, Events/Logging, Tracing). This is the observability surface that every OCP cluster ships with. When the LLM is debugging an issue, it can query metrics, search logs, find traces, and check alerts — the same workflow a human SRE follows, but faster.

**Tools:**

| Tool | Description |
|------|-------------|
| `obs_promql` | Run a PromQL query against Prometheus (cluster monitoring or User Workload Monitoring). Returns time-series data, instant vectors, or scalar values. Supports range queries with step intervals. |
| `obs_alerts` | List firing and pending alerts from AlertManager. Returns: alert name, severity (critical/warning/info), namespace, description, since, and silenced status. |
| `obs_alert_silence` | Silence an alert for a duration (with permission prompt). |
| `obs_logs` | Run a LogQL query against Loki via the Cluster Logging stack. Search by namespace, pod, container, severity, and regex. Returns log lines with timestamps. |
| `obs_traces` | Query Tempo for distributed traces by service name, operation, duration threshold, or trace ID. Returns trace spans with timing, status, and service graph. |
| `obs_trace_detail` | Get the full span tree for a specific trace ID. Returns the entire call chain with latency per span. |
| `obs_network_flows` | Query Network Observability (eBPF-based) for traffic flows between pods, namespaces, or nodes. Returns source/dest, bytes, packets, protocol, and direction. |
| `obs_dashboards` | List available Grafana dashboards (if COO or external Grafana is installed). Returns dashboard name, folder, and URL. |

**System prompt transform hook:**
```
<observability-context>
firing-alerts: 2 critical (KubePodCrashLooping: payments-api, NodeFilesystemAlmostOutOfSpace: worker-3)
pending-alerts: 1 warning (TargetDown: user-workload-monitoring)
</observability-context>
```

The hook queries AlertManager on session start and injects firing alerts. The LLM immediately knows "there's a crashlooping pod and a disk issue" without being asked to check.

**Why this is a fundamental gap:** The observability stack is the debugging surface. Without it, the LLM can look at pod status and logs (Plugin 3), but can't:
- See if CPU/memory metrics are trending toward a resource limit
- Correlate a 500 error spike with a deployment rollout
- Find the specific trace that shows where latency is introduced
- Check if the error rate alerting threshold has been breached
- See which pods are generating the most network traffic

**PromQL examples the LLM can run:**
- `rate(container_cpu_usage_seconds_total{namespace="my-app"}[5m])` — CPU usage
- `histogram_quantile(0.99, rate(http_request_duration_seconds_bucket{service="api"}[5m]))` — p99 latency
- `sum(rate(http_requests_total{status=~"5.."}[5m])) by (service)` — 5xx rate by service
- `kubelet_volume_stats_available_bytes / kubelet_volume_stats_capacity_bytes` — disk free %

**Permission model:**
- All tools are read-only except `obs_alert_silence` (requires permission prompt)
- PromQL queries constrained to namespaces the user's token has access to (same RBAC as the OpenShift console)

**Pairs with:** Plugin 3 (OCP Cluster Ops) — metrics context + resource context = full debugging picture. Plugin 20 (RHACM) — federated observability via `acm_observability` for cross-cluster queries.

---

### 22. MLFlow Experiment & Model Registry Tools (Read + Write)

**Plugin hooks:** `tool`
**Complexity:** Medium
**Red Hat product:** RHOAI 3.5 — MLflow (GA in 3.4, Operator-managed)

**What it does:** Full read/write access to MLflow experiments, runs, and the model registry. Plugin 10 (Experiment Tracker) writes session telemetry TO MLflow. This plugin gives the LLM the ability to READ from MLflow — compare runs, pull metrics, query the model registry, and promote models between stages.

**Tools:**

| Tool | Description |
|------|-------------|
| `mlflow_experiments` | List MLflow experiments with run counts, last activity, and lifecycle stage. |
| `mlflow_runs` | List runs for an experiment with status (RUNNING/FINISHED/FAILED), metrics summary (loss, accuracy, latency), parameters, and duration. |
| `mlflow_compare` | Compare 2-5 runs side by side. Returns a diff table of parameters and metrics. The LLM can answer "which model performed better?" with data. |
| `mlflow_artifacts` | List artifacts for a run (model files, configs, plots, reasoning traces). Returns artifact paths and sizes. |
| `mlflow_model_registry` | List registered models with latest version, stage (Staging/Production/Archived), and description. |
| `mlflow_model_version` | Get details for a specific model version: source run, metrics at registration, stage transitions, and tags. |
| `mlflow_promote` | Transition a model version to a new stage (None → Staging → Production) (with permission prompt). |
| `mlflow_log_metric` | Log a metric to the current or specified run. Useful when the LLM runs an evaluation and wants to record the result. |

**Use cases:**

1. **Model comparison during development:** "Compare the last 3 Granite runs" → `mlflow_compare` returns a table showing accuracy, latency, and token efficiency per run. The LLM recommends the best one.

2. **Model promotion workflow:** "Promote granite-7b-v3 to production" → `mlflow_promote` transitions the model version from Staging to Production, updating the model registry. The RHOAI model serving plugin (Plugin 1) can then discover the promoted model and serve it.

3. **Plugin 10 integration:** Plugin 10 writes session traces to MLflow. This plugin reads them back — "show me the reasoning traces from yesterday's session" → `mlflow_artifacts` pulls the trace files.

4. **EvalHub result lookup:** EvalHub (see Plugin 23) writes evaluation results to MLflow. This plugin reads them — "what was the RAGAS score for the last RAG eval?" → `mlflow_runs` + `mlflow_compare`.

**Why it's separate from Plugin 10:** Plugin 10 is a lifecycle hook that passively logs data. This plugin is an active tool the LLM invokes when it needs MLflow data. Different hooks, different use patterns. They can share auth.

---

### 23. RHOAI Platform Tools (Data Science Pipelines, EvalHub, TrustyAI, Workbenches)

**Plugin hooks:** `tool`, `auth`
**Complexity:** High (broad surface area — build incrementally)
**Red Hat product:** RHOAI 3.5 — Data Science Pipelines, EvalHub, TrustyAI, Workbenches, Model Registry, AutoRAG, GenAI Playground

**What it does:** Exposes the full RHOAI platform surface beyond model serving (Plugin 1) and experiment tracking (Plugin 10). This is the "RHOAI workbench" for the LLM — it can create and run ML pipelines, trigger model evaluations, monitor model health, and manage data science projects.

**Tools — Data Science Pipelines (Kubeflow Pipelines):**

| Tool | Description |
|------|-------------|
| `rhoai_pipeline_list` | List Data Science Pipelines in a project with status, last run, and schedule. |
| `rhoai_pipeline_run` | Trigger a pipeline run with parameters (with permission prompt). Returns run ID and status URL. |
| `rhoai_pipeline_status` | Check run status: step-by-step task completion, logs, and output artifacts. |
| `rhoai_pipeline_create` | Create a pipeline from a Python function or YAML definition (with permission prompt). |

**Tools — EvalHub (Model Evaluation):**

| Tool | Description |
|------|-------------|
| `rhoai_eval_run` | Run an evaluation against a model using a specified provider (LM-Eval, RAGAS, Garak, GuideLLM). Returns eval job ID. |
| `rhoai_eval_status` | Check evaluation status and results. Returns scores per metric, comparison to baseline, and pass/fail against thresholds. |
| `rhoai_eval_compare` | Compare evaluation results across multiple models or RAG configurations. Returns a ranked leaderboard. |

**Tools — TrustyAI (Model Monitoring):**

| Tool | Description |
|------|-------------|
| `rhoai_trusty_metrics` | Get TrustyAI metrics for a deployed model: data drift score, feature distribution changes, prediction bias metrics. |
| `rhoai_trusty_alerts` | List active TrustyAI alerts (drift detected, bias threshold exceeded). |

**Tools — Workbench Management:**

| Tool | Description |
|------|-------------|
| `rhoai_workbench_list` | List workbenches (Jupyter environments) in a project with status, image, and GPU allocation. |
| `rhoai_workbench_create` | Create a new workbench with specified image, resources, and storage (with permission prompt). |

**Tools — Model Registry:**

| Tool | Description |
|------|-------------|
| `rhoai_registry_list` | List registered models with version, format (ONNX, safetensors, GGUF), serving status, and metadata. |
| `rhoai_registry_deploy` | Deploy a model from the registry to a serving endpoint (with permission prompt). |

**Tools — AutoRAG (Tech Preview):**

| Tool | Description |
|------|-------------|
| `rhoai_autorag_run` | Start an AutoRAG optimization run with a document set. AutoRAG tests combinations of chunking, embedding, retrieval, and generation. |
| `rhoai_autorag_results` | Get AutoRAG leaderboard: ranked RAG configurations with scores. |

**Why this is a high-value plugin despite the complexity:** RHOAI 3.5 is the most feature-rich AI platform Red Hat has ever shipped. The LLM should be able to orchestrate the full ML lifecycle:
1. Create a pipeline → `rhoai_pipeline_create`
2. Run training → `rhoai_pipeline_run`
3. Register the model → `rhoai_registry_list` (model auto-registered)
4. Evaluate the model → `rhoai_eval_run` (with RAGAS for RAG, Garak for security)
5. Check for bias → `rhoai_trusty_metrics`
6. Deploy to serving → `rhoai_registry_deploy`
7. Monitor in production → `rhoai_trusty_alerts`

That's the full MLOps loop from tinycode.

**Build incrementally:** Start with read-only tools (pipeline_list, eval_status, trusty_metrics), then add write tools as the auth story matures.

---

### 24. RHOAI MCP Bridge (RHOAI 3.5 Ships an MCP Server)

**Plugin hooks:** `tool` (or MCP client configuration)
**Complexity:** Low
**Red Hat product:** RHOAI 3.5 — RHOAI MCP Server (Developer Preview)

**What it does:** Bridges tinycode to RHOAI's own MCP server — the same approach as Plugin 5 (AAP MCP Bridge). RHOAI 3.5 ships an MCP server that enables MCP-compatible clients to interact with the platform via natural language.

**What RHOAI's MCP server already provides:**
- Model recommendation from registry (matches against MMLU, HumanEval benchmarks with cost comparisons)
- Project management (create/list/manage data science projects)
- Workbench creation and management
- Pipeline monitoring
- Production-ready Kubernetes manifest generation (model serving, auto-scaling, observability)

**Implementation (same as AAP MCP bridge — two paths):**

**Path A — MCP bridge (simpler):**
- Plugin configures tinycode's MCP client to connect to the RHOAI MCP server
- All RHOAI MCP tools appear as native tinycode tools automatically
- Plugin handles OpenShift OAuth token injection
- Could be <100 lines

**Path B — Native tools (if finer control is needed):**
- Plugin wraps the RHOAI REST APIs directly
- More control over tool descriptions and output formatting
- Can add tinycode-specific features (progress reporting, structured metadata)

**Why Path A first:** RHOAI already did the hard work of building the MCP server. The plugin is just a bridge. Same logic as the AAP MCP bridge — ship the bridge first, then wrap individual tools natively if the MCP interface is too coarse.

**Pairs with:** Plugin 1 (RHOAI Model Serving) for provider integration, Plugin 22 (MLFlow tools) for experiment data, Plugin 23 (RHOAI Platform Tools) for direct API access. The MCP bridge and native tools complement each other — the MCP bridge gives broad coverage fast, native tools give precision where it matters.

---

### Plugin 10 Enhancement: RHOAI Experiment Tracker — Add Read-Side Integration

**Current spec:** Writes reasoning traces, tool call metrics, and session metadata TO MLflow.

**Enhancement:** Add a `tool.execute.after` hook on Plugin 22 (MLFlow tools) that enriches MLflow query results with tinycode session context. When the LLM queries `mlflow_compare`, the experiment tracker can annotate each run with "this was a 45-minute session using Granite 3.3 8B with 127 tool calls" — context that raw MLflow metrics don't capture.

**Also:** Add a session-start hook that checks MLflow for the last session's experiment and injects a one-liner: `<last-session>model=granite-3.3-8b tasks=3-completed accuracy=87% tool-calls=127</last-session>`. The LLM starts each session knowing how the last one went.

---

### Plugin 3 Enhancement: OpenShift Cluster Ops — Add Observability Shortcuts

**Current spec:** Tools for `oc` CLI operations.

**Enhancement:** Add high-level observability shortcuts that wrap common PromQL/LogQL patterns without requiring the full observability plugin (Plugin 21).

**New tools:**

| Tool | Description |
|------|-------------|
| `ocp_top_pods` | Get pod CPU and memory usage (top consumers) in a namespace. Wraps `oc adm top pods` with structured output. |
| `ocp_resource_usage` | Get namespace-level resource utilization vs. requests vs. limits. Answers "is this namespace over/under-provisioned?" |
| `ocp_error_rate` | Quick error rate check for a service (wraps a standard PromQL query for 5xx rate). Returns current rate, 1h trend, and whether it's above the alerting threshold. |

**Why shortcuts instead of full Plugin 21:** Not every user needs the full observability surface. These 3 tools cover the most common debugging questions without requiring Prometheus/Loki auth setup. Plugin 21 is for users who need PromQL/LogQL/Tempo queries directly.

---

## Part 5: Updated Priority List (All 24 Plugins)

### Tier 1 — Build First (highest daily value, lowest friction)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 1 | **Cluster Context Injection** (11) + Cost + GitOps + Alert enhancements | Lifecycle | ~150 lines with all enhancements. Immediate value. |
| 2 | **Red Hat Developer Content Search** (14) | Tool | Local file search over 4,360 articles. No auth. Directly improves code accuracy. |
| 3 | **Containerfile/Image Mode Linter** (17) | Tool | Catches the most common local-model mistakes. Auto-lint on edit. |
| 4 | **Observability Stack** (21) | Tool | Prometheus, Loki, Tempo, AlertManager. THE debugging surface. Start with `obs_promql` + `obs_alerts` + `obs_logs`. |

### Tier 2 — Core Infrastructure (enables everything else)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 5 | **OpenShift OAuth Auth** (13) | Auth | Shared auth. Build before or alongside OCP/RHOAI plugins. |
| 6 | **RHOAI Model Serving** (1) + Sandbox enhancement | Provider | Headline feature. Sandbox path removes "need a cluster" barrier. |
| 7 | **AAP MCP Bridge** (5) + Playbook linting | Tool | AAP already did the MCP work. Linting catches bad playbook generation. |
| 8 | **RHOAI MCP Bridge** (24) | Tool | Same pattern as AAP — RHOAI 3.5 ships an MCP server. Could be <100 lines. |

### Tier 3 — Fleet & Cluster Operations (the workhorse)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 9 | **RHACM Multi-Cluster Management** (20) | Tool | Fleet-level visibility. Multi-cluster PromQL via `acm_observability`. Governance/policy compliance. |
| 10 | **OpenShift Cluster Ops** (3) + GitOps + Insights + Observability shortcuts | Tool | Build incrementally: read-only → GitOps awareness → Insights → obs shortcuts. |
| 11 | **RHACS Scanner** (4) + Compliance | Tool | Security scanning + compliance profiles in the dev loop. |
| 12 | **Tekton Pipeline Runner** (9) | Tool | CI feedback loop. Pairs with OCP plugin. |

### Tier 4 — AI/ML Lifecycle (the RHOAI story)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 13 | **MLFlow Tools** (22) | Tool | Read-side: compare runs, query model registry, promote models. Complements Plugin 10 (write-side). |
| 14 | **RHOAI Platform Tools** (23) | Tool | DSP, EvalHub, TrustyAI, workbenches. Build incrementally — read-only first. |
| 15 | **RHOAI Experiment Tracker** (10) + Read-side enhancement | Lifecycle | Already built. Add session context injection. |

### Tier 5 — Differentiation (novel, no one else has these)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 16 | **Lightwell Dependency Checker** (7) + Containerfile scanning | Tool | Supply chain patches at dev time. Dockerfile parsing is the enhancement. |
| 17 | **Ecosystem Catalog Search** (18) | Tool | Local search, no auth. Eliminates partner/operator hallucination. |
| 18 | **Red Hat API Catalog** (15) | Tool | OpenAPI specs for 50 console.redhat.com APIs. Correct client code generation. |

### Tier 6 — Specialized (valuable for specific workflows)

| Rank | Plugin | Type | Notes |
|------|--------|------|-------|
| 19 | **EDA Session Events** (12) | Lifecycle | Coding-to-operations bridge. Requires EDA infrastructure. |
| 20 | **RHDP Demo Provisioner** (16) | Tool | "I need a cluster to test against" — removes the blocker. |
| 21 | **RHDH Catalog Query** (8) | Tool | Service catalog lookups. Large org value only. |
| 22 | **Quay Registry Tools** (6) | Tool | Image registry. Pairs with RHACS. |
| 23 | **Red Hat Learning Path Guide** (19) | Tool | Learning-mode detection. Niche but good for adoption. |
| 24 | **Satellite Lightspeed Provider** (2) | Provider | Infra-as-code niche. |

### What moved and why

**Observability Stack (21) jumped to Tier 1** because it's the debugging surface. Without it, the LLM is blind to metrics, logs, traces, and alerts — it can see pod status but not why a pod is failing. Every SRE workflow starts with "check Prometheus, check logs, find the trace." The plugin gives the LLM that same workflow.

**RHACM (20) is Tier 3** — not Tier 1 because it requires multi-cluster setups (not every user has ACM), but it's the highest-value tool for enterprise users managing fleets. The federated PromQL alone (`acm_observability`) is worth the plugin.

**RHOAI MCP Bridge (24) jumped to Tier 2** because RHOAI already built the MCP server. The plugin is just a bridge — same pattern as the AAP MCP bridge, possibly <100 lines. Gets broad RHOAI coverage instantly.

**MLFlow Tools (22) and RHOAI Platform Tools (23) form a new Tier 4** — the AI/ML lifecycle tier. These complete the RHOAI story from "serve a model" (Plugin 1) to "the full MLOps loop."

### Updated Bundling Strategy (all 24 plugins)

- **`@tinycode/plugin-openshift`** — Plugins 3, 11, 13, 21 (Cluster Ops + Context + OAuth + Observability + GitOps + Insights)
- **`@tinycode/plugin-fleet`** — Plugin 20 (RHACM multi-cluster management). Separate from openshift because not all users have ACM.
- **`@tinycode/plugin-rhoai`** — Plugins 1, 10, 22, 23, 24 (Model Serving + Experiment Tracker + MLFlow + Platform Tools + MCP Bridge). The full RHOAI experience.
- **`@tinycode/plugin-security`** — Plugins 4, 7, 17 (RHACS + Lightwell + Container Linter)
- **`@tinycode/plugin-automation`** — Plugins 5, 12 (AAP Bridge + EDA Events + Playbook Linting)
- **`@tinycode/plugin-devex`** — Plugins 6, 8, 9 (Quay + RHDH + Tekton)
- **`@tinycode/plugin-reference`** — Plugins 14, 15, 18, 19 (Developer Content + API Catalog + Ecosystem + Learning Paths). Local-first, no auth needed.
