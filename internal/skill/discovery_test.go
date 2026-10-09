package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscover_NonExistentDirsReturnsDefaults(t *testing.T) {
	skills := Discover("/nonexistent/config", "/nonexistent/project")
	defaultCount := len(DefaultSkills())
	if len(skills) != defaultCount {
		t.Errorf("expected %d skills (defaults only), got %d", defaultCount, len(skills))
	}
	for _, s := range skills {
		if s.Source != "builtin" {
			t.Errorf("expected all skills to be builtin, got source %q for %q", s.Source, s.Name)
		}
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
	expectedCount := 1 + len(DefaultSkills())
	if len(skills) != expectedCount {
		t.Fatalf("expected %d skills (1 user + %d defaults), got %d", expectedCount, len(DefaultSkills()), len(skills))
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
	expectedCount := 1 + len(DefaultSkills())
	if len(skills) != expectedCount {
		t.Fatalf("expected %d skills (1 project + %d defaults), got %d", expectedCount, len(DefaultSkills()), len(skills))
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
	expectedCount := 1 + len(DefaultSkills())
	if len(skills) != expectedCount {
		t.Fatalf("expected %d skills, got %d", expectedCount, len(skills))
	}
	if skills[0].Name != "custom-name" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "custom-name")
	}
	if skills[0].Dir != skillDir {
		t.Errorf("Dir = %q, want %q", skills[0].Dir, skillDir)
	}
}

func TestDiscover_DirSetForUserSkill(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	if skills[0].Dir != skillDir {
		t.Errorf("Dir = %q, want %q", skills[0].Dir, skillDir)
	}
	for _, s := range skills {
		if s.Source == "builtin" && s.Dir != "" {
			t.Errorf("builtin skill %q should have empty Dir, got %q", s.Name, s.Dir)
		}
	}
}

func TestDiscoverWithPaths(t *testing.T) {
	configDir := t.TempDir()
	extra := t.TempDir()
	skillDir := filepath.Join(extra, "extra-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: extra-skill\ndescription: From paths\n---\nExtra body"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := DiscoverWithPaths(configDir, "", []string{extra})
	found := false
	for _, s := range skills {
		if s.Name == "extra-skill" {
			found = true
			if s.Source != "path" {
				t.Errorf("Source = %q, want path", s.Source)
			}
			if s.Dir != skillDir {
				t.Errorf("Dir = %q, want %q", s.Dir, skillDir)
			}
		}
	}
	if !found {
		t.Fatal("expected extra-skill from skills.paths")
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
	defaultCount := len(DefaultSkills())
	if len(skills) != defaultCount {
		t.Errorf("expected %d skills (defaults only, file entry skipped), got %d", defaultCount, len(skills))
	}
}

func TestDiscover_DirWithoutSkillMDSkipped(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "empty-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	defaultCount := len(DefaultSkills())
	if len(skills) != defaultCount {
		t.Errorf("expected %d skills (defaults only, no SKILL.md), got %d", defaultCount, len(skills))
	}
}

func TestDefaultSkills_LoadedAndNamed(t *testing.T) {
	skills := DefaultSkills()
	if len(skills) < 4 {
		t.Fatalf("expected at least 4 default skills, got %d", len(skills))
	}

	expected := map[string]bool{
		"remember": false, "deepinit": false, "doctor": false,
		"mcp-setup": false, "incident": false, "change": false, "host": false,
	}
	for _, s := range skills {
		if _, ok := expected[s.Name]; ok {
			expected[s.Name] = true
		}
		if s.Source != "builtin" {
			t.Errorf("default skill %q source = %q, want %q", s.Name, s.Source, "builtin")
		}
		if s.Description == "" {
			t.Errorf("default skill %q has empty description", s.Name)
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected default skill %q not found", name)
		}
	}
}

func TestReadDefaultSkill(t *testing.T) {
	content, err := ReadDefaultSkill("doctor")
	if err != nil {
		t.Fatalf("ReadDefaultSkill(doctor) error: %v", err)
	}
	if content == "" {
		t.Fatal("ReadDefaultSkill(doctor) returned empty content")
	}
	if !strings.Contains(content, "name: doctor") {
		t.Error("expected doctor skill content to contain frontmatter")
	}
	if !strings.Contains(content, "# Doctor") {
		t.Error("expected doctor skill content to contain body heading")
	}
}

func TestReadDefaultSkill_OpsChecklists(t *testing.T) {
	cases := []struct {
		name    string
		needles []string
	}{
		{"incident", []string{"name: incident", "what is broken", "One next command"}},
		{"change", []string{"name: change", "exact command", "Undo"}},
		{"host", []string{"name: host", "ssh", "permission prompt", "Do not print key material", "SELinux"}},
	}
	for _, c := range cases {
		content, err := ReadDefaultSkill(c.name)
		if err != nil {
			t.Errorf("ReadDefaultSkill(%s): %v", c.name, err)
			continue
		}
		for _, needle := range c.needles {
			if !strings.Contains(content, needle) {
				t.Errorf("%s skill missing %q", c.name, needle)
			}
		}
	}
}

func TestReadDefaultSkill_NotFound(t *testing.T) {
	_, err := ReadDefaultSkill("nonexistent-skill")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestDiscover_UserOverridesDefault(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "debug-custom")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: debug\ndescription: My custom debug\n---\nCustom body."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	count := 0
	for _, s := range skills {
		if s.Name == "debug" {
			count++
			if s.Source != "user" {
				t.Errorf("expected user source for overridden 'debug', got %q", s.Source)
			}
			if s.Description != "My custom debug" {
				t.Errorf("expected custom description, got %q", s.Description)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'debug' skill, got %d", count)
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
