package plugin

import (
	"context"
	"strings"
	"testing"
)

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
