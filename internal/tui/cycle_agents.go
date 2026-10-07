package tui

import "github.com/bobbyjohnstx/tinycode/internal/tui/api"

// DefaultCycleAgents is the Tab/Shift-Tab persona list when config.cycle_agents is unset.
var DefaultCycleAgents = []string{"build", "plan", "architect", "code-reviewer"}

// resolveCycleAgents returns the Tab cycle list: preferred (or defaults),
// filtered to agents that are currently enabled.
func resolveCycleAgents(preferred []string, enabled []api.AgentInfo) []string {
	enabledSet := make(map[string]bool, len(enabled))
	for _, ag := range enabled {
		enabledSet[ag.Name] = true
	}
	src := preferred
	if len(src) == 0 {
		src = DefaultCycleAgents
	}
	out := make([]string, 0, len(src))
	for _, name := range src {
		if enabledSet[name] {
			out = append(out, name)
		}
	}
	return out
}
