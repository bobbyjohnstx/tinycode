package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestExecShellCommand_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	got, err := execShellCommand(context.Background(), `printf 'ran:%s' "$OPENROUTER_API_KEY"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ran:" {
		t.Fatalf("shell hook output = %q", got)
	}
}

func TestDefaultCommandFactory_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	cmd := defaultCommandFactory(context.Background(), "true")
	if cmd.Env == nil {
		t.Fatal("nil env inherits the parent")
	}
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "OPENROUTER_API_KEY=") {
			t.Fatalf("leaked %s", entry)
		}
	}
}
