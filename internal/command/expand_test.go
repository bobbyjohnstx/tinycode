package command

import (
	"strings"
	"testing"
)

func TestExpandSlashCommand_Swarm(t *testing.T) {
	result := ExpandSlashCommand("/swarm run file on all go files")
	if !strings.Contains(result, "SWARM mode") {
		t.Error("expected SWARM mode prefix")
	}
	if !strings.Contains(result, "run file on all go files") {
		t.Error("expected user task preserved")
	}
	if strings.Contains(result, "/swarm") {
		t.Error("expected /swarm prefix stripped")
	}
}

func TestExpandSlashCommand_WorkLoop(t *testing.T) {
	result := ExpandSlashCommand("/work-loop fix all lint errors")
	if !strings.Contains(result, "WORK-LOOP mode") {
		t.Error("expected WORK-LOOP mode prefix")
	}
	if !strings.Contains(result, "fix all lint errors") {
		t.Error("expected user task preserved")
	}
}

func TestExpandSlashCommand_NoMatch(t *testing.T) {
	input := "just a regular prompt"
	result := ExpandSlashCommand(input)
	if result != input {
		t.Errorf("expected passthrough, got %q", result)
	}
}

func TestExpandSlashCommand_SwarmPreservedAfterParse(t *testing.T) {
	expanded := ExpandSlashCommand("/swarm run tests on all packages")
	if !strings.Contains(expanded, "SWARM mode") {
		t.Error("expected SWARM mode prefix preserved")
	}
	if !strings.Contains(expanded, "run tests on all packages") {
		t.Error("expected user task preserved")
	}
	if strings.Contains(expanded, "/swarm") {
		t.Error("expected /swarm prefix stripped")
	}
}

func TestExpandSlashCommand_SwarmForegroundInstruction(t *testing.T) {
	result := ExpandSlashCommand("/swarm test something")
	if !strings.Contains(result, "foreground") {
		t.Error("expected foreground instruction in swarm prefix")
	}
	if !strings.Contains(result, "ONLY the task tool") {
		t.Error("expected restriction to task tool only")
	}
	if !strings.Contains(result, "MULTIPLE task calls") {
		t.Error("expected parallel task call instruction")
	}
}
