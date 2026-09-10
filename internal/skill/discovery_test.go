package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_NonExistentDirsReturnsEmpty(t *testing.T) {
	skills := Discover("/nonexistent/config", "/nonexistent/project")
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestDiscover_UserSkills(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: my-skill\ndescription: A user skill\n---\nBody."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "my-skill" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "my-skill")
	}
	if skills[0].Description != "A user skill" {
		t.Errorf("Description = %q, want %q", skills[0].Description, "A user skill")
	}
	if skills[0].Source != "user" {
		t.Errorf("Source = %q, want %q", skills[0].Source, "user")
	}
	if skills[0].ID != "my-skill" {
		t.Errorf("ID = %q, want %q", skills[0].ID, "my-skill")
	}
}

func TestDiscover_ProjectSkills(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	skillDir := filepath.Join(projectDir, ".tinycode", "skills", "proj-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: proj-skill\ndescription: A project skill\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, projectDir)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Source != "project" {
		t.Errorf("Source = %q, want %q", skills[0].Source, "project")
	}
}

func TestDiscover_UserSkillDeduplicatesProjectSkill(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()

	userSkillDir := filepath.Join(configDir, "skills", "shared-skill")
	if err := os.MkdirAll(userSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte("---\nname: shared\ndescription: User version\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	projSkillDir := filepath.Join(projectDir, ".tinycode", "skills", "shared-proj")
	if err := os.MkdirAll(projSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projSkillDir, "SKILL.md"), []byte("---\nname: shared\ndescription: Project version\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, projectDir)
	count := 0
	for _, s := range skills {
		if s.Name == "shared" {
			count++
			if s.Source != "user" {
				t.Errorf("expected first-wins (user), got source %q", s.Source)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'shared' skill, got %d", count)
	}
}

func TestDiscover_FrontmatterOverridesDirName(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "dir-name")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: custom-name\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "custom-name" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "custom-name")
	}
}

func TestDiscover_NonDirectoryEntriesSkipped(t *testing.T) {
	configDir := t.TempDir()
	skillsDir := filepath.Join(configDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "not-a-dir.md"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	if len(skills) != 0 {
		t.Errorf("expected 0 skills (file entry skipped), got %d", len(skills))
	}
}

func TestDiscover_DirWithoutSkillMDSkipped(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "empty-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	if len(skills) != 0 {
		t.Errorf("expected 0 skills (no SKILL.md), got %d", len(skills))
	}
}

func TestParseParamsList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"[name, language]", []string{"name", "language"}},
		{"[]", nil},
		{"", nil},
		{"[single]", []string{"single"}},
		{"name, language", []string{"name", "language"}},
		{"[  spaced  ,  items  ]", []string{"spaced", "items"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseParamsList(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("parseParamsList(%q) len = %d, want %d", tt.input, len(result), len(tt.expected))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseParamsList(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}
