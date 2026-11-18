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

func TestIsDestructive_DetectsClusterMutations(t *testing.T) {
	destructive := []string{
		"oc delete pod api-1",
		"kubectl apply -f deploy.yaml",
		"oc scale deployment/api --replicas=0",
		"kubectl patch deploy/api -p '{}'",
		"oc adm drain node1",
		"oc adm cordon node1",
		"kubectl rollout restart deployment/api",
		"oc exec pod/api -- rm /tmp/x",
		"helm upgrade myrel ./chart",
		"sudo reboot",
	}
	for _, cmd := range destructive {
		if !IsDestructive(cmd) {
			t.Errorf("expected destructive for %q", cmd)
		}
	}
}

func TestIsDestructive_IgnoresReadOnlyClusterCommands(t *testing.T) {
	safe := []string{
		"oc get pods",
		"oc describe node/worker-1",
		"kubectl logs deploy/api",
		"oc adm top pods",
		"oc whoami",
		"helm list",
	}
	for _, cmd := range safe {
		if IsDestructive(cmd) {
			t.Errorf("expected safe for %q", cmd)
		}
	}
}

func TestIsDestructive_DetectsHostMutations(t *testing.T) {
	destructive := []string{
		"systemctl restart sshd",
		"systemctl --no-block stop firewalld",
		"ssh node systemctl restart sshd",
		"service nginx reload",
		"useradd deploy",
		"timedatectl set-ntp true",
		"setenforce 0",
		"firewall-cmd --add-port=22/tcp",
		"iptables -A INPUT -j DROP",
		"nft add rule inet filter input drop",
		"nmcli connection down eth0",
		"ip route add 10.0.0.0/8 via 10.1.1.1",
	}
	for _, cmd := range destructive {
		if !IsDestructive(cmd) {
			t.Errorf("expected destructive for %q", cmd)
		}
	}
}

func TestIsDestructive_IgnoresReadOnlyHostCommands(t *testing.T) {
	safe := []string{
		"hostname",
		"systemctl status sshd",
		"systemctl --failed",
		"ssh node systemctl status sshd",
		"service nginx status",
		"firewall-cmd --list-all",
		"iptables -L",
		"iptables -S",
		"nft list ruleset",
		"nmcli device status",
		"ip route",
		"getenforce",
	}
	for _, cmd := range safe {
		if IsDestructive(cmd) {
			t.Errorf("expected safe for %q", cmd)
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

func TestExecuteShell_HostMutationBlockedWithoutPerms(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	for _, cmd := range []string{
		"systemctl restart sshd",
		"ssh node systemctl restart sshd",
	} {
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		output, isErr, execErr := r.Execute(context.Background(), "bash", raw, "sess-host")
		if execErr != nil {
			t.Errorf("%s execute error: %v", cmd, execErr)
		}
		if !isErr {
			t.Errorf("%s should be blocked without a permission service", cmd)
		}
		if !strings.Contains(output, "destructive") {
			t.Errorf("%s output = %q, want a destructive block", cmd, output)
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
	perms.SetHasResponder(true)
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
	perms.SetHasResponder(true)
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

func TestExecuteShell_DropsCredentialEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "super-secret")
	args := shellArgs{Command: `printf '%s' "$OPENROUTER_API_KEY"`}
	raw, _ := json.Marshal(args)

	tc := &Context{Directory: t.TempDir(), ReadFiles: NewSafeReadFiles()}
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if strings.Contains(result.Output, "super-secret") {
		t.Fatalf("shell output leaked the API key: %q", result.Output)
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

func TestExecuteShell_WiderPermissionRequiresJustification(t *testing.T) {
	tc := &Context{Directory: t.TempDir(), ReadFiles: NewSafeReadFiles()}
	raw := json.RawMessage(`{"command":"echo hi","permission":"wider"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error when wider permission has no justification")
	}
	if !strings.Contains(result.Output, "justification is required") {
		t.Errorf("expected justification error, got %q", result.Output)
	}
}

func TestExecuteShell_WiderPermissionWithJustificationShowsOnAsk(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	perms.SetHasResponder(true)
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
	go func() {
		r.Execute(ctx, "bash", json.RawMessage(`{
			"command":"rm -rf /tmp/testdir",
			"permission":"wider",
			"justification":"Need to clean build artifacts"
		}`), "sess-wider")
		close(done)
	}()

	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "destructive-shell" {
		t.Fatalf("permission %q, want destructive-shell", req.Permission)
	}
	if req.Metadata["justification"] != "Need to clean build artifacts" {
		t.Errorf("justification = %v, want 'Need to clean build artifacts'", req.Metadata["justification"])
	}
	if req.Metadata["escalation"] != true {
		t.Errorf("escalation = %v, want true", req.Metadata["escalation"])
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
		t.Fatal("timed out")
	}
}

func TestExecuteShell_DefaultPermissionStillAsksForDestructive(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	perms.SetHasResponder(true)
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
	go func() {
		r.Execute(ctx, "bash", json.RawMessage(`{
			"command":"rm -rf /tmp/testdir",
			"permission":"default"
		}`), "sess-default")
		close(done)
	}()

	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "destructive-shell" {
		t.Fatalf("permission %q, want destructive-shell", req.Permission)
	}
	// Default permission should not carry justification or escalation
	if _, ok := req.Metadata["justification"]; ok {
		t.Error("default permission should not carry justification")
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
		t.Fatal("timed out")
	}
}

func TestExecuteShell_BackgroundReturnsMonitorID(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	tc := &Context{
		Directory:      t.TempDir(),
		ReadFiles:      NewSafeReadFiles(),
		MonitorManager: mm,
	}

	raw := json.RawMessage(`{"command":"sleep 10","background":true}`)
	start := time.Now()
	result, err := executeShell(context.Background(), tc, raw)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if elapsed > 2*time.Second {
		t.Errorf("background command blocked for %v, expected immediate return", elapsed)
	}
	if !strings.Contains(result.Output, "background") {
		t.Errorf("expected 'background' in output, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "mon_") {
		t.Errorf("expected monitor ID in output, got %q", result.Output)
	}

	monitors := mm.List()
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}
	if !monitors[0].Running {
		t.Error("expected monitor to be running")
	}
}

func TestExecuteShell_BackgroundNoMonitorManagerFails(t *testing.T) {
	tc := &Context{
		Directory: t.TempDir(),
		ReadFiles: NewSafeReadFiles(),
	}

	raw := json.RawMessage(`{"command":"echo hi","background":true}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when no monitor manager")
	}
	if !strings.Contains(result.Output, "not available") {
		t.Errorf("expected 'not available' in output, got %q", result.Output)
	}
}

func TestExecuteShell_TimeoutPromotesToBackground(t *testing.T) {
	mm := NewMonitorManager()
	defer mm.Shutdown()

	tc := &Context{
		Directory:      t.TempDir(),
		ReadFiles:      NewSafeReadFiles(),
		MonitorManager: mm,
	}

	// Use a very short timeout (500ms) so the sleep command triggers promotion.
	timeoutMS := 500
	raw, _ := json.Marshal(shellArgs{Command: "sleep 30", Timeout: &timeoutMS})
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful promotion, got error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "promoted to background") {
		t.Errorf("expected 'promoted to background' in output, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "mon_") {
		t.Errorf("expected monitor ID in output, got %q", result.Output)
	}

	// Verify the process is still running in the monitor.
	monitors := mm.List()
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor after promotion, got %d", len(monitors))
	}
	if !monitors[0].Running {
		t.Error("expected promoted process to still be running")
	}
}

func TestExecuteShell_TimeoutWithoutMonitorKills(t *testing.T) {
	tc := &Context{
		Directory: t.TempDir(),
		ReadFiles: NewSafeReadFiles(),
		// No MonitorManager — promotion should fail, process killed.
	}

	timeoutMS := 500
	raw, _ := json.Marshal(shellArgs{Command: "sleep 30", Timeout: &timeoutMS})
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected timeout error when no monitor manager")
	}
	if !strings.Contains(result.Output, "timed out") {
		t.Errorf("expected 'timed out' in output, got %q", result.Output)
	}
}

func TestExecuteShell_ForegroundStillWorks(t *testing.T) {
	tc := &Context{
		Directory:      t.TempDir(),
		ReadFiles:      NewSafeReadFiles(),
		MonitorManager: NewMonitorManager(),
	}
	defer tc.MonitorManager.Shutdown()

	raw := json.RawMessage(`{"command":"echo hello_world"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if got := strings.TrimSpace(result.Output); got != "hello_world" {
		t.Errorf("expected 'hello_world', got %q", got)
	}

	// No monitors should have been created.
	monitors := tc.MonitorManager.List()
	if len(monitors) != 0 {
		t.Errorf("expected 0 monitors for foreground command, got %d", len(monitors))
	}
}

func TestClassifyMutation(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantMut   bool
		wantPaths []string // nil means don't check paths
	}{
		{
			name:      "redirect_stdout_to_file",
			command:   "echo hi > out.txt",
			wantMut:   true,
			wantPaths: []string{"out.txt"},
		},
		{
			name:      "append_redirect_to_file",
			command:   "echo hi >> log.txt",
			wantMut:   true,
			wantPaths: []string{"log.txt"},
		},
		{
			name:      "stderr_redirect_2_ampersand_1",
			command:   "cat a 2>&1",
			wantMut:   false,
			wantPaths: nil,
		},
		{
			name:      "redirect_to_dev_null",
			command:   "echo hi > /dev/null",
			wantMut:   false,
			wantPaths: nil,
		},
		{
			name:      "tee_to_file",
			command:   "echo hi | tee output.txt",
			wantMut:   true,
			wantPaths: []string{"output.txt"},
		},
		{
			name:      "find_delete",
			command:   "find . -name x -delete",
			wantMut:   true,
			wantPaths: []string{"unknown"},
		},
		{
			name:      "sed_in_place",
			command:   "sed -i 's/a/b/' file.go",
			wantMut:   true,
			wantPaths: []string{"file.go"},
		},
		{
			name:      "sort_output",
			command:   "sort -o sorted.txt input.txt",
			wantMut:   true,
			wantPaths: []string{"sorted.txt"},
		},
		{
			name:      "safe_ls",
			command:   "ls -la",
			wantMut:   false,
			wantPaths: nil,
		},
		{
			name:      "safe_grep",
			command:   "grep foo file",
			wantMut:   false,
			wantPaths: nil,
		},
		{
			name:      "safe_cat",
			command:   "cat file",
			wantMut:   false,
			wantPaths: nil,
		},
		{
			name:      "pipe_with_redirect",
			command:   "echo hi | grep foo > result.txt",
			wantMut:   true,
			wantPaths: []string{"result.txt"},
		},
		{
			name:      "unparseable_subshell_path",
			command:   "$(cmd) > $(other)",
			wantMut:   true,
			wantPaths: []string{"unknown"},
		},
		{
			name:      "find_exec_rm",
			command:   "find /tmp -name '*.bak' -exec rm {} \\;",
			wantMut:   true,
			wantPaths: []string{"unknown"},
		},
		{
			name:      "tee_with_append_flag",
			command:   "echo data | tee -a log.txt",
			wantMut:   true,
			wantPaths: []string{"log.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMut, gotPaths := ClassifyMutation(tt.command)
			if gotMut != tt.wantMut {
				t.Errorf("ClassifyMutation(%q) mutation = %v, want %v", tt.command, gotMut, tt.wantMut)
			}
			if tt.wantPaths != nil {
				if len(gotPaths) != len(tt.wantPaths) {
					t.Fatalf("ClassifyMutation(%q) paths = %v, want %v", tt.command, gotPaths, tt.wantPaths)
				}
				for i, want := range tt.wantPaths {
					if gotPaths[i] != want {
						t.Errorf("ClassifyMutation(%q) path[%d] = %q, want %q", tt.command, i, gotPaths[i], want)
					}
				}
			}
		})
	}
}

func TestIsMutation(t *testing.T) {
	if IsMutation("ls -la") {
		t.Error("ls -la should not be a mutation")
	}
	if !IsMutation("echo hi > out.txt") {
		t.Error("echo hi > out.txt should be a mutation")
	}
}

func TestExecuteShell_MutationRecordsMetadata(t *testing.T) {
	tc := &Context{
		Directory: t.TempDir(),
		ReadFiles: NewSafeReadFiles(),
	}

	raw := json.RawMessage(`{"command":"echo test > mutation_test.txt"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if result.Metadata == nil {
		t.Fatal("expected metadata on mutation result")
	}
	paths, ok := result.Metadata["mutation_paths"].([]string)
	if !ok {
		t.Fatalf("mutation_paths not []string: %T", result.Metadata["mutation_paths"])
	}
	if len(paths) != 1 || paths[0] != "mutation_test.txt" {
		t.Errorf("mutation_paths = %v, want [mutation_test.txt]", paths)
	}
}

func TestExecuteShell_NonMutationHasNoMetadata(t *testing.T) {
	tc := &Context{
		Directory: t.TempDir(),
		ReadFiles: NewSafeReadFiles(),
	}

	raw := json.RawMessage(`{"command":"echo hello"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if result.Metadata != nil {
		t.Errorf("expected no metadata on non-mutation, got %v", result.Metadata)
	}
}

func TestExecuteShell_WiderPermissionOnSecretShowsEscalation(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	perms.SetHasResponder(true)
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
	go func() {
		r.Execute(ctx, "bash", json.RawMessage(`{
			"command":"cat .env",
			"permission":"wider",
			"justification":"Need to verify env vars for debugging"
		}`), "sess-secret-wider")
		close(done)
	}()

	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "secret-shell" {
		t.Fatalf("permission %q, want secret-shell", req.Permission)
	}
	if req.Metadata["justification"] != "Need to verify env vars for debugging" {
		t.Errorf("justification = %v, want debugging reason", req.Metadata["justification"])
	}
	if req.Metadata["escalation"] != true {
		t.Errorf("escalation = %v, want true", req.Metadata["escalation"])
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
		t.Fatal("timed out")
	}
}

func TestExecuteShell_WorkdirSubdirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Resolve symlinks (macOS /var -> /private/var) so pwd output matches.
	subResolved, _ := filepath.EvalSymlinks(sub)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	raw := json.RawMessage(`{"command":"pwd","workdir":"subdir"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	got := strings.TrimSpace(result.Output)
	if got != subResolved {
		t.Errorf("pwd = %q, want %q", got, subResolved)
	}
}

func TestExecuteShell_WorkdirMissing(t *testing.T) {
	dir := t.TempDir()
	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	raw := json.RawMessage(`{"command":"pwd","workdir":"nonexistent"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing workdir")
	}
	if !strings.Contains(result.Output, "does not exist") {
		t.Errorf("expected 'does not exist' in output, got %q", result.Output)
	}
}

func TestExecuteShell_NoWorkdirUsesSessionDir(t *testing.T) {
	dir := t.TempDir()
	dirResolved, _ := filepath.EvalSymlinks(dir)
	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	raw := json.RawMessage(`{"command":"pwd"}`)
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	got := strings.TrimSpace(result.Output)
	if got != dirResolved {
		t.Errorf("pwd = %q, want %q", got, dirResolved)
	}
}

func TestExecuteShell_WorkdirIsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "afile.txt")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	raw, _ := json.Marshal(map[string]string{"command": "pwd", "workdir": f})
	result, err := executeShell(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when workdir is a file")
	}
	if !strings.Contains(result.Output, "is not a directory") {
		t.Errorf("expected 'is not a directory' in output, got %q", result.Output)
	}
}
