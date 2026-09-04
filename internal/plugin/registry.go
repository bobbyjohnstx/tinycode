package plugin

// RegistryEntry describes a plugin available in the curated registry.
type RegistryEntry struct {
	Name        string
	Description string
	Repo        string
	Binary      string
}

// registry is the hardcoded list of known plugins.
var registry = []RegistryEntry{
	{Name: "notify", Description: "Desktop notifications for session events", Repo: "github.com/bobbyjohnstx/tinycode-plugin-notify", Binary: "tinycode-plugin-notify"},
	{Name: "safety-net", Description: "Pre-execution safety checks for destructive commands", Repo: "github.com/bobbyjohnstx/tinycode-plugin-safety-net", Binary: "tinycode-plugin-safety-net"},
	{Name: "code-review", Description: "Automated code review on session end", Repo: "github.com/bobbyjohnstx/tinycode-plugin-code-review", Binary: "tinycode-plugin-code-review"},
	{Name: "handoff", Description: "Session handoff between agents", Repo: "github.com/bobbyjohnstx/tinycode-plugin-handoff", Binary: "tinycode-plugin-handoff"},
	{Name: "telemetry", Description: "Usage telemetry and analytics", Repo: "github.com/bobbyjohnstx/tinycode-plugin-telemetry", Binary: "tinycode-plugin-telemetry"},
	{Name: "web-search", Description: "Web search tool for agents", Repo: "github.com/bobbyjohnstx/tinycode-plugin-web-search", Binary: "tinycode-plugin-web-search"},
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
