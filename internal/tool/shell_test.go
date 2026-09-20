package tool

import "testing"

func TestCheckSecretAccess_DetectsStandaloneDotEnv(t *testing.T) {
	// Standalone .env preceded by space — the original \b\.env\b regex missed this
	// because \b doesn't fire before a dot when preceded by whitespace.
	for _, cmd := range []string{"cat .env", "source .env", "less .env", "vim .env"} {
		if w := checkSecretAccess(cmd); w == "" {
			t.Errorf("expected warning for %q, got empty string", cmd)
		}
	}
}

func TestCheckSecretAccess_DetectsDotEnvInFilename(t *testing.T) {
	result := checkSecretAccess("cat config.env")
	if result == "" {
		t.Error("expected warning for 'cat config.env', got empty string")
	}
}

func TestCheckSecretAccess_DetectsDotEnvWithEnvironmentSuffix(t *testing.T) {
	// The \b\.env\.\w+ pattern matches filenames like app.env.production
	result := checkSecretAccess("cat app.env.production")
	if result == "" {
		t.Error("expected warning for 'cat app.env.production', got empty string")
	}
}

func TestCheckSecretAccess_DetectsEnvSuffixOnSource(t *testing.T) {
	result := checkSecretAccess("source config.env.local")
	if result == "" {
		t.Error("expected warning for 'source config.env.local', got empty string")
	}
}

func TestCheckSecretAccess_DetectsCredentialsJSON(t *testing.T) {
	result := checkSecretAccess("cat credentials.json")
	if result == "" {
		t.Error("expected warning for 'cat credentials.json', got empty string")
	}
}

func TestCheckSecretAccess_DetectsCredentialsFile(t *testing.T) {
	result := checkSecretAccess("less credentials")
	if result == "" {
		t.Error("expected warning for 'less credentials', got empty string")
	}
}

func TestCheckSecretAccess_DetectsKeyFile(t *testing.T) {
	result := checkSecretAccess("cat server.key")
	if result == "" {
		t.Error("expected warning for 'cat server.key', got empty string")
	}
}

func TestCheckSecretAccess_DetectsPemFile(t *testing.T) {
	result := checkSecretAccess("openssl x509 -in cert.pem")
	if result == "" {
		t.Error("expected warning for 'openssl x509 -in cert.pem', got empty string")
	}
}

func TestCheckSecretAccess_IgnoresNonSecretFiles(t *testing.T) {
	benign := []string{
		"cat environment.txt",
		"echo 'not a key'",
		"ls -la",
		"grep pattern file.go",
		"cat readme.md",
	}
	for _, cmd := range benign {
		if w := checkSecretAccess(cmd); w != "" {
			t.Errorf("expected no warning for %q, got %q", cmd, w)
		}
	}
}

func TestIsDestructive_DetectsRmRecursiveForce(t *testing.T) {
	destructive := []string{
		"rm -rf /tmp/dir",
		"rm -Rf /tmp/dir",
		"rm --recursive /tmp/dir",
	}
	for _, cmd := range destructive {
		if !isDestructive(cmd) {
			t.Errorf("expected destructive for %q", cmd)
		}
	}
}

func TestIsDestructive_DetectsGitForceOperations(t *testing.T) {
	destructive := []string{
		"git push --force origin main",
		"git reset --hard HEAD~1",
		"git clean -fd",
		"git branch -D feature",
	}
	for _, cmd := range destructive {
		if !isDestructive(cmd) {
			t.Errorf("expected destructive for %q", cmd)
		}
	}
}

func TestIsDestructive_DetectsSQLDestructiveOps(t *testing.T) {
	destructive := []string{
		"DROP TABLE users",
		"TRUNCATE TABLE sessions",
	}
	for _, cmd := range destructive {
		if !isDestructive(cmd) {
			t.Errorf("expected destructive for %q", cmd)
		}
	}
}

func TestIsDestructive_IgnoresSafeCommands(t *testing.T) {
	safe := []string{
		"ls -la",
		"git status",
		"git push origin main",
		"cat file.txt",
		"go test ./...",
	}
	for _, cmd := range safe {
		if isDestructive(cmd) {
			t.Errorf("expected safe for %q", cmd)
		}
	}
}
