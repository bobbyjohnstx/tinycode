package plugin

// RegistryEntry describes a plugin available in the curated registry.
type RegistryEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Package     string `json:"package,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Binary      string `json:"binary,omitempty"`
	Category    string `json:"category,omitempty"`
}

// Plugin categories.
const (
	CategorySRE       = "sre"
	CategorySecurity  = "security"
	CategoryAIML      = "ai-ml"
	CategoryPlatform  = "platform"
	CategoryDeveloper = "developer"
	CategoryEssential = "essential"
)

// Categories returns the list of valid category slugs with display labels.
func Categories() []CategoryInfo {
	return []CategoryInfo{
		{Slug: CategorySRE, Label: "OpenShift SRE / Platform Admin"},
		{Slug: CategorySecurity, Label: "Security / Compliance"},
		{Slug: CategoryAIML, Label: "AI/ML / Data Science"},
		{Slug: CategoryPlatform, Label: "Platform / Infrastructure"},
		{Slug: CategoryDeveloper, Label: "Developer"},
		{Slug: CategoryEssential, Label: "Essential"},
	}
}

// CategoryInfo pairs a slug with a human-readable label.
type CategoryInfo struct {
	Slug  string
	Label string
}

// registry is the hardcoded list of known plugins.
var registry = []RegistryEntry{
	// platform
	{Name: "aap-bridge", Description: "Ansible Automation Platform bridge (job templates, inventories, lint)", Binary: "tinycode-plugin-aap-bridge", Category: CategoryPlatform},
	// sre
	{Name: "audit-logs", Description: "API audit log analysis (top callers, search, timeline, anomaly detection)", Binary: "tinycode-plugin-audit-logs", Category: CategorySRE},
	// security
	{Name: "container-linter", Description: "Containerfile linting, bootc validation, UBI base image suggestions", Binary: "tinycode-plugin-container-linter", Category: CategorySecurity},
	// sre
	{Name: "etcd-diag", Description: "etcd performance diagnostics (slow writes, leader elections, cross-pod comparison)", Binary: "tinycode-plugin-etcd-diag", Category: CategorySRE},
	{Name: "ingress-inspect", Description: "HAProxy/Ingress inspection (controllers, backends, route validation, misconfiguration detection)", Binary: "tinycode-plugin-ingress-inspect", Category: CategorySRE},
	{Name: "insights", Description: "OpenShift Insights archive analysis (nodes, operators, memory, etcd, storage, alerts)", Binary: "tinycode-plugin-insights", Category: CategorySRE},
	// security
	{Name: "lightwell", Description: "Red Hat Lightwell package security (CVE checks, provenance, Containerfile scanning)", Binary: "tinycode-plugin-lightwell", Category: CategorySecurity},
	{Name: "log-sanitizer", Description: "Sanitize sensitive data from logs", Binary: "tinycode-plugin-log-sanitizer", Category: CategorySecurity},
	// sre
	{Name: "ocp-context-injection", Description: "OpenShift cluster context injection (version, nodes, operators, alerts, cost)", Binary: "tinycode-plugin-ocp-context-injection", Category: CategorySRE},
	{Name: "ocp-odf", Description: "OpenShift Data Foundation storage health, Ceph status, pools, PVCs, buckets", Binary: "tinycode-plugin-ocp-odf", Category: CategorySRE},
	{Name: "ocp-must-gather", Description: "Must-gather offline analysis (cluster version, nodes, operators, certs, etcd, pods, events)", Binary: "tinycode-plugin-ocp-must-gather", Category: CategorySRE},
	{Name: "ocp-obs-logging", Description: "OpenShift observability logging (Loki, Tempo, NetObserv)", Binary: "tinycode-plugin-ocp-obs-logging", Category: CategorySRE},
	{Name: "ocp-virt", Description: "OpenShift Virtualization VM lifecycle, migration, DataVolumes, templates", Binary: "tinycode-plugin-ocp-virt", Category: CategorySRE},
	{Name: "ocp-obs-metrics", Description: "OpenShift observability metrics (PromQL, alerts, silencing)", Binary: "tinycode-plugin-ocp-obs-metrics", Category: CategorySRE},
	// essential
	{Name: "pilot", Description: "Autonomous agent pilot mode", Binary: "tinycode-plugin-pilot", Category: CategoryEssential},
	// developer
	{Name: "quay", Description: "Quay container registry (search, tags, manifests, vulnerabilities)", Binary: "tinycode-plugin-quay", Category: CategoryDeveloper},
	{Name: "rh-api-catalog", Description: "Red Hat API catalog (list, spec, endpoints)", Binary: "tinycode-plugin-rh-api-catalog", Category: CategoryDeveloper},
	{Name: "rh-dev-content", Description: "Red Hat developer content (search, articles, recent posts)", Binary: "tinycode-plugin-rh-dev-content", Category: CategoryDeveloper},
	{Name: "rh-ecosystem-catalog", Description: "Red Hat ecosystem catalog (containers, operators via Pyxis)", Binary: "tinycode-plugin-rh-ecosystem-catalog", Category: CategoryDeveloper},
	// platform
	{Name: "rhacm", Description: "Red Hat ACM fleet management (clusters, policies, applications, observability)", Binary: "tinycode-plugin-rhacm", Category: CategoryPlatform},
	// security
	{Name: "rhacs", Description: "Red Hat ACS security (image scan, policy check, violations, compliance)", Binary: "tinycode-plugin-rhacs", Category: CategorySecurity},
	// developer
	{Name: "rhdh", Description: "Red Hat Developer Hub (catalog, APIs, TechDocs, dependencies)", Binary: "tinycode-plugin-rhdh", Category: CategoryDeveloper},
	// platform
	{Name: "rhdp-provisioner", Description: "Red Hat Developer Platform provisioner (search, provision, status)", Binary: "tinycode-plugin-rhdp-provisioner", Category: CategoryPlatform},
	// ai-ml
	{Name: "rhoai-mlflow", Description: "MLflow experiment tracking, model registry, and session metrics", Binary: "tinycode-plugin-rhoai-mlflow", Category: CategoryAIML},
	{Name: "rhoai-pipelines", Description: "RHOAI data science pipelines (list, run, status, create)", Binary: "tinycode-plugin-rhoai-pipelines", Category: CategoryAIML},
	{Name: "rhoai-serving", Description: "RHOAI model serving, evaluation, TrustyAI fairness, workbenches, and Developer Sandbox", Binary: "tinycode-plugin-rhoai-serving", Category: CategoryAIML},
	// security
	{Name: "safety-net", Description: "Pre-execution safety checks for destructive commands", Binary: "tinycode-plugin-safety-net", Category: CategorySecurity},
	// platform
	{Name: "satellite", Description: "Red Hat Satellite (hosts, errata, content views, services, proxies, REX)", Binary: "tinycode-plugin-satellite", Category: CategoryPlatform},
	{Name: "tekton", Description: "Tekton pipelines (list, runs, status, logs, tasks)", Binary: "tinycode-plugin-tekton", Category: CategoryPlatform},
	// essential
	{Name: "telemetry", Description: "Usage telemetry and analytics", Binary: "tinycode-plugin-telemetry", Category: CategoryEssential},
}

// Registry returns the full list of known plugins in the curated registry.
func Registry() []RegistryEntry {
	out := make([]RegistryEntry, len(registry))
	copy(out, registry)
	return out
}

// RegistryByCategory returns registry entries matching the given category slug.
func RegistryByCategory(cat string) []RegistryEntry {
	var out []RegistryEntry
	for _, e := range registry {
		if e.Category == cat {
			out = append(out, e)
		}
	}
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
