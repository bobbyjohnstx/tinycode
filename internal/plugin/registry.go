package plugin

// RegistryEntry describes a plugin available in the curated registry.
type RegistryEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Package     string `json:"package,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Binary      string `json:"binary,omitempty"`
}

// registry is the hardcoded list of known plugins.
var registry = []RegistryEntry{
	{Name: "aap-bridge", Description: "Ansible Automation Platform bridge (job templates, inventories, lint)", Binary: "tinycode-plugin-aap-bridge"},
	{Name: "cluster-ops", Description: "Kubernetes cluster operations tools", Binary: "tinycode-plugin-cluster-ops"},
	{Name: "code-review", Description: "Automated code review on session end", Binary: "tinycode-plugin-code-review"},
	{Name: "command-inject", Description: "Custom command injection", Binary: "tinycode-plugin-command-inject"},
	{Name: "container-linter", Description: "Containerfile linting, bootc validation, UBI base image suggestions", Binary: "tinycode-plugin-container-linter"},
	{Name: "context-pruning", Description: "Context window pruning strategies", Binary: "tinycode-plugin-context-pruning"},
	{Name: "eda-events", Description: "Event-Driven Ansible event bridge (session lifecycle, tool events)", Binary: "tinycode-plugin-eda-events"},
	{Name: "handoff", Description: "Session handoff between agents", Binary: "tinycode-plugin-handoff"},
	{Name: "lightwell", Description: "Red Hat Lightwell package security (CVE checks, provenance, Containerfile scanning)", Binary: "tinycode-plugin-lightwell"},
	{Name: "log-sanitizer", Description: "Sanitize sensitive data from logs", Binary: "tinycode-plugin-log-sanitizer"},
	{Name: "notify", Description: "Desktop notifications for session events", Binary: "tinycode-plugin-notify"},
	{Name: "ocp-context-injection", Description: "OpenShift cluster context injection (version, nodes, operators, alerts, cost)", Binary: "tinycode-plugin-ocp-context-injection"},
	{Name: "ocp-oauth", Description: "OpenShift OAuth login and shell environment", Binary: "tinycode-plugin-ocp-oauth"},
	{Name: "ocp-obs-logging", Description: "OpenShift observability logging (Loki, Tempo, NetObserv)", Binary: "tinycode-plugin-ocp-obs-logging"},
	{Name: "ocp-obs-metrics", Description: "OpenShift observability metrics (PromQL, alerts, silencing)", Binary: "tinycode-plugin-ocp-obs-metrics"},
	{Name: "pilot", Description: "Autonomous agent pilot mode", Binary: "tinycode-plugin-pilot"},
	{Name: "quay", Description: "Quay container registry (search, tags, manifests, vulnerabilities)", Binary: "tinycode-plugin-quay"},
	{Name: "rh-api-catalog", Description: "Red Hat API catalog (list, spec, endpoints)", Binary: "tinycode-plugin-rh-api-catalog"},
	{Name: "rh-dev-content", Description: "Red Hat developer content (search, articles, recent posts)", Binary: "tinycode-plugin-rh-dev-content"},
	{Name: "rh-ecosystem-catalog", Description: "Red Hat ecosystem catalog (containers, operators via Pyxis)", Binary: "tinycode-plugin-rh-ecosystem-catalog"},
	{Name: "rhacm", Description: "Red Hat ACM fleet management (clusters, policies, applications, observability)", Binary: "tinycode-plugin-rhacm"},
	{Name: "rhacs", Description: "Red Hat ACS security (image scan, policy check, violations, compliance)", Binary: "tinycode-plugin-rhacs"},
	{Name: "rhdh", Description: "Red Hat Developer Hub (catalog, APIs, TechDocs, dependencies)", Binary: "tinycode-plugin-rhdh"},
	{Name: "rhdp-provisioner", Description: "Red Hat Developer Platform provisioner (search, provision, status)", Binary: "tinycode-plugin-rhdp-provisioner"},
	{Name: "rhoai-eval-trustyai", Description: "RHOAI model evaluation and TrustyAI fairness metrics", Binary: "tinycode-plugin-rhoai-eval-trustyai"},
	{Name: "rhoai-experiment-tracker", Description: "RHOAI experiment tracking via MLflow", Binary: "tinycode-plugin-rhoai-experiment-tracker"},
	{Name: "rhoai-mcp-bridge", Description: "RHOAI Model Context Protocol bridge", Binary: "tinycode-plugin-rhoai-mcp-bridge"},
	{Name: "rhoai-mlflow-tools", Description: "MLflow experiment, run, and model registry tools", Binary: "tinycode-plugin-rhoai-mlflow-tools"},
	{Name: "rhoai-model-serving", Description: "RHOAI model serving and sandbox provisioning", Binary: "tinycode-plugin-rhoai-model-serving"},
	{Name: "rhoai-pipelines", Description: "RHOAI data science pipelines (list, run, status, create)", Binary: "tinycode-plugin-rhoai-pipelines"},
	{Name: "safety-net", Description: "Pre-execution safety checks for destructive commands", Binary: "tinycode-plugin-safety-net"},
	{Name: "satellite-lightspeed", Description: "Red Hat Satellite Lightspeed (query, hosts, errata, content views)", Binary: "tinycode-plugin-satellite-lightspeed"},
	{Name: "snippets", Description: "Code snippet management", Binary: "tinycode-plugin-snippets"},
	{Name: "tekton", Description: "Tekton pipelines (list, runs, status, logs, tasks)", Binary: "tinycode-plugin-tekton"},
	{Name: "telemetry", Description: "Usage telemetry and analytics", Binary: "tinycode-plugin-telemetry"},
	{Name: "web-search", Description: "Web search tool for agents", Binary: "tinycode-plugin-web-search"},
}

// Registry returns the full list of known plugins in the curated registry.
func Registry() []RegistryEntry {
	out := make([]RegistryEntry, len(registry))
	copy(out, registry)
	return out
}

// LookupRegistry finds a registry entry by name. Returns the entry and true if
// found, or zero value and false otherwise.
func LookupRegistry(name string) (RegistryEntry, bool) {
	for _, e := range registry {
		if e.Name == name {
			return e, true
		}
	}
	return RegistryEntry{}, false
}
