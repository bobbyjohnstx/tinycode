package agent

import (
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

	bundled := []string{"architect", "debugger", "executor", "code-reviewer", "planner"}
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

	agent := r.Get("architect", nil)
	if agent == nil {
		t.Fatal("architect should still exist")
	}
	if !agent.Disabled {
		t.Error("architect should be disabled")
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
		agent := r.Get(name, nil)
		if agent == nil {
			t.Errorf("archived agent %q should exist", name)
			continue
		}
		if !agent.Disabled {
			t.Errorf("archived agent %q should be disabled by default", name)
		}
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
}
