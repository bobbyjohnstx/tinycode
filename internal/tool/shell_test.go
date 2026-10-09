package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

func TestCheckSecretAccess_DetectsStandaloneDotEnv(t *testing.T) {
	// Standalone .env preceded by space — the original \b\.env\b regex missed this
	// because \b doesn't fire before a dot when preceded by whitespace.
	for _, cmd := range []string{"cat .env", "source .env", "less .env", "vim .env"} {
		if w := CheckSecretAccess(cmd); w == "" {
			t.Errorf("expected warning for %q, got empty string", cmd)
		}
	}
}

func TestCheckSecretAccess_DetectsDotEnvInFilename(t *testing.T) {
	result := CheckSecretAccess("cat config.env")
	if result == "" {
		t.Error("expected warning for 'cat config.env', got empty string")
	}
}

func TestCheckSecretAccess_DetectsDotEnvWithEnvironmentSuffix(t *testing.T) {
	// The \b\.env\.\w+ pattern matches filenames like app.env.production
	result := CheckSecretAccess("cat app.env.production")
	if result == "" {
		t.Error("expected warning for 'cat app.env.production', got empty string")
	}
}

func TestCheckSecretAccess_DetectsEnvSuffixOnSource(t *testing.T) {
	result := CheckSecretAccess("source config.env.local")
	if result == "" {
		t.Error("expected warning for 'source config.env.local', got empty string")
	}
}

func TestCheckSecretAccess_DetectsCredentialsJSON(t *testing.T) {
	result := CheckSecretAccess("cat credentials.json")
	if result == "" {
		t.Error("expected warning for 'cat credentials.json', got empty string")
	}
}

func TestCheckSecretAccess_DetectsCredentialsFile(t *testing.T) {
	result := CheckSecretAccess("less credentials")
	if result == "" {
		t.Error("expected warning for 'less credentials', got empty string")
	}
}

func TestCheckSecretAccess_DetectsKeyFile(t *testing.T) {
	result := CheckSecretAccess("cat server.key")
	if result == "" {
		t.Error("expected warning for 'cat server.key', got empty string")
	}
}

func TestCheckSecretAccess_DetectsPemFile(t *testing.T) {
	result := CheckSecretAccess("openssl x509 -in cert.pem")
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
		if w := CheckSecretAccess(cmd); w != "" {
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
		if !IsDestructive(cmd) {
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
		if !IsDestructive(cmd) {
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
		if !IsDestructive(cmd) {
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
		if IsDestructive(cmd) {
			t.Errorf("expected safe for %q", cmd)
		}
	}
}

func TestExecuteShell_SecretBlockedWithoutPerms(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	output, isErr, err := r.Execute(context.Background(), "bash", json.RawMessage(`{"command":"touch .env"}`), "sess-secret")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !isErr {
		t.Fatal("expected secret command to be blocked without a permission service")
	}
	if !strings.Contains(output, "Access to secret") {
		t.Errorf("expected secret block message, got %q", output)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf("command ran and created .env: %v", statErr)
	}
}

func TestExecuteShell_SecretAsksWhenShellIsAllowed(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	defer perms.Close()

	dir := t.TempDir()
	r := NewRegistry(&Context{
		Directory: dir,
		Bus:       b,
		Perms:     perms,
		Ruleset: permission.Ruleset{
			{Permission: "shell", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "secret-shell", Pattern: "*", Action: permission.ActionAsk},
		},
	})
	RegisterBuiltins(r)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output string
	var isErr bool
	go func() {
		output, isErr, _ = r.Execute(ctx, "bash", json.RawMessage(`{"command":"touch .env"}`), "sess-secret")
		close(done)
	}()

	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "secret-shell" {
		t.Fatalf("permission %q, want secret-shell", req.Permission)
	}
	if err := perms.RespondToAsk(permission.ReplyInput{
		RequestID: req.ID,
		Reply:     permission.ReplyOnce,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for approved secret command")
	}
	if isErr {
		t.Fatalf("approved command failed: %s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		t.Fatalf("expected .env to be created after approval: %v", err)
	}
}

func TestExecuteShell_SecretRejectSkipsCommand(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	defer perms.Close()

	dir := t.TempDir()
	r := NewRegistry(&Context{
		Directory: dir,
		Bus:       b,
		Perms:     perms,
		Ruleset: permission.Ruleset{
			{Permission: "shell", Pattern: "*", Action: permission.ActionAllow},
		},
	})
	RegisterBuiltins(r)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output string
	var isErr bool
	go func() {
		output, isErr, _ = r.Execute(ctx, "bash", json.RawMessage(`{"command":"touch .env"}`), "sess-secret")
		close(done)
	}()

	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "secret-shell" {
		t.Fatalf("permission %q, want secret-shell", req.Permission)
	}
	if err := perms.RespondToAsk(permission.ReplyInput{
		RequestID: req.ID,
		Reply:     permission.ReplyReject,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for rejected secret command")
	}
	if !isErr {
		t.Fatal("expected rejection to fail the command")
	}
	if output == "" {
		t.Fatal("expected rejection output")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected command created .env: %v", statErr)
	}
}

func TestExecuteShell_ShellEnvHookInjectsEnv(t *testing.T) {
	args := shellArgs{Command: "echo $FOO"}
	raw, _ := json.Marshal(args)

	tc := &Context{
		SessionID: "sess-env",
		Directory: t.TempDir(),
		ReadFiles: NewSafeReadFiles(),
		ShellEnvHook: func(sessionID, directory string, env map[string]string) map[string]string {
			if sessionID != "sess-env" {
				t.Errorf("expected sessionID sess-env, got %q", sessionID)
			}
			env["FOO"] = "bar"
			return env
		},
	}

	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if got := strings.TrimSpace(result.Output); got != "bar" {
		t.Errorf("expected FOO=bar in command output, got %q", got)
	}
}

func TestExecuteShell_TruncatesLargeOutput(t *testing.T) {
	// Generate >10MB of stdout via a shell command.
	// yes produces infinite output; head caps it at 11MB.
	args := shellArgs{Command: "yes AAAA | head -c 11534336"}
	raw, _ := json.Marshal(args)

	tc := &Context{Directory: t.TempDir(), ReadFiles: NewSafeReadFiles()}
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Output, "[output truncated at 10MB]") {
		t.Error("expected truncation notice in output")
	}

	// The captured content should be at most MaxOutputSize + truncation message.
	maxExpected := MaxOutputSize + 200 // allow for the truncation message itself
	if len(result.Output) > maxExpected {
		t.Errorf("output size %d exceeds expected cap %d", len(result.Output), maxExpected)
	}
}
