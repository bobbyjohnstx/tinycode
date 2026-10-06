package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubstituteSkillParams_Positional(t *testing.T) {
	content := "Hello $1, welcome to $2!"
	result := substituteSkillParams(content, "Alice Wonderland")
	if result != "Hello Alice, welcome to Wonderland!" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSubstituteSkillParams_Arguments(t *testing.T) {
	content := "Run: $ARGUMENTS"
	result := substituteSkillParams(content, "foo bar baz")
	if result != "Run: foo bar baz" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSubstituteSkillParams_Mixed(t *testing.T) {
	content := "Name: $1\nAll: $ARGUMENTS"
	result := substituteSkillParams(content, "alpha beta")
	expected := "Name: alpha\nAll: alpha beta"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteSkillParams_NoArgs(t *testing.T) {
	content := "Static content with $1 placeholder"
	result := substituteSkillParams(content, "")
	// $1 is not replaced since there are no arguments
	if result != "Static content with $1 placeholder" {
		t.Errorf("unexpected result: %s", result)
	}
	// $ARGUMENTS should be replaced with empty string
	content2 := "Args: $ARGUMENTS end"
	result2 := substituteSkillParams(content2, "")
	if result2 != "Args:  end" {
		t.Errorf("unexpected result: %s", result2)
	}
}

func TestSubstituteSkillParams_ExtraPositional(t *testing.T) {
	content := "$1 and $2 and $3"
	result := substituteSkillParams(content, "a b")
	// $3 should remain unreplaced
	if result != "a and b and $3" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSubstituteSkillParams_TenNotCorruptedByOne(t *testing.T) {
	content := "args: $1 $10"
	result := substituteSkillParams(content, "A B C D E F G H I J")
	if result != "args: A J" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSkillTool_StripsFrontmatter(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()

	skillDir := filepath.Join(configDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: greet\n---\nHello $1!"), 0644)

	def := SkillTool(configDir, projectDir)
	args, _ := json.Marshal(skillArgs{Name: "greet", Arguments: "World"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if strings.Contains(result.Output, "---") || strings.Contains(result.Output, "name: greet") {
		t.Errorf("frontmatter leaked into output: %s", result.Output)
	}
	if result.Output != "Hello World!" {
		t.Errorf("got %q, want Hello World!", result.Output)
	}
}

func TestSkillTool_NameOverrideReadsDir(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()

	skillDir := filepath.Join(configDir, "skills", "dir-name")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: custom-name\n---\nLoaded OK $1"), 0644)

	def := SkillTool(configDir, projectDir)
	args, _ := json.Marshal(skillArgs{Name: "custom-name", Arguments: "x"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "Loaded OK x" {
		t.Errorf("got %q, want Loaded OK x", result.Output)
	}
}

func TestSkillTool_NotFound(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()

	def := SkillTool(configDir, projectDir)
	args, _ := json.Marshal(skillArgs{Name: "nonexistent"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing skill")
	}
	if !strings.Contains(result.Output, "not found") {
		t.Errorf("expected 'not found' message, got: %s", result.Output)
	}
}

func TestSkillTool_ExecuteSkill(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()

	// Create a skill
	skillDir := filepath.Join(configDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: greet\ndescription: Greeting skill\n---\nHello $1! $ARGUMENTS"), 0644)

	def := SkillTool(configDir, projectDir)
	args, _ := json.Marshal(skillArgs{Name: "greet", Arguments: "World extra"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Hello World!") {
		t.Errorf("expected greeting in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "World extra") {
		t.Errorf("expected full arguments in output, got: %s", result.Output)
	}
}

func TestSkillTool_ListsAvailable(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()

	// Create two skills
	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(configDir, "skills", name)
		os.MkdirAll(skillDir, 0755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: Skill "+name+"\n---\nContent"), 0644)
	}

	def := SkillTool(configDir, projectDir)
	args, _ := json.Marshal(skillArgs{Name: "nonexistent"})
	result, _ := def.Execute(context.Background(), &Context{}, args)
	if !result.IsError {
		t.Error("expected error")
	}
	if !strings.Contains(result.Output, "alpha") || !strings.Contains(result.Output, "beta") {
		t.Errorf("expected available skills listed, got: %s", result.Output)
	}
}
