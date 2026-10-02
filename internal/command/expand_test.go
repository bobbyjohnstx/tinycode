package command

import (
	"strings"
	"testing"
)

func TestExpandSlashCommand_Swarm(t *testing.T) {
	result := ExpandSlashCommand("/swarm run file on all go files")
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
	result := ExpandSlashCommand("/swarm run tests on all packages")
	if !result.AutoApprove {
		t.Error("expected AutoApprove=true for /swarm")
	}
}

func TestExpandSlashCommand_WorkLoop(t *testing.T) {
	result := ExpandSlashCommand("/work-loop fix all lint errors")
	if !strings.Contains(result.Text, "WORK-LOOP mode") {
		t.Error("expected WORK-LOOP mode prefix")
	}
	if !strings.Contains(result.Text, "fix all lint errors") {
		t.Error("expected user task preserved")
	}
}

func TestExpandSlashCommand_WorkLoopNoAutoApprove(t *testing.T) {
	result := ExpandSlashCommand("/work-loop fix lint errors")
	if result.AutoApprove {
		t.Error("expected AutoApprove=false for /work-loop")
	}
}

func TestExpandSlashCommand_NoMatch(t *testing.T) {
	input := "just a regular prompt"
	result := ExpandSlashCommand(input)
	if result.Text != input {
		t.Errorf("expected passthrough, got %q", result.Text)
	}
	if result.AutoApprove {
		t.Error("expected AutoApprove=false for plain text")
	}
}

func TestExpandSlashCommand_SwarmPreservedAfterParse(t *testing.T) {
	result := ExpandSlashCommand("/swarm run tests on all packages")
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
	result := ExpandSlashCommand("/swarm test something")
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
	result := ExpandSlashCommand("/swarm --plan refactor the auth module")
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
	result := ExpandSlashCommand("/swarm refactor the auth module --plan")
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
	result := ExpandSlashCommand("/swarm run tests on all packages")
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
	result := ExpandSlashCommand("/swarm --plan fix all lint errors")
	if result.DisplayText != "/swarm --plan fix all lint errors" {
		t.Errorf("expected display text with --plan, got %q", result.DisplayText)
	}
}
