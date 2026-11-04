package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandSlashCommand_Swarm(t *testing.T) {
	result := ExpandSlashCommand("/swarm run file on all go files", "", "")
	if !strings.Contains(result.Text, "SWARM mode") {
		t.Error("expected SWARM mode prefix")
	}
	if !strings.Contains(result.Text, "run file on all go files") {
		t.Error("expected user task preserved")
	}
	if strings.Contains(result.Text, "/swarm") {
		t.Error("expected /swarm prefix stripped")
	}
}

func TestExpandSlashCommand_SwarmAutoApprove(t *testing.T) {
	result := ExpandSlashCommand("/swarm run tests on all packages", "", "")
	if !result.AutoApprove {
		t.Error("expected AutoApprove=true for /swarm")
	}
}

func TestExpandSlashCommand_WorkLoop(t *testing.T) {
	result := ExpandSlashCommand("/work-loop fix all lint errors", "", "")
	if !strings.Contains(result.Text, "WORK-LOOP mode") {
		t.Error("expected WORK-LOOP mode prefix")
	}
	if !strings.Contains(result.Text, "fix all lint errors") {
		t.Error("expected user task preserved")
	}
}

func TestExpandSlashCommand_WorkLoopNoAutoApprove(t *testing.T) {
	result := ExpandSlashCommand("/work-loop fix lint errors", "", "")
	if result.AutoApprove {
		t.Error("expected AutoApprove=false for /work-loop")
	}
}

func TestExpandSlashCommand_NoMatch(t *testing.T) {
	input := "just a regular prompt"
	result := ExpandSlashCommand(input, "", "")
	if result.Text != input {
		t.Errorf("expected passthrough, got %q", result.Text)
	}
	if result.AutoApprove {
		t.Error("expected AutoApprove=false for plain text")
	}
}

func TestExpandSlashCommand_SwarmPreservedAfterParse(t *testing.T) {
	result := ExpandSlashCommand("/swarm run tests on all packages", "", "")
	if !strings.Contains(result.Text, "SWARM mode") {
		t.Error("expected SWARM mode prefix preserved")
	}
	if !strings.Contains(result.Text, "run tests on all packages") {
		t.Error("expected user task preserved")
	}
	if strings.Contains(result.Text, "/swarm") {
		t.Error("expected /swarm prefix stripped")
	}
}

func TestExpandSlashCommand_SwarmForegroundInstruction(t *testing.T) {
	result := ExpandSlashCommand("/swarm test something", "", "")
	if !strings.Contains(result.Text, "foreground") {
		t.Error("expected foreground instruction in swarm prefix")
	}
	if !strings.Contains(result.Text, "ONLY the task tool") {
		t.Error("expected restriction to task tool only")
	}
	if !strings.Contains(result.Text, "MULTIPLE task calls") {
		t.Error("expected parallel task call instruction")
	}
}

func TestExpandSlashCommand_SwarmPlanFlag(t *testing.T) {
	result := ExpandSlashCommand("/swarm --plan refactor the auth module", "", "")
	if !strings.Contains(result.Text, "SWARM PLANNING mode") {
		t.Error("expected SWARM PLANNING mode prefix")
	}
	if !strings.Contains(result.Text, "refactor the auth module") {
		t.Error("expected user task preserved")
	}
	if strings.Contains(result.Text, "--plan") {
		t.Error("expected --plan stripped from prompt text")
	}
	if !result.PlanOnly {
		t.Error("expected PlanOnly=true")
	}
	if result.AutoApprove {
		t.Error("expected AutoApprove=false for --plan")
	}
}

func TestExpandSlashCommand_SwarmPlanFlagAfterTask(t *testing.T) {
	result := ExpandSlashCommand("/swarm refactor the auth module --plan", "", "")
	if !strings.Contains(result.Text, "SWARM PLANNING mode") {
		t.Error("expected SWARM PLANNING mode prefix when --plan after task")
	}
	if !strings.Contains(result.Text, "refactor the auth module") {
		t.Error("expected user task preserved")
	}
	if strings.Contains(result.Text, "--plan") {
		t.Error("expected --plan stripped from prompt text")
	}
	if !result.PlanOnly {
		t.Error("expected PlanOnly=true when --plan after task")
	}
}

func TestExpandSlashCommand_SwarmWithoutPlanUnchanged(t *testing.T) {
	result := ExpandSlashCommand("/swarm run tests on all packages", "", "")
	if result.PlanOnly {
		t.Error("expected PlanOnly=false for /swarm without --plan")
	}
	if !result.AutoApprove {
		t.Error("expected AutoApprove=true for /swarm without --plan")
	}
	if !strings.Contains(result.Text, "SWARM mode") {
		t.Error("expected SWARM mode prefix for /swarm without --plan")
	}
}

func TestExpandSlashCommand_SwarmPlanDisplayText(t *testing.T) {
	result := ExpandSlashCommand("/swarm --plan fix all lint errors", "", "")
	if result.DisplayText != "/swarm --plan fix all lint errors" {
		t.Errorf("expected display text with --plan, got %q", result.DisplayText)
	}
}

func TestExpandSlashCommand_SwarmDisplayText(t *testing.T) {
	result := ExpandSlashCommand("/swarm review the code", "", "")
	if result.DisplayText != "/swarm review the code" {
		t.Errorf("expected DisplayText %q, got %q", "/swarm review the code", result.DisplayText)
	}
}

func TestSwarmPrefix_ContainsAllSpecialistTypes(t *testing.T) {
	specialists := []string{
		"executor",
		"explore",
		"code-reviewer",
		"security-reviewer",
		"test-engineer",
		"architect",
		"debugger",
	}
	for _, s := range specialists {
		if !strings.Contains(SwarmPrefix, `"`+s+`"`) {
			t.Errorf("SwarmPrefix missing specialist subagent_type %q", s)
		}
	}
}

func TestSwarmPlanPrefix_ContainsAllSpecialistTypes(t *testing.T) {
	specialists := []string{
		"executor",
		"explore",
		"code-reviewer",
		"security-reviewer",
		"test-engineer",
		"architect",
		"debugger",
	}
	for _, s := range specialists {
		if !strings.Contains(SwarmPlanPrefix, `"`+s+`"`) {
			t.Errorf("SwarmPlanPrefix missing specialist subagent_type %q", s)
		}
	}
}

func TestWorkLoopPrefix_ContainsKeyInstructions(t *testing.T) {
	checks := []struct {
		label  string
		needle string
	}{
		{"mode header", "WORK-LOOP mode"},
		{"completion signal", "until it is complete"},
		{"no confirmation", "Do NOT ask for confirmation"},
		{"blocked exit", "blocked"},
	}
	for _, c := range checks {
		if !strings.Contains(WorkLoopPrefix, c.needle) {
			t.Errorf("WorkLoopPrefix missing %s phrase %q", c.label, c.needle)
		}
	}
}

func TestExpandSlashCommand_WorkLoopDisplayText(t *testing.T) {
	result := ExpandSlashCommand("/work-loop fix the bug", "", "")
	if result.DisplayText != "/work-loop fix the bug" {
		t.Errorf("expected DisplayText %q, got %q", "/work-loop fix the bug", result.DisplayText)
	}
}

func TestExpandSlashCommand_BuiltinSkill(t *testing.T) {
	result := ExpandSlashCommand("/debug investigate flaky test", "", "")
	if result.Text == "/debug investigate flaky test" {
		t.Fatal("expected skill body expansion, got original slash line")
	}
	if strings.HasPrefix(strings.TrimSpace(result.Text), "---") {
		t.Error("expanded skill should not include frontmatter")
	}
	if result.DisplayText != "/debug investigate flaky test" {
		t.Errorf("DisplayText = %q", result.DisplayText)
	}
	// Args should be substituted into $ARGUMENTS when present in skill body.
	if strings.Contains(result.Text, "$ARGUMENTS") {
		t.Error("$ARGUMENTS should be substituted")
	}
	if !strings.Contains(result.Text, `subagent_type "debugger"`) {
		t.Errorf("debug alias should delegate to debugger, got %q", result.Text)
	}
	if !strings.Contains(result.Text, "investigate flaky test") {
		t.Errorf("debug alias should keep the task text, got %q", result.Text)
	}
}

func TestExpandSlashCommand_OpsSkills(t *testing.T) {
	cases := []struct {
		cmd    string
		needle string
	}{
		{"/incident api is down", "One next command"},
		{"/change scale the deployment", "Undo"},
		{"/host check this machine", "ssh"},
	}
	for _, c := range cases {
		result := ExpandSlashCommand(c.cmd, "", "")
		if result.Text == c.cmd {
			t.Errorf("%s was not expanded", c.cmd)
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(result.Text), "---") {
			t.Errorf("%s expansion includes frontmatter", c.cmd)
		}
		if !strings.Contains(result.Text, c.needle) {
			t.Errorf("%s expansion missing %q", c.cmd, c.needle)
		}
		if result.DisplayText != c.cmd {
			t.Errorf("%s DisplayText = %q", c.cmd, result.DisplayText)
		}
	}
}

func TestExpandSlashCommand_ReviewDelegates(t *testing.T) {
	result := ExpandSlashCommand("/review the auth change", "", "")
	if !strings.Contains(result.Text, `subagent_type "code-reviewer"`) {
		t.Fatalf("review alias should delegate to code-reviewer, got %q", result.Text)
	}
	if !strings.Contains(result.Text, "the auth change") {
		t.Errorf("review alias should keep the task text, got %q", result.Text)
	}
	if result.DisplayText != "/review the auth change" {
		t.Errorf("DisplayText = %q", result.DisplayText)
	}
}

func TestExpandSlashCommand_UserSkill(t *testing.T) {
	configDir := t.TempDir()
	skillDir := filepath.Join(configDir, "skills", "greet")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: greet\n---\nHello $1! Args: $ARGUMENTS"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ExpandSlashCommand("/greet World extra", configDir, "")
	if result.Text != "Hello World! Args: World extra" {
		t.Errorf("got %q", result.Text)
	}
}
