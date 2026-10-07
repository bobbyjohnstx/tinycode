package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

func defaultPerms() permission.Ruleset {
	return permission.Ruleset{
		{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
	}
}

func TestLoadDefaults_LoadsNativeAgents(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	natives := []string{"build", "plan", "general", "explore", "scout", "compaction", "title", "summary"}
	for _, name := range natives {
		agent := r.Get(name, nil)
		if agent == nil {
			t.Errorf("native agent %q not loaded", name)
			continue
		}
		if !agent.Native {
			t.Errorf("agent %q should be native", name)
		}
	}
}

func TestLoadDefaults_LoadsBundledAgents(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	bundled := []string{"architect", "debugger", "executor", "code-reviewer"}
	for _, name := range bundled {
		agent := r.Get(name, nil)
		if agent == nil {
			t.Errorf("bundled agent %q not loaded", name)
			continue
		}
		if agent.Native {
			t.Errorf("agent %q should not be native", name)
		}
		if agent.Prompt == "" {
			t.Errorf("agent %q has empty prompt", name)
		}
	}
}

func TestLoadDefaults_CompactVariantsLoaded(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	compacts := []string{"architect.compact", "debugger.compact", "explore.compact"}
	for _, name := range compacts {
		agent := r.Get(name, nil)
		if agent == nil {
			t.Errorf("compact agent %q not loaded", name)
		}
	}
}

func TestGet_CompactVariantForSmallModel(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	size := 7.0
	agent := r.Get("architect", &size)
	if agent == nil {
		t.Fatal("expected to get architect agent")
	}
	if !agent.Compact {
		t.Error("expected compact=true for 7B model")
	}
	if agent.Name != "architect" {
		t.Errorf("compact Get should return base name, got %q", agent.Name)
	}
}

func TestGet_FullVariantForLargeModel(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	size := 70.0
	agent := r.Get("architect", &size)
	if agent == nil {
		t.Fatal("expected to get architect agent")
	}
	if agent.Compact {
		t.Error("expected compact=false for 70B model")
	}
}

func TestGet_MissingAgent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	agent := r.Get("nonexistent", nil)
	if agent != nil {
		t.Error("expected nil for missing agent")
	}
}

func TestList_ExcludesCompactVariants(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	list := r.List("")
	for _, agent := range list {
		if strings.Contains(agent.Name, ".compact") {
			t.Errorf("compact variant %q should not appear in list", agent.Name)
		}
	}
}

func TestList_DefaultAgentFirst(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	list := r.List("explore")
	if len(list) == 0 {
		t.Fatal("list should not be empty")
	}
	if list[0].Name != "explore" {
		t.Errorf("expected explore first, got %s", list[0].Name)
	}
}

func TestApplyConfigOverrides_DisableAgent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	r.ApplyConfigOverrides(map[string]ConfigOverride{
		"architect": {Disable: true},
	}, defaultPerms(), nil)

	if agent := r.Get("architect", nil); agent != nil {
		t.Error("Get should not serve disabled architect")
	}

	found := false
	for _, agent := range r.ListAll("") {
		if agent.Name == "architect" {
			found = true
			if !agent.Disabled {
				t.Error("architect should be disabled in ListAll")
			}
		}
	}
	if !found {
		t.Error("architect should still exist in ListAll")
	}
}

func TestApplyConfigOverrides_DisableNativeAgentBlocked(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	r.ApplyConfigOverrides(map[string]ConfigOverride{
		"build": {Disable: true},
	}, defaultPerms(), nil)

	agent := r.Get("build", nil)
	if agent == nil {
		t.Fatal("build agent should still exist")
	}
	if agent.Disabled {
		t.Error("native agent build should not be disabled")
	}
}

func TestList_ExcludesDisabledAgents(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	r.ApplyConfigOverrides(map[string]ConfigOverride{
		"architect": {Disable: true},
	}, defaultPerms(), nil)

	list := r.List("")
	for _, agent := range list {
		if agent.Name == "architect" {
			t.Error("disabled agent architect should not appear in List()")
		}
	}
}

func TestListAll_IncludesDisabledAgents(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	r.ApplyConfigOverrides(map[string]ConfigOverride{
		"architect": {Disable: true},
	}, defaultPerms(), nil)

	list := r.ListAll("")
	found := false
	for _, agent := range list {
		if agent.Name == "architect" {
			found = true
			if !agent.Disabled {
				t.Error("architect should be marked disabled in ListAll")
			}
		}
	}
	if !found {
		t.Error("disabled agent architect should appear in ListAll()")
	}
}

func TestDefaultAgent_SkipsDisabled(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	// Disable the default "build" agent via direct field manipulation.
	r.mu.Lock()
	r.agents["build"].Disabled = true
	r.mu.Unlock()

	name, err := r.DefaultAgent("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name == "build" {
		t.Error("disabled build should not be returned as default")
	}
}

func TestDefaultAgent_RejectsDisabledConfig(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	// Directly disable a primary agent to test DefaultAgent rejection.
	r.mu.Lock()
	r.agents["plan"].Disabled = true
	r.mu.Unlock()

	_, err := r.DefaultAgent("plan")
	if err == nil {
		t.Fatal("expected error for disabled agent default")
	}
}

func TestLoadDefaults_ArchivedAgentsDisabled(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	for _, name := range []string{"code-simplifier", "qa-tester", "scientist"} {
		if agent := r.Get(name, nil); agent != nil {
			t.Errorf("Get should not serve archived agent %q", name)
		}
		size := 7.0
		if agent := r.Get(name, &size); agent != nil {
			t.Errorf("Get should not serve archived compact peer for %q", name)
		}

		r.mu.RLock()
		base := r.agents[name]
		compact := r.agents[name+".compact"]
		r.mu.RUnlock()
		if base == nil || !base.Disabled {
			t.Errorf("archived agent %q should exist and be disabled", name)
		}
		if compact == nil || !compact.Disabled {
			t.Errorf("archived compact peer %q.compact should exist and be disabled", name)
		}
	}
}

func TestGet_DoesNotServeDisabled(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	r.mu.Lock()
	r.agents["architect"].Disabled = true
	r.mu.Unlock()

	if agent := r.Get("architect", nil); agent != nil {
		t.Error("Get should return nil for disabled agent")
	}
}

func TestExploreUsesMarkdownPrompt(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	explore := r.Get("explore", nil)
	if explore == nil {
		t.Fatal("explore agent not found")
	}
	if !explore.Native {
		t.Error("explore should remain native")
	}
	if !strings.Contains(explore.Prompt, "You are Explorer") {
		t.Error("explore prompt should come from explore.md body")
	}
	if strings.Contains(explore.Prompt, "file search specialist") {
		t.Error("explore prompt should not be the explore.txt fallback")
	}

	// Native permissions retained.
	rule := permission.Evaluate("webfetch", "*", explore.Permission)
	if rule.Action != permission.ActionAllow {
		t.Errorf("explore should allow webfetch, got %s", rule.Action)
	}
}

func TestExploreCompactPermissionsAligned(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	size := 7.0
	compact := r.Get("explore", &size)
	if compact == nil {
		t.Fatal("explore compact not found")
	}
	for _, perm := range []string{"bash", "glob", "grep", "read", "webfetch", "websearch"} {
		rule := permission.Evaluate(perm, "*", compact.Permission)
		if rule.Action != permission.ActionAllow {
			t.Errorf("explore.compact should allow %s, got %s", perm, rule.Action)
		}
	}
	rule := permission.Evaluate("edit", "*", compact.Permission)
	if rule.Action != permission.ActionDeny {
		t.Errorf("explore.compact should deny edit, got %s", rule.Action)
	}
}

func TestApplyFrontmatter_ModelTempTopPName(t *testing.T) {
	info := &Info{Name: "file-stem", Mode: ModeAll, Options: make(map[string]any)}
	applyFrontmatter(info, map[string]any{
		"name":        "display-name",
		"temperature": 0.7,
		"top_p":       0.9,
		"model":       "ollama/qwen3:8b",
	})
	if info.Name != "display-name" {
		t.Errorf("expected Name display-name, got %q", info.Name)
	}
	if info.Temperature == nil || *info.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", info.Temperature)
	}
	if info.TopP == nil || *info.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %v", info.TopP)
	}
	if info.Model == nil || info.Model.ProviderID != "ollama" || info.Model.ModelID != "qwen3:8b" {
		t.Errorf("unexpected model: %+v", info.Model)
	}
}

func TestParseFrontmatter_NestedPermissionMaps(t *testing.T) {
	input := `---
description: nested perms
permission:
  "*": deny
  bash:
    "*": allow
    "rm *": deny
  read: allow
---

body`

	fm, _ := parseFrontmatter(input)
	rules := extractPermissionRules(fm)

	var foundBashAll, foundBashRm, foundRead, foundDenyAll bool
	for _, r := range rules {
		switch {
		case r.Permission == "*" && r.Pattern == "*" && r.Action == permission.ActionDeny:
			foundDenyAll = true
		case r.Permission == "bash" && r.Pattern == "*" && r.Action == permission.ActionAllow:
			foundBashAll = true
		case r.Permission == "bash" && r.Pattern == "rm *" && r.Action == permission.ActionDeny:
			foundBashRm = true
		case r.Permission == "read" && r.Pattern == "*" && r.Action == permission.ActionAllow:
			foundRead = true
		}
	}
	if !foundDenyAll || !foundBashAll || !foundBashRm || !foundRead {
		t.Errorf("nested permission rules incomplete: denyAll=%v bashAll=%v bashRm=%v read=%v rules=%+v",
			foundDenyAll, foundBashAll, foundBashRm, foundRead, rules)
	}
}

func TestLoadFromDirectory_OverwritesBundledNotNative(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	dir := t.TempDir()
	bundledOverride := `---
description: overridden architect
---
Overridden architect prompt.`
	nativeOverride := `---
description: should not replace build
---
Should not win.`
	if err := os.WriteFile(filepath.Join(dir, "architect.md"), []byte(bundledOverride), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.md"), []byte(nativeOverride), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.LoadFromDirectory(dir, defaultPerms(), nil); err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	architect := r.Get("architect", nil)
	if architect == nil {
		t.Fatal("architect missing")
	}
	if architect.Prompt != "Overridden architect prompt." {
		t.Errorf("bundled architect should be overwritten, got %q", architect.Prompt)
	}
	if architect.Description != "overridden architect" {
		t.Errorf("unexpected description: %q", architect.Description)
	}

	build := r.Get("build", nil)
	if build == nil || !build.Native {
		t.Fatal("build should remain native")
	}
	if build.Description == "should not replace build" {
		t.Error("native build must not be overwritten")
	}
}

func TestLoadUserAgents_ProjectWinsOverUser(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	configDir := t.TempDir()
	projectDir := t.TempDir()
	userAgents := filepath.Join(configDir, "agents")
	userAgentCompat := filepath.Join(configDir, "agent")
	projectAgents := filepath.Join(projectDir, ".tinycode", "agent")
	for _, d := range []string{userAgents, userAgentCompat, projectAgents} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(userAgents, "custom.md", "---\ndescription: from agents/\n---\nuser agents")
	write(userAgentCompat, "compat.md", "---\ndescription: from agent/\n---\nuser agent compat")
	write(userAgents, "architect.md", "---\ndescription: user architect\n---\nuser")
	write(projectAgents, "architect.md", "---\ndescription: project architect\n---\nproject")

	r.LoadUserAgents(configDir, projectDir, defaultPerms(), nil)

	if a := r.Get("custom", nil); a == nil || a.Description != "from agents/" {
		t.Errorf("expected user agents/ load, got %+v", a)
	}
	if a := r.Get("compat", nil); a == nil || a.Description != "from agent/" {
		t.Errorf("expected user agent/ compat load, got %+v", a)
	}
	if a := r.Get("architect", nil); a == nil || a.Description != "project architect" || a.Prompt != "project" {
		t.Errorf("project should win over user for bundled override, got %+v", a)
	}
}

func TestApplyConfigOverrides_AddCustomAgent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	r.ApplyConfigOverrides(map[string]ConfigOverride{
		"custom": {
			Description: "My custom agent",
			Prompt:      "You are a custom agent.",
			Mode:        ModeSubagent,
		},
	}, defaultPerms(), nil)

	agent := r.Get("custom", nil)
	if agent == nil {
		t.Fatal("custom agent should exist")
	}
	if agent.Description != "My custom agent" {
		t.Errorf("unexpected description: %s", agent.Description)
	}
	if agent.Mode != ModeSubagent {
		t.Errorf("expected subagent mode, got %s", agent.Mode)
	}
}

func TestApplyConfigOverrides_ModifyExisting(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	temp := 0.8
	r.ApplyConfigOverrides(map[string]ConfigOverride{
		"build": {Temperature: &temp, Color: "#00ff00"},
	}, defaultPerms(), nil)

	agent := r.Get("build", nil)
	if agent == nil {
		t.Fatal("build agent should exist")
	}
	if agent.Temperature == nil || *agent.Temperature != 0.8 {
		t.Error("temperature should be 0.8")
	}
	if agent.Color != "#00ff00" {
		t.Errorf("expected color #00ff00, got %s", agent.Color)
	}
}

func TestDefaultAgent_ReturnsBuild(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	name, err := r.DefaultAgent("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "build" {
		t.Errorf("expected build, got %s", name)
	}
}

func TestDefaultAgent_RespectsConfig(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	name, err := r.DefaultAgent("plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "plan" {
		t.Errorf("expected plan, got %s", name)
	}
}

func TestDefaultAgent_RejectsSubagent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	_, err := r.DefaultAgent("general")
	if err == nil {
		t.Fatal("expected error for subagent default")
	}
}

func TestDefaultAgent_RejectsHidden(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	_, err := r.DefaultAgent("compaction")
	if err == nil {
		t.Fatal("expected error for hidden agent default")
	}
}

func TestParseModel(t *testing.T) {
	tests := []struct {
		input      string
		providerID string
		modelID    string
	}{
		{"ollama/llama3.3", "ollama", "llama3.3"},
		{"openrouter/meta-llama/llama-3.3-70b", "openrouter", "meta-llama/llama-3.3-70b"},
		{"just-model", "", "just-model"},
	}

	for _, tt := range tests {
		ref := ParseModel(tt.input)
		if ref.ProviderID != tt.providerID {
			t.Errorf("ParseModel(%q) providerID = %q, want %q", tt.input, ref.ProviderID, tt.providerID)
		}
		if ref.ModelID != tt.modelID {
			t.Errorf("ParseModel(%q) modelID = %q, want %q", tt.input, ref.ModelID, tt.modelID)
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	input := `---
name: test-agent
description: A test agent
mode: subagent
steps: 30
hidden: true
permission:
  "*": deny
  read: allow
---

This is the prompt body.`

	fm, body := parseFrontmatter(input)
	if fm == nil {
		t.Fatal("expected frontmatter")
	}

	if fm["name"] != "test-agent" {
		t.Errorf("expected name test-agent, got %v", fm["name"])
	}
	if fm["description"] != "A test agent" {
		t.Errorf("expected description, got %v", fm["description"])
	}
	if fm["mode"] != "subagent" {
		t.Errorf("expected mode subagent, got %v", fm["mode"])
	}
	if fm["steps"] != 30 {
		t.Errorf("expected steps 30, got %v", fm["steps"])
	}
	if fm["hidden"] != true {
		t.Errorf("expected hidden true, got %v", fm["hidden"])
	}

	permMap, ok := fm["permission"].(map[string]any)
	if !ok {
		t.Fatal("expected permission map")
	}
	if permMap["*"] != "deny" {
		t.Errorf("expected * deny, got %v", permMap["*"])
	}
	if permMap["read"] != "allow" {
		t.Errorf("expected read allow, got %v", permMap["read"])
	}

	if !strings.Contains(body, "This is the prompt body") {
		t.Errorf("expected prompt body, got %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	input := "Just a prompt with no frontmatter."
	fm, body := parseFrontmatter(input)
	if fm != nil {
		t.Error("expected nil frontmatter")
	}
	if body != input {
		t.Errorf("expected body to be entire input")
	}
}

func TestNativeAgentPermissions(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	// Build agent should allow question
	build := r.Get("build", nil)
	if build == nil {
		t.Fatal("build agent not found")
	}
	rule := permission.Evaluate("question", "*", build.Permission)
	if rule.Action != permission.ActionAllow {
		t.Errorf("build should allow question, got %s", rule.Action)
	}

	// Explore agent should deny edit
	explore := r.Get("explore", nil)
	if explore == nil {
		t.Fatal("explore agent not found")
	}
	rule = permission.Evaluate("edit", "*", explore.Permission)
	if rule.Action != permission.ActionDeny {
		t.Errorf("explore should deny edit via * deny, got %s", rule.Action)
	}

	// Explore allow bash * should match shell Ask (alias).
	rule = permission.Evaluate("shell", "ls", explore.Permission)
	if rule.Action != permission.ActionAllow {
		t.Errorf("explore should allow shell via bash alias, got %s", rule.Action)
	}
}

func TestPermissionFromConfigMap_PathPatterns(t *testing.T) {
	rules := permissionFromConfigMap(map[string]any{
		"read .env*": "ask",
		"edit":       "deny",
		"bash": map[string]any{
			"rm *": "deny",
		},
	})

	var foundEnv, foundEdit, foundBash bool
	for _, r := range rules {
		switch {
		case r.Permission == "read" && r.Pattern == ".env*" && r.Action == permission.ActionAsk:
			foundEnv = true
		case r.Permission == "edit" && r.Pattern == "*" && r.Action == permission.ActionDeny:
			foundEdit = true
		case r.Permission == "bash" && r.Pattern == "rm *" && r.Action == permission.ActionDeny:
			foundBash = true
		}
	}
	if !foundEnv {
		t.Error("expected read .env* ask from space-separated key")
	}
	if !foundEdit {
		t.Error("expected edit * deny")
	}
	if !foundBash {
		t.Error("expected bash rm * deny from nested map")
	}
}

func TestLoadDefaults_BundledSpecialistsDefaultSubagent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	for _, name := range []string{"architect", "debugger", "code-reviewer", "critic", "verifier"} {
		agent := r.Get(name, nil)
		if agent == nil {
			t.Errorf("bundled agent %q not loaded", name)
			continue
		}
		if agent.Mode != ModeSubagent {
			t.Errorf("expected %q mode subagent (omitted frontmatter), got %s", name, agent.Mode)
		}
	}

	for _, name := range []string{"executor"} {
		agent := r.Get(name, nil)
		if agent == nil {
			t.Errorf("bundled agent %q not loaded", name)
			continue
		}
		if agent.Mode != ModePrimary {
			t.Errorf("expected %q mode primary (explicit frontmatter), got %s", name, agent.Mode)
		}
	}
}

func TestLoadFromDirectory_DefaultModeSubagent(t *testing.T) {
	r := NewRegistry()
	dir := t.TempDir()
	content := `---
description: no mode set
---
Prompt body.`
	if err := os.WriteFile(filepath.Join(dir, "custom.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadFromDirectory(dir, defaultPerms(), nil); err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}
	agent := r.Get("custom", nil)
	if agent == nil {
		t.Fatal("custom agent not loaded")
	}
	if agent.Mode != ModeSubagent {
		t.Errorf("expected default mode subagent, got %s", agent.Mode)
	}
}

func TestLoadDefaults_PlanPathScopedEdit(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	plan := r.Get("plan", nil)
	if plan == nil {
		t.Fatal("plan not loaded")
	}

	allow := permission.Evaluate("edit", "plans/foo.md", plan.Permission)
	if allow.Action != permission.ActionAllow {
		t.Errorf("expected edit plans/foo.md allow, got %s", allow.Action)
	}
	drafts := permission.Evaluate("edit", "drafts/notes.md", plan.Permission)
	if drafts.Action != permission.ActionAllow {
		t.Errorf("expected edit drafts/notes.md allow, got %s", drafts.Action)
	}
	deny := permission.Evaluate("edit", "internal/foo.go", plan.Permission)
	if deny.Action != permission.ActionDeny {
		t.Errorf("expected edit internal/foo.go deny, got %s", deny.Action)
	}
	read := permission.Evaluate("read", "internal/foo.go", plan.Permission)
	if read.Action != permission.ActionAllow {
		t.Errorf("expected read internal/foo.go allow, got %s", read.Action)
	}
}

func TestLoadDefaults_PlanAgentAllowsPlanExit(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	plan := r.Get("plan", nil)
	if plan == nil {
		t.Fatal("plan not loaded")
	}

	exit := permission.Evaluate("plan_exit", "*", plan.Permission)
	if exit.Action != permission.ActionAllow {
		t.Errorf("expected plan_exit allow on plan agent, got %s", exit.Action)
	}
}

func TestLoadDefaults_BuildAgentAllowsPlanEnter(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	build := r.Get("build", nil)
	if build == nil {
		t.Fatal("build not loaded")
	}

	enter := permission.Evaluate("plan_enter", "*", build.Permission)
	if enter.Action != permission.ActionAllow {
		t.Errorf("expected plan_enter allow on build agent, got %s", enter.Action)
	}
}

func TestLoadDefaults_AgentListHasPlanNotPlanner(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}

	names := make(map[string]bool)
	for _, agent := range r.List("build") {
		names[agent.Name] = true
	}
	if !names["plan"] {
		t.Error("expected agent list to contain \"plan\"")
	}
	if names["planner"] {
		t.Error("expected agent list to not contain \"planner\"")
	}
	if r.Get("planner", nil) != nil {
		t.Error("expected \"planner\" agent to no longer be registered")
	}
}

func TestLoadDefaults_ScoutPromptHonesty(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDefaults(defaultPerms(), nil); err != nil {
		t.Fatalf("LoadDefaults failed: %v", err)
	}
	scout := r.Get("scout", nil)
	if scout == nil {
		t.Fatal("scout not loaded")
	}
	for _, banned := range []string{"repo_clone", "repo_overview"} {
		if strings.Contains(scout.Prompt, banned) {
			t.Errorf("scout prompt must not mention %q", banned)
		}
	}
	for _, want := range []string{"WebFetch", "WebSearch", "Read", "Grep", "Glob"} {
		if !strings.Contains(scout.Prompt, want) {
			t.Errorf("scout prompt should mention %q", want)
		}
	}
}
