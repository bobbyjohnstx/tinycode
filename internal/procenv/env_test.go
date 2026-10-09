package procenv

import (
	"strings"
	"testing"
)

func TestSecretName(t *testing.T) {
	secret := []string{
		"OPENROUTER_API_KEY",
		"openai_api_key",
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_ACCESS_KEY_ID",
		"TINYCODE_AUTH_TOKEN",
		"DB_PASSWORD",
	}
	for _, name := range secret {
		if !SecretName(name) {
			t.Errorf("%s should be treated as a credential", name)
		}
	}

	kept := []string{"PATH", "HOME", "USER", "LANG", "KUBECONFIG", "SSH_AUTH_SOCK", "GOPROXY", "TERM"}
	for _, name := range kept {
		if SecretName(name) {
			t.Errorf("%s should stay available to child processes", name)
		}
	}
}

func TestChild_DropsCredentialsAndKeepsPath(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	t.Setenv("PATH", "/usr/bin")

	got := Child(nil)
	if got == nil {
		t.Fatal("nil env inherits the parent")
	}
	for _, entry := range got {
		if strings.HasPrefix(entry, "OPENROUTER_API_KEY=") {
			t.Fatalf("leaked %s", entry)
		}
	}
	if !containsEnv(got, "PATH=/usr/bin") {
		t.Fatalf("PATH missing from %#v", got)
	}
}

func TestChild_ExtraCanPassACredential(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "from-parent")
	got := Child(map[string]string{"OPENROUTER_API_KEY": "from-config"})
	if !containsEnv(got, "OPENROUTER_API_KEY=from-config") {
		t.Fatalf("configured value missing from %#v", got)
	}
	if containsEnv(got, "OPENROUTER_API_KEY=from-parent") {
		t.Fatal("parent credential was kept")
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
