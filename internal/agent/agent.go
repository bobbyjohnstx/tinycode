package agent

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
)

//go:embed defaults/*.md defaults/*.txt
var defaultsFS embed.FS

type Mode string

const (
	ModePrimary  Mode = "primary"
	ModeSubagent Mode = "subagent"
	ModeAll      Mode = "all"
)

type Info struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Mode        Mode                 `json:"mode"`
	Native      bool                 `json:"native,omitempty"`
	Hidden      bool                 `json:"hidden,omitempty"`
	TopP        *float64             `json:"topP,omitempty"`
	Temperature *float64             `json:"temperature,omitempty"`
	Color       string               `json:"color,omitempty"`
	Permission  permission.Ruleset   `json:"permission"`
	Prompt      string               `json:"prompt,omitempty"`
	Compact     bool                 `json:"compact,omitempty"`
	Steps       *int                 `json:"steps,omitempty"`
	Options     map[string]any       `json:"options"`
	Model       *ModelRef            `json:"model,omitempty"`
	Variant     string               `json:"variant,omitempty"`
}

type ModelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type ConfigOverride struct {
	Model       string             `json:"model,omitempty"`
	Variant     string             `json:"variant,omitempty"`
	Prompt      string             `json:"prompt,omitempty"`
	Description string             `json:"description,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Mode        Mode               `json:"mode,omitempty"`
	Color       string             `json:"color,omitempty"`
	Hidden      *bool              `json:"hidden,omitempty"`
	Name        string             `json:"name,omitempty"`
	Steps       *int               `json:"steps,omitempty"`
	Disable     bool               `json:"disable,omitempty"`
	Options     map[string]any     `json:"options,omitempty"`
	Permission  map[string]any     `json:"permission,omitempty"`
}

type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Info
}

func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*Info),
	}
}

// LoadDefaults loads all bundled agent definitions from the embedded filesystem.
// Native agents (build, plan, general, explore, scout, compaction, title, summary)
// are registered first with hardcoded permissions, then bundled .md agents are loaded.
func (r *Registry) LoadDefaults(defaultPerms, userPerms permission.Ruleset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.registerNativeAgents(defaultPerms, userPerms)

	entries, err := defaultsFS.ReadDir("defaults")
	if err != nil {
		return fmt.Errorf("reading embedded agent defaults: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		agentName := strings.TrimSuffix(name, ".md")
		if r.agents[agentName] != nil {
			continue
		}

		data, err := defaultsFS.ReadFile(filepath.Join("defaults", name))
		if err != nil {
			continue
		}

		frontmatter, body := parseFrontmatter(string(data))
		info := &Info{
			Name:    agentName,
			Mode:    ModeAll,
			Options: make(map[string]any),
			Native:  false,
			Prompt:  strings.TrimSpace(body),
		}

		if desc, ok := frontmatter["description"].(string); ok {
			info.Description = desc
		}
		if mode, ok := frontmatter["mode"].(string); ok {
			info.Mode = Mode(mode)
		}
		if hidden, ok := frontmatter["hidden"].(bool); ok {
			info.Hidden = hidden
		}
		if color, ok := frontmatter["color"].(string); ok {
			info.Color = color
		}
		if steps, ok := frontmatter["steps"].(int); ok {
			info.Steps = &steps
		}

		agentPerm := extractPermissionRules(frontmatter)
		info.Permission = permission.Merge(defaultPerms, agentPerm, userPerms)

		r.agents[agentName] = info
	}

	return nil
}

// ApplyConfigOverrides applies user config overrides to the agent registry.
func (r *Registry) ApplyConfigOverrides(overrides map[string]ConfigOverride, defaultPerms, userPerms permission.Ruleset) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, override := range overrides {
		if override.Disable {
			delete(r.agents, key)
			continue
		}

		agent, exists := r.agents[key]
		if !exists {
			agent = &Info{
				Name:       key,
				Mode:       ModeAll,
				Permission: permission.Merge(defaultPerms, userPerms),
				Options:    make(map[string]any),
				Native:     false,
			}
			r.agents[key] = agent
		}

		if override.Model != "" {
			agent.Model = ParseModel(override.Model)
		}
		if override.Variant != "" {
			agent.Variant = override.Variant
		}
		if override.Prompt != "" {
			agent.Prompt = override.Prompt
		}
		if override.Description != "" {
			agent.Description = override.Description
		}
		if override.Temperature != nil {
			agent.Temperature = override.Temperature
		}
		if override.TopP != nil {
			agent.TopP = override.TopP
		}
		if override.Mode != "" {
			agent.Mode = override.Mode
		}
		if override.Color != "" {
			agent.Color = override.Color
		}
		if override.Hidden != nil {
			agent.Hidden = *override.Hidden
		}
		if override.Name != "" {
			agent.Name = override.Name
		}
		if override.Steps != nil {
			agent.Steps = override.Steps
		}
		if override.Options != nil {
			for k, v := range override.Options {
				agent.Options[k] = v
			}
		}
		if override.Permission != nil {
			configPerm := permissionFromConfigMap(override.Permission)
			agent.Permission = permission.Merge(agent.Permission, configPerm)
		}
	}
}

// Get retrieves an agent by name. If modelSizeB is provided and <= 8,
// the compact variant is returned if available.
func (r *Registry) Get(name string, modelSizeB *float64) *Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if modelSizeB != nil && *modelSizeB <= 8 {
		compact := r.agents[name+".compact"]
		if compact != nil {
			result := *compact
			result.Compact = true
			return &result
		}
	}

	agent := r.agents[name]
	if agent == nil {
		return nil
	}
	result := *agent
	result.Compact = false
	return &result
}

// List returns all agents sorted by name, excluding compact variants.
// If defaultAgent is set, it sorts first.
func (r *Registry) List(defaultAgent string) []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Info
	for _, agent := range r.agents {
		if strings.Contains(agent.Name, ".compact") {
			continue
		}
		result = append(result, *agent)
	}

	sort.Slice(result, func(i, j int) bool {
		iDefault := result[i].Name == defaultAgent
		jDefault := result[j].Name == defaultAgent
		if iDefault != jDefault {
			return iDefault
		}
		return result[i].Name < result[j].Name
	})

	return result
}

// DefaultAgent returns the default agent name from config, or "build".
func (r *Registry) DefaultAgent(configDefault string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if configDefault != "" {
		agent := r.agents[configDefault]
		if agent == nil {
			return "", fmt.Errorf("default agent %q not found", configDefault)
		}
		if agent.Mode == ModeSubagent {
			return "", fmt.Errorf("default agent %q is a subagent", configDefault)
		}
		if agent.Hidden {
			return "", fmt.Errorf("default agent %q is hidden", configDefault)
		}
		return configDefault, nil
	}

	if agent := r.agents["build"]; agent != nil {
		return "build", nil
	}
	for _, agent := range r.agents {
		if agent.Mode != ModeSubagent && !agent.Hidden {
			return agent.Name, nil
		}
	}
	return "", fmt.Errorf("no primary visible agent found")
}

func (r *Registry) registerNativeAgents(defaultPerms, userPerms permission.Ruleset) {
	buildPrompt := readEmbeddedTxt("build.txt")
	compactionPrompt := readEmbeddedTxt("compaction.txt")
	generalPrompt := readEmbeddedTxt("general.txt")
	explorePrompt := readEmbeddedTxt("explore.txt")
	scoutPrompt := readEmbeddedTxt("scout.txt")
	titlePrompt := readEmbeddedTxt("title.txt")
	summaryPrompt := readEmbeddedTxt("summary.txt")

	denyAll := permission.Ruleset{{Permission: "*", Pattern: "*", Action: permission.ActionDeny}}
	temp05 := 0.5

	r.agents["build"] = &Info{
		Name:        "build",
		Description: "The default agent. Executes tools based on configured permissions.",
		Color:       "#ff0000",
		Prompt:      buildPrompt,
		Permission: permission.Merge(
			defaultPerms,
			permission.Ruleset{
				{Permission: "question", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "plan_enter", Pattern: "*", Action: permission.ActionAllow},
			},
			userPerms,
		),
		Mode:    ModePrimary,
		Native:  true,
		Options: make(map[string]any),
	}

	r.agents["plan"] = &Info{
		Name:        "plan",
		Description: "Plan mode. Disallows all edit tools.",
		Prompt:      buildPrompt,
		Permission: permission.Merge(
			defaultPerms,
			permission.Ruleset{
				{Permission: "question", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "plan_exit", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "edit", Pattern: "*", Action: permission.ActionDeny},
			},
			userPerms,
		),
		Mode:    ModePrimary,
		Native:  true,
		Options: make(map[string]any),
	}

	r.agents["general"] = &Info{
		Name:        "general",
		Description: "General-purpose agent for researching complex questions and executing multi-step tasks.",
		Prompt:      generalPrompt,
		Permission: permission.Merge(
			defaultPerms,
			permission.Ruleset{
				{Permission: "todowrite", Pattern: "*", Action: permission.ActionDeny},
			},
			userPerms,
		),
		Mode:    ModeSubagent,
		Native:  true,
		Options: make(map[string]any),
	}

	r.agents["explore"] = &Info{
		Name:        "explore",
		Description: "Fast agent specialized for exploring codebases.",
		Prompt:      explorePrompt,
		Permission: permission.Merge(
			defaultPerms,
			permission.Ruleset{
				{Permission: "*", Pattern: "*", Action: permission.ActionDeny},
				{Permission: "grep", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "glob", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "list", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "bash", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "webfetch", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "websearch", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
			},
			userPerms,
		),
		Mode:    ModeSubagent,
		Native:  true,
		Options: make(map[string]any),
	}

	r.agents["scout"] = &Info{
		Name:        "scout",
		Description: "External research specialist. Clones and inspects dependency repos, fetches docs.",
		Prompt:      scoutPrompt,
		Permission: permission.Merge(
			defaultPerms,
			permission.Ruleset{
				{Permission: "*", Pattern: "*", Action: permission.ActionDeny},
				{Permission: "grep", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "glob", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "webfetch", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "websearch", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "repo_clone", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "repo_overview", Pattern: "*", Action: permission.ActionAllow},
			},
			userPerms,
		),
		Mode:    ModeSubagent,
		Native:  true,
		Options: make(map[string]any),
	}

	r.agents["compaction"] = &Info{
		Name:       "compaction",
		Mode:       ModePrimary,
		Native:     true,
		Hidden:     true,
		Prompt:     compactionPrompt,
		Permission: permission.Merge(defaultPerms, denyAll, userPerms),
		Options:    make(map[string]any),
	}

	r.agents["title"] = &Info{
		Name:        "title",
		Mode:        ModePrimary,
		Native:      true,
		Hidden:      true,
		Temperature: &temp05,
		Prompt:      titlePrompt,
		Permission:  permission.Merge(defaultPerms, denyAll, userPerms),
		Options:     make(map[string]any),
	}

	r.agents["summary"] = &Info{
		Name:       "summary",
		Mode:       ModePrimary,
		Native:     true,
		Hidden:     true,
		Prompt:     summaryPrompt,
		Permission: permission.Merge(defaultPerms, denyAll, userPerms),
		Options:    make(map[string]any),
	}
}

func readEmbeddedTxt(name string) string {
	data, err := defaultsFS.ReadFile(filepath.Join("defaults", name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ParseModel splits "provider/model" into a ModelRef.
func ParseModel(s string) *ModelRef {
	idx := strings.IndexByte(s, '/')
	if idx < 0 {
		return &ModelRef{ModelID: s}
	}
	return &ModelRef{
		ProviderID: s[:idx],
		ModelID:    s[idx+1:],
	}
}

// parseFrontmatter extracts YAML-like frontmatter from markdown content.
// Returns a simple key-value map and the body after the frontmatter.
func parseFrontmatter(content string) (map[string]any, string) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content
	}

	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return nil, content
	}

	fm := content[4 : 4+end]
	body := content[4+end+4:]

	result := make(map[string]any)
	var currentMap map[string]any
	var currentKey string

	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		if indent > 0 && currentMap != nil {
			k, v, ok := splitFMLine(trimmed)
			if ok {
				currentMap[stripQuotes(k)] = v
			}
			continue
		}

		k, v, ok := splitFMLine(trimmed)
		if !ok {
			continue
		}

		key := stripQuotes(k)
		value := v

		if value == "" {
			currentMap = make(map[string]any)
			currentKey = key
			result[currentKey] = currentMap
			continue
		}

		currentMap = nil
		currentKey = ""

		switch value {
		case "true":
			result[key] = true
		case "false":
			result[key] = false
		default:
			if n, ok := parseInt(value); ok {
				result[key] = n
			} else {
				result[key] = value
			}
		}
	}

	return result, body
}

func splitFMLine(s string) (key, value string, ok bool) {
	// Handle quoted keys like "\"*\": deny"
	if strings.HasPrefix(s, "\"") {
		end := strings.Index(s[1:], "\"")
		if end < 0 {
			return "", "", false
		}
		key = s[1 : 1+end]
		rest := s[1+end+1:]
		if !strings.HasPrefix(rest, ":") {
			return "", "", false
		}
		value = strings.TrimSpace(rest[1:])
		return key, value, true
	}

	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseInt(s string) (int, bool) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if len(s) == 0 {
		return 0, false
	}
	return n, true
}

func extractPermissionRules(fm map[string]any) permission.Ruleset {
	permMap, ok := fm["permission"].(map[string]any)
	if !ok {
		return nil
	}
	return permissionFromConfigMap(permMap)
}

func permissionFromConfigMap(m map[string]any) permission.Ruleset {
	var rules permission.Ruleset
	for key, val := range m {
		switch v := val.(type) {
		case string:
			rules = append(rules, permission.Rule{
				Permission: key,
				Pattern:    "*",
				Action:     permission.Action(v),
			})
		case map[string]any:
			for pattern, action := range v {
				if actionStr, ok := action.(string); ok {
					rules = append(rules, permission.Rule{
						Permission: key,
						Pattern:    pattern,
						Action:     permission.Action(actionStr),
					})
				}
			}
		}
	}
	return rules
}
