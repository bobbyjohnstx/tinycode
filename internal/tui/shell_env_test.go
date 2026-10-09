package tui

import "testing"

func TestRunUserShell_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	msg := runUserShell(`printf 'ran:%s' "$OPENROUTER_API_KEY"`, t.TempDir())()
	got, ok := msg.(ShellResultMsg)
	if !ok {
		t.Fatalf("message = %T", msg)
	}
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.Output != "ran:" {
		t.Fatalf("shell output = %q", got.Output)
	}
}
