package agent

import (
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

//go:embed defaults/*.md defaults/*.txt
var defaultsFS embed.FS

// DefaultPerms is the shared seed merged ahead of each agent's own rules.
// It is empty on purpose. A catch-all allow is evaluated after DefaultRules
// and would approve .env reads, webfetch, external directories, and
// destructive shell for every agent that does not override it.
var DefaultPerms permission.Ruleset

// buildPerms allows ordinary work for the default agent. The ask rules come
// after those allows so last-match prompts for .env reads, webfetch,
// directories outside the project, and destructive shell. User rules are
// merged after this set and can still override an ask.
//
// read covers grep, glob, question, skill, and websearch, which use the read
// permission. edit covers write, apply_patch, and todowrite. shell covers
// bash and monitor. task, diagnostics, notepad, and report_findings have no
// permission of their own; naming them here keeps them in the tool list.
var buildPerms = permission.Ruleset{
	{Permission: "question", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "plan_enter", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "grep", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "glob", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "edit", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "shell", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "task", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "diagnostics", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "notepad", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "report_findings", Pattern: "*", Action: permission.ActionAllow},
	{Permission: "read", Pattern: ".env*", Action: permission.ActionAsk},
	{Permission: "webfetch", Pattern: "*", Action: permission.ActionAsk},
	{Permission: "external_directory", Pattern: "*", Action: permission.ActionAsk},
	{Permission: "destructive-shell", Pattern: "*", Action: permission.ActionAsk},
}

type Mode string

const (
	ModePrimary  Mode = "primary"
	ModeSubagent Mode = "subagent"
	ModeAll      Mode = "all"
)

type Info struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Mode        Mode               `json:"mode"`
	Native      bool               `json:"native,omitempty"`
	Hidden      bool               `json:"hidden,omitempty"`
	Disabled    bool               `json:"disabled,omitempty"`
	TopP        *float64           `json:"topP,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	Color       string             `json:"color,omitempty"`
	Permission  permission.Ruleset `json:"permission"`
	Prompt      string             `json:"prompt,omitempty"`
	Compact     bool               `json:"compact,omitempty"`
	Steps       *int               `json:"steps,omitempty"`
	Options     map[string]any     `json:"options"`
	Model       *session.ModelRef  `json:"model,omitempty"`
	Variant     string             `json:"variant,omitempty"`
}

type ConfigOverride struct {
	Model       string         `json:"model,omitempty"`
	Variant     string         `json:"variant,omitempty"`
	Prompt      string         `json:"prompt,omitempty"`
	Description string         `json:"description,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	Mode        Mode           `json:"mode,omitempty"`
	Color       string         `json:"color,omitempty"`
	Hidden      *bool          `json:"hidden,omitempty"`
	Name        string         `json:"name,omitempty"`
	Steps       *int           `json:"steps,omitempty"`
	Disable     bool           `json:"disable,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	Permission  map[string]any `json:"permission,omitempty"`
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
			Mode:    ModeSubagent,
			Options: make(map[string]any),
			Native:  false,
			Prompt:  strings.TrimSpace(body),
		}
		applyFrontmatter(info, frontmatter)

		agentPerm := extractPermissionRules(frontmatter)
		info.Permission = permission.Merge(defaultPerms, agentPerm, userPerms)

		r.agents[agentName] = info
	}

	// Default-disable archived agents (and their compact peers).
	for _, name := range []string{"code-simplifier", "qa-tester", "scientist"} {
		if agent := r.agents[name]; agent != nil {
			agent.Disabled = true
		}
		if agent := r.agents[name+".compact"]; agent != nil {
			agent.Disabled = true
		}
	}

	return nil
}

// ApplyConfigOverrides applies user config overrides to the agent registry.
func (r *Registry) ApplyConfigOverrides(overrides map[string]ConfigOverride, defaultPerms, userPerms permission.Ruleset) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, override := range overrides {
		if override.Disable {
			agent := r.agents[key]
			if agent != nil && agent.Native {
				slog.Warn("cannot disable native agent", "agent", key)
				continue
			}
			if agent != nil {
				agent.Disabled = true
			}
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
		agent.Disabled = false

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
// the compact variant is returned if available. Disabled agents are never served.
func (r *Registry) Get(name string, modelSizeB *float64) *Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if modelSizeB != nil && *modelSizeB <= 8 {
		compact := r.agents[name+".compact"]
		if compact != nil && !compact.Disabled {
			result := *compact
			result.Name = name
			result.Compact = true
			return &result
		}
	}

	agent := r.agents[name]
	if agent == nil || agent.Disabled {
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
		if agent.Hidden {
			continue
		}
		if agent.Disabled {
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

// ListAll returns all agents including disabled ones, sorted by name.
// Compact and hidden variants are still excluded.
func (r *Registry) ListAll(defaultAgent string) []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Info
	for _, agent := range r.agents {
		if strings.Contains(agent.Name, ".compact") {
			continue
		}
		if agent.Hidden {
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
		// Sort disabled agents to the bottom.
		if result[i].Disabled != result[j].Disabled {
			return !result[i].Disabled
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
		if agent.Disabled {
			return "", fmt.Errorf("default agent %q is disabled", configDefault)
		}
		return configDefault, nil
	}

	if agent := r.agents["build"]; agent != nil && !agent.Disabled {
		return "build", nil
	}
	for _, agent := range r.agents {
		if agent.Mode != ModeSubagent && !agent.Hidden && !agent.Disabled {
			return agent.Name, nil
		}
	}
	return "", fmt.Errorf("no primary visible agent found")
}

func (r *Registry) registerNativeAgents(defaultPerms, userPerms permission.Ruleset) {
	buildPrompt := readEmbeddedTxt("build.txt")
	planPrompt := readEmbeddedTxt("plan.txt")
	generalPrompt := readEmbeddedTxt("general.txt")
	explorePrompt := readEmbeddedExplorePrompt()
	scoutPrompt := readEmbeddedTxt("scout.txt")
	compactionPrompt := readEmbeddedTxt("compaction.txt")
	titlePrompt := readEmbeddedTxt("title.txt")
	summaryPrompt := readEmbeddedTxt("summary.txt")

	r.registerPrimaryAgents(buildPrompt, planPrompt, defaultPerms, userPerms)
	r.registerSubagents(generalPrompt, explorePrompt, scoutPrompt, defaultPerms, userPerms)
	r.registerUtilityAgents(compactionPrompt, titlePrompt, summaryPrompt, defaultPerms, userPerms)
}

func (r *Registry) registerPrimaryAgents(buildPrompt, planPrompt string, defaultPerms, userPerms permission.Ruleset) {
	r.agents["build"] = &Info{
		Name:        "build",
		Description: "The default agent. Executes tools based on configured permissions.",
		Color:       "#ff0000",
		Prompt:      buildPrompt,
		Permission: permission.Merge(
			defaultPerms,
			buildPerms,
			userPerms,
		),
		Mode:    ModePrimary,
		Native:  true,
		Options: make(map[string]any),
	}

	r.agents["plan"] = &Info{
		Name:        "plan",
		Description: "Plan mode. Interviews the user, researches the codebase, and produces work plans; edits restricted to plans/* and drafts/*.",
		Color:       "#8833AA",
		Prompt:      planPrompt,
		Permission: permission.Merge(
			defaultPerms,
			permission.Ruleset{
				{Permission: "question", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
				{Permission: "plan_exit", Pattern: "*", Action: permission.ActionAllow},
				// Catch-all edit deny must precede the scoped allows below so the
				// scoped rules win under Evaluate's last-match-from-end semantics.
				{Permission: "edit", Pattern: "*", Action: permission.ActionDeny},
				{Permission: "edit", Pattern: "plans/*", Action: permission.ActionAllow},
				{Permission: "edit", Pattern: "drafts/*", Action: permission.ActionAllow},
			},
			userPerms,
		),
		Mode:    ModePrimary,
		Native:  true,
		Options: make(map[string]any),
	}
}

func (r *Registry) registerSubagents(generalPrompt, explorePrompt, scoutPrompt string, defaultPerms, userPerms permission.Ruleset) {
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
			},
			userPerms,
		),
		Mode:    ModeSubagent,
		Native:  true,
		Options: make(map[string]any),
	}
}

func (r *Registry) registerUtilityAgents(compactionPrompt, titlePrompt, summaryPrompt string, defaultPerms, userPerms permission.Ruleset) {
	denyAll := permission.Ruleset{{Permission: "*", Pattern: "*", Action: permission.ActionDeny}}
	temp05 := 0.5

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

// readEmbeddedExplorePrompt prefers explore.md body (richer prompt) and falls
// back to explore.txt when the markdown file is missing or empty.
func readEmbeddedExplorePrompt() string {
	data, err := defaultsFS.ReadFile(filepath.Join("defaults", "explore.md"))
	if err == nil {
		_, body := parseFrontmatter(string(data))
		if prompt := strings.TrimSpace(body); prompt != "" {
			return prompt
		}
	}
	return readEmbeddedTxt("explore.txt")
}

// applyFrontmatter copies recognized frontmatter fields onto info.
// Registry map keys stay as the filename stem; Info.Name may be overridden.
func applyFrontmatter(info *Info, frontmatter map[string]any) {
	if info == nil || frontmatter == nil {
		return
	}
	if name, ok := frontmatter["name"].(string); ok && name != "" {
		info.Name = name
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
	if temp, ok := asFloat64(frontmatter["temperature"]); ok {
		info.Temperature = &temp
	}
	if topP, ok := asFloat64(frontmatter["top_p"]); ok {
		info.TopP = &topP
	} else if topP, ok := asFloat64(frontmatter["topP"]); ok {
		info.TopP = &topP
	}
	if model, ok := frontmatter["model"].(string); ok && model != "" {
		info.Model = ParseModel(model)
	}
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// ParseModel splits "provider/model" into a ModelRef.
func ParseModel(s string) *session.ModelRef {
	idx := strings.IndexByte(s, '/')
	if idx < 0 {
		return &session.ModelRef{ModelID: s}
	}
	return &session.ModelRef{
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
	var nestedMap map[string]any

	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		// Nested under a pattern map (e.g. bash: / "*": allow).
		if indent >= 4 && nestedMap != nil {
			k, v, ok := splitFMLine(trimmed)
			if ok {
				nestedMap[stripQuotes(k)] = parseFMValue(v)
			}
			continue
		}

		// Nested under a top-level map (e.g. permission: / bash:).
		if indent > 0 && currentMap != nil {
			k, v, ok := splitFMLine(trimmed)
			if !ok {
				continue
			}
			key := stripQuotes(k)
			if v == "" {
				nestedMap = make(map[string]any)
				currentMap[key] = nestedMap
				continue
			}
			nestedMap = nil
			currentMap[key] = parseFMValue(v)
			continue
		}

		k, v, ok := splitFMLine(trimmed)
		if !ok {
			continue
		}

		key := stripQuotes(k)
		if v == "" {
			currentMap = make(map[string]any)
			nestedMap = nil
			result[key] = currentMap
			continue
		}

		currentMap = nil
		nestedMap = nil
		result[key] = parseFMValue(v)
	}

	return result, body
}

func parseFMValue(value string) any {
	switch value {
	case "true":
		return true
	case "false":
		return false
	default:
		if n, ok := parseInt(value); ok {
			return n
		}
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
		return value
	}
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
	// Emit catch-all ("*") rules first so specific allows win under Evaluate's
	// last-wins semantics. Map iteration order is otherwise non-deterministic.
	var wildcards, specific permission.Ruleset
	for key, val := range m {
		switch v := val.(type) {
		case string:
			perm, pattern := splitPermissionKey(key)
			rule := permission.Rule{
				Permission: perm,
				Pattern:    pattern,
				Action:     permission.Action(v),
			}
			if perm == "*" {
				wildcards = append(wildcards, rule)
			} else {
				specific = append(specific, rule)
			}
		case map[string]any:
			var nestedWild, nestedSpecific permission.Ruleset
			for pattern, action := range v {
				actionStr, ok := action.(string)
				if !ok {
					continue
				}
				rule := permission.Rule{
					Permission: key,
					Pattern:    pattern,
					Action:     permission.Action(actionStr),
				}
				if pattern == "*" {
					nestedWild = append(nestedWild, rule)
				} else {
					nestedSpecific = append(nestedSpecific, rule)
				}
			}
			specific = append(specific, nestedWild...)
			specific = append(specific, nestedSpecific...)
		}
	}
	return append(wildcards, specific...)
}

// splitPermissionKey splits "permission pattern" on the first space.
// When no space is present, the pattern defaults to "*".
func splitPermissionKey(s string) (permission, pattern string) {
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return s, "*"
	}
	return s[:idx], s[idx+1:]
}
