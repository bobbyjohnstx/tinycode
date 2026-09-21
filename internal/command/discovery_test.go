package command

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinCommands_Count(t *testing.T) {
	cmds := builtinCommands()
	if len(cmds) != 5 {
		t.Fatalf("expected 5 builtins, got %d", len(cmds))
	}
	expected := []string{"init", "review", "ask", "swarm", "auto-approve"}
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

func TestDiscover_EmptyDirsReturnsBuiltins(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	cmds := Discover(configDir, projectDir, nil)

	if len(cmds) != 5 {
		t.Fatalf("expected 5 commands (builtins only), got %d", len(cmds))
	}
}

func TestDiscover_AgentNamesAddedAsCommands(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	agents := []string{"debugger", "executor", "architect"}
	cmds := Discover(configDir, projectDir, agents)

	if len(cmds) != 8 {
		t.Fatalf("expected 8 commands (5 builtins + 3 agents), got %d", len(cmds))
	}
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

	if len(cmds) != 6 {
		t.Fatalf("expected 6 commands (5 builtins + 1 new agent), got %d", len(cmds))
	}
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
