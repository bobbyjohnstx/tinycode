package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadContent_StripsFrontmatter(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "greet")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: greet\ndescription: Hi\n---\nHello $1!"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	content, err := LoadContent(skills[0])
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	if strings.Contains(content, "name: greet") || strings.HasPrefix(content, "---") {
		t.Errorf("frontmatter not stripped: %q", content)
	}
	if content != "Hello $1!" {
		t.Errorf("content = %q, want %q", content, "Hello $1!")
	}
}

func TestLoadContent_Builtin(t *testing.T) {
	var debug Skill
	for _, s := range DefaultSkills() {
		if s.Name == "debug" {
			debug = s
			break
		}
	}
	if debug.Name == "" {
		t.Fatal("debug skill not found")
	}
	content, err := LoadContent(debug)
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		t.Error("expected frontmatter stripped from builtin skill")
	}
	if !strings.Contains(content, "Debug") && !strings.Contains(content, "debug") {
		snippet := content
		if len(snippet) > 80 {
			snippet = snippet[:80]
		}
		t.Errorf("unexpected builtin body: %q", snippet)
	}
}

func TestLoadContent_NameOverrideUsesDir(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "dir-name")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: custom-name\n---\nBody from dir."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := Discover(configDir, "")
	if skills[0].Name != "custom-name" {
		t.Fatalf("Name = %q", skills[0].Name)
	}
	content, err := LoadContent(skills[0])
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	if content != "Body from dir." {
		t.Errorf("content = %q, want Body from dir.", content)
	}
}

func TestSubstituteParams_HighIndexBeforeLow(t *testing.T) {
	content := "got $10 and $1"
	result := SubstituteParams(content, "A B C D E F G H I J")
	if result != "got J and A" {
		t.Errorf("got %q, want %q", result, "got J and A")
	}
}

func TestSubstituteParams_Arguments(t *testing.T) {
	result := SubstituteParams("Run: $ARGUMENTS", "foo bar")
	if result != "Run: foo bar" {
		t.Errorf("got %q", result)
	}
}
