package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/skill"
)

func TestBuiltinCommands_Count(t *testing.T) {
	cmds := builtinCommands()
	if len(cmds) != 17 {
		t.Fatalf("expected 17 builtins, got %d", len(cmds))
	}
	expected := []string{"branch", "init", "review", "debug", "trace", "plan", "verify", "test", "ask", "swarm", "auto-approve", "effort", "btw", "goal", "rewind", "hooks", "context"}
	for i, name := range expected {
		if cmds[i].Name != name {
			t.Errorf("builtins[%d].Name = %q, want %q", i, cmds[i].Name, name)
		}
	}
}

func TestBuiltinCommands_SourceIsBuiltin(t *testing.T) {
	for _, cmd := range builtinCommands() {
		if cmd.Source != "builtin" {
			t.Errorf("command %q source = %q, want %q", cmd.Name, cmd.Source, "builtin")
		}
	}
}

func TestDiscover_EmptyDirsReturnsBuiltinsAndDefaults(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	cmds := Discover(configDir, projectDir, nil)

	defaultSkills := skill.DefaultSkills()
	builtins := builtinCommands()
	builtinNames := make(map[string]bool, len(builtins))
	for _, cmd := range builtins {
		builtinNames[cmd.Name] = true
	}
	deduped := 0
	for _, ds := range defaultSkills {
		if builtinNames[ds.Name] {
			deduped++
		}
	}
	expected := len(builtins) + len(defaultSkills) - deduped
	if len(cmds) != expected {
		t.Fatalf("expected %d commands (%d builtins + %d default skills - %d deduped), got %d", expected, len(builtins), len(defaultSkills), deduped, len(cmds))
	}
}

func TestDiscover_AgentNamesAddedAsCommands(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	agents := []string{"debugger", "executor", "architect"}
	cmds := Discover(configDir, projectDir, agents)

	for _, name := range agents {
		found := false
		for _, cmd := range cmds {
			if cmd.Name == name {
				found = true
				if cmd.Source != "builtin" {
					t.Errorf("agent command %q source = %q, want %q", name, cmd.Source, "builtin")
				}
				break
			}
		}
		if !found {
			t.Errorf("agent command %q not found", name)
		}
	}
}

func TestDiscover_AgentNameDuplicatingBuiltinSkipped(t *testing.T) {
	configDir := t.TempDir()
	agents := []string{"init", "debugger"}
	cmds := Discover(configDir, "", agents)

	count := 0
	for _, cmd := range cmds {
		if cmd.Name == "init" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'init' command, got %d", count)
	}
}

func TestDiscover_UserSkills(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: my-skill\ndescription: A custom skill\n---\nSkill body."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := Discover(configDir, "", nil)

	found := false
	for _, cmd := range cmds {
		if cmd.Name == "my-skill" {
			found = true
			if cmd.Description != "A custom skill" {
				t.Errorf("description = %q, want %q", cmd.Description, "A custom skill")
			}
			if cmd.Source != "skill" {
				t.Errorf("source = %q, want %q", cmd.Source, "skill")
			}
			break
		}
	}
	if !found {
		t.Error("user skill 'my-skill' not discovered")
	}
}

func TestDiscover_ProjectSkills(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	skillDir := filepath.Join(projectDir, ".tinycode", "skills", "proj-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: proj-skill\ndescription: Project-level skill\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := Discover(configDir, projectDir, nil)

	found := false
	for _, cmd := range cmds {
		if cmd.Name == "proj-skill" {
			found = true
			if cmd.Description != "Project-level skill" {
				t.Errorf("description = %q, want %q", cmd.Description, "Project-level skill")
			}
			break
		}
	}
	if !found {
		t.Error("project skill 'proj-skill' not discovered")
	}
}

func TestDiscover_SkillDuplicatingBuiltinSkipped(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "review-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: review\ndescription: Duplicate of builtin\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := Discover(configDir, "", nil)

	count := 0
	for _, cmd := range cmds {
		if cmd.Name == "review" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'review' command (builtin only), got %d", count)
	}
}

func TestDiscover_FrontmatterOverridesDirName(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "dir-name")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: custom-name\ndescription: Overridden name\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := Discover(configDir, "", nil)

	foundCustom := false
	foundDir := false
	for _, cmd := range cmds {
		if cmd.Name == "custom-name" {
			foundCustom = true
		}
		if cmd.Name == "dir-name" {
			foundDir = true
		}
	}
	if !foundCustom {
		t.Error("expected command with frontmatter name 'custom-name'")
	}
	if foundDir {
		t.Error("directory name 'dir-name' should not appear when frontmatter overrides")
	}
}

func TestDiscover_DefaultSkillsAppearAsCommands(t *testing.T) {
	configDir := t.TempDir()
	cmds := Discover(configDir, "", nil)

	expectedSkills := []string{"remember", "deepinit", "doctor", "mcp-setup", "incident", "change", "host"}
	for _, name := range expectedSkills {
		found := false
		for _, cmd := range cmds {
			if cmd.Name == name {
				found = true
				if cmd.Source != "skill" {
					t.Errorf("default skill command %q source = %q, want %q", name, cmd.Source, "skill")
				}
				if cmd.Description == "" {
					t.Errorf("default skill command %q has empty description", name)
				}
				break
			}
		}
		if !found {
			t.Errorf("default skill command %q not found", name)
		}
	}
}

func TestDiscover_DefaultSkillReviewDedupedByBuiltin(t *testing.T) {
	configDir := t.TempDir()
	cmds := Discover(configDir, "", nil)

	count := 0
	for _, cmd := range cmds {
		if cmd.Name == "review" {
			count++
			if cmd.Source != "builtin" {
				t.Errorf("review command source = %q, want %q (builtin should win)", cmd.Source, "builtin")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'review' command, got %d", count)
	}
}
