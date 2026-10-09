package permission

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestWildcardMatch_ExactMatch(t *testing.T) {
	if !WildcardMatch("bash", "bash") {
		t.Error("expected exact match")
	}
}

func TestWildcardMatch_Star(t *testing.T) {
	if !WildcardMatch("bash", "*") {
		t.Error("expected * to match anything")
	}
}

func TestWildcardMatch_Prefix(t *testing.T) {
	if !WildcardMatch("bash foo bar", "bash *") {
		t.Error("expected prefix match with *")
	}
}

func TestWildcardMatch_QuestionMark(t *testing.T) {
	if !WildcardMatch("a", "?") {
		t.Error("expected ? to match single char")
	}
	if WildcardMatch("ab", "?") {
		t.Error("expected ? to match only single char")
	}
}

func TestWildcardMatch_TrailingSpaceStar(t *testing.T) {
	if !WildcardMatch("bash", "bash *") {
		t.Error("expected trailing ' *' to be optional")
	}
	if !WildcardMatch("bash --help", "bash *") {
		t.Error("expected trailing ' *' to match args")
	}
}

func TestWildcardMatch_NoMatch(t *testing.T) {
	if WildcardMatch("edit", "bash") {
		t.Error("expected no match")
	}
}

func TestWildcardMatch_BackslashNormalized(t *testing.T) {
	if !WildcardMatch("path\\to\\file", "path/to/*") {
		t.Error("expected backslashes normalized to forward slashes")
	}
}

func TestWildcardMatch_RegexSpecialChars(t *testing.T) {
	if !WildcardMatch("file.txt", "file.txt") {
		t.Error("expected dot to be escaped in pattern")
	}
	if WildcardMatch("filextxt", "file.txt") {
		t.Error("dot should be literal, not regex any")
	}
}

func TestEvaluate_LastWins(t *testing.T) {
	rules := Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionAllow},
		{Permission: "bash", Pattern: "*", Action: ActionDeny},
	}
	result := Evaluate("bash", "/tmp/foo", rules)
	if result.Action != ActionDeny {
		t.Errorf("expected deny (last-wins), got %s", result.Action)
	}
}

func TestEvaluate_DefaultAsk(t *testing.T) {
	result := Evaluate("bash", "/tmp/foo")
	if result.Action != ActionAsk {
		t.Errorf("expected ask default, got %s", result.Action)
	}
}

func TestEvaluate_MultipleRulesets(t *testing.T) {
	agent := Ruleset{{Permission: "bash", Pattern: "*", Action: ActionAllow}}
	config := Ruleset{{Permission: "bash", Pattern: "*.secret", Action: ActionDeny}}
	result := Evaluate("bash", "data.secret", agent, config)
	if result.Action != ActionDeny {
		t.Errorf("expected deny from config override, got %s", result.Action)
	}
}

func TestEvaluate_WildcardPermission(t *testing.T) {
	rules := Ruleset{{Permission: "*", Pattern: "*", Action: ActionAllow}}
	result := Evaluate("anything", "/any/path", rules)
	if result.Action != ActionAllow {
		t.Errorf("expected allow from wildcard permission, got %s", result.Action)
	}
}

func TestMerge_ConcatenatesRulesets(t *testing.T) {
	a := Ruleset{{Permission: "bash", Pattern: "*", Action: ActionAllow}}
	b := Ruleset{{Permission: "edit", Pattern: "*", Action: ActionDeny}}
	merged := Merge(a, b)
	if len(merged) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(merged))
	}
	if merged[0].Permission != "bash" || merged[1].Permission != "edit" {
		t.Error("merge should preserve order")
	}
}

func TestDisabled_DeniedTools(t *testing.T) {
	rules := Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionDeny},
		{Permission: "edit", Pattern: "*", Action: ActionDeny},
	}
	disabled := Disabled([]string{"bash", "edit", "write", "read"}, rules)
	if !disabled["bash"] {
		t.Error("bash should be disabled")
	}
	if !disabled["write"] {
		t.Error("write (maps to edit) should be disabled")
	}
	if disabled["read"] {
		t.Error("read should not be disabled")
	}
}

func TestDisabled_SpecificPatternNotDisabled(t *testing.T) {
	rules := Ruleset{{Permission: "bash", Pattern: "/specific/path", Action: ActionDeny}}
	disabled := Disabled([]string{"bash"}, rules)
	if disabled["bash"] {
		t.Error("specific-pattern deny should not disable the whole tool")
	}
}

func newTestBus() *bus.Bus {
	return bus.New()
}

func TestService_AskAllowed(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
		Always:     []string{"/tmp/foo"},
		Ruleset:    Ruleset{{Permission: "bash", Pattern: "*", Action: ActionAllow}},
	})
	if err != nil {
		t.Fatalf("expected nil error for allowed, got %v", err)
	}
}

func TestService_AskDenied(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
		Ruleset:    Ruleset{{Permission: "bash", Pattern: "*", Action: ActionDeny}},
	})
	var de *DeniedError
	if !errors.As(err, &de) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
}

func TestService_AskBlocksUntilReply(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	var askErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		askErr = svc.Ask(context.Background(), AskInput{
			SessionID:  "ses_001",
			Permission: "bash",
			Patterns:   []string{"/tmp/foo"},
			Metadata:   map[string]any{},
			Always:     []string{"/tmp/foo"},
			Ruleset:    Ruleset{},
		})
	}()

	// Wait for the ask to be pending
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	pending := svc.List()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}

	err := svc.RespondToAsk(ReplyInput{
		RequestID: pending[0].ID,
		Reply:     ReplyOnce,
	})
	if err != nil {
		t.Fatalf("reply failed: %v", err)
	}

	wg.Wait()
	if askErr != nil {
		t.Fatalf("expected nil after approval, got %v", askErr)
	}
}

func TestService_AskInterceptor_Denies(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	svc.SetAskInterceptor(func(req Request) error {
		cmd, _ := req.Metadata["command"].(string)
		if cmd == "rm -rf /" {
			return fmt.Errorf("%w: blocked by safety-net: dangerous pattern", ErrRejected)
		}
		return nil
	})

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "destructive-shell",
		Patterns:   []string{"rm -rf /"},
		Metadata:   map[string]any{"command": "rm -rf /"},
	})
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("expected ErrRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "blocked by safety-net") {
		t.Fatalf("expected reason in error, got %v", err)
	}
	if n := len(svc.List()); n != 0 {
		t.Fatalf("expected no pending after interceptor deny, got %d", n)
	}
}

func TestService_AskInterceptor_AllowsContinuesToUI(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	called := false
	svc.SetAskInterceptor(func(req Request) error {
		called = true
		return nil
	})

	var askErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		askErr = svc.Ask(context.Background(), AskInput{
			SessionID:  "ses_001",
			Permission: "bash",
			Patterns:   []string{"/tmp/foo"},
			Metadata:   map[string]any{},
			Always:     []string{"/tmp/foo"},
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	pending := svc.List()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending after interceptor allow, got %d", len(pending))
	}
	if !called {
		t.Fatal("expected interceptor to be called")
	}

	if err := svc.RespondToAsk(ReplyInput{RequestID: pending[0].ID, Reply: ReplyOnce}); err != nil {
		t.Fatalf("reply failed: %v", err)
	}
	wg.Wait()
	if askErr != nil {
		t.Fatalf("expected nil after approval, got %v", askErr)
	}
}

func TestService_AskInterceptor_SkippedWhenAllowed(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	called := false
	svc.SetAskInterceptor(func(req Request) error {
		called = true
		return fmt.Errorf("%w: should not run", ErrRejected)
	})

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Ruleset:    Ruleset{{Permission: "bash", Pattern: "*", Action: ActionAllow}},
	})
	if err != nil {
		t.Fatalf("expected allowed without interceptor, got %v", err)
	}
	if called {
		t.Fatal("interceptor must not run when rules already allow")
	}
}

func TestService_RejectCascadesSameSession(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	var errs [2]error
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.Ask(context.Background(), AskInput{
				SessionID:  "ses_001",
				Permission: "bash",
				Patterns:   []string{"/tmp/foo"},
				Metadata:   map[string]any{},
				Ruleset:    Ruleset{},
			})
		}(i)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	pending := svc.List()
	if len(pending) < 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}

	// Reject one — should cascade to the other
	err := svc.RespondToAsk(ReplyInput{
		RequestID: pending[0].ID,
		Reply:     ReplyReject,
	})
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}

	wg.Wait()

	for i, e := range errs {
		if e == nil {
			t.Errorf("ask %d: expected rejection error, got nil", i)
		}
	}

	if len(svc.List()) != 0 {
		t.Error("expected no pending after cascade rejection")
	}
}

func TestService_AlwaysAutoApprovesPending(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	var errs [2]error
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.Ask(context.Background(), AskInput{
				SessionID:  "ses_001",
				Permission: "bash",
				Patterns:   []string{"/tmp/foo"},
				Metadata:   map[string]any{},
				Always:     []string{"/tmp/foo"},
				Ruleset:    Ruleset{},
			})
		}(i)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	pending := svc.List()
	if len(pending) < 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}

	// "Always" approve one — should auto-approve the matching one
	err := svc.RespondToAsk(ReplyInput{
		RequestID: pending[0].ID,
		Reply:     ReplyAlways,
	})
	if err != nil {
		t.Fatalf("always reply failed: %v", err)
	}

	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("ask %d: expected nil after always approval, got %v", i, e)
		}
	}
}

func TestService_ReplyNotFound(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	err := svc.RespondToAsk(ReplyInput{
		RequestID: "per_nonexistent",
		Reply:     ReplyOnce,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_CorrectedError(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	var askErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		askErr = svc.Ask(context.Background(), AskInput{
			SessionID:  "ses_001",
			Permission: "bash",
			Patterns:   []string{"/tmp/foo"},
			Metadata:   map[string]any{},
			Ruleset:    Ruleset{},
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	pending := svc.List()
	err := svc.RespondToAsk(ReplyInput{
		RequestID: pending[0].ID,
		Reply:     ReplyReject,
		Message:   "use a different approach",
	})
	if err != nil {
		t.Fatalf("reject with message failed: %v", err)
	}

	wg.Wait()

	var ce *CorrectedError
	if !errors.As(askErr, &ce) {
		t.Fatalf("expected CorrectedError, got %v", askErr)
	}
	if ce.Feedback != "use a different approach" {
		t.Errorf("expected feedback message, got %s", ce.Feedback)
	}
}

func TestService_ContextCancellation(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := svc.Ask(ctx, AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
		Ruleset:    Ruleset{},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestService_Close(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	var askErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		askErr = svc.Ask(context.Background(), AskInput{
			SessionID:  "ses_001",
			Permission: "bash",
			Patterns:   []string{"/tmp/foo"},
			Metadata:   map[string]any{},
			Ruleset:    Ruleset{},
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	svc.Close()
	wg.Wait()

	if !errors.Is(askErr, ErrRejected) {
		t.Errorf("expected ErrRejected after close, got %v", askErr)
	}
}

func TestWildcardMatch_QuestionMarkNoMatchEmpty(t *testing.T) {
	if WildcardMatch("a", "a?") {
		t.Error("expected a? not to match single char 'a' (? requires one more char)")
	}
}

func TestWildcardMatch_EnvPattern(t *testing.T) {
	tests := []struct {
		input   string
		pattern string
		want    bool
	}{
		{".env", ".env*", true},
		{".env.local", ".env*", true},
		{".envrc", ".env*", true},
		{"env", ".env*", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := WildcardMatch(tt.input, tt.pattern)
			if got != tt.want {
				t.Errorf("WildcardMatch(%q, %q) = %v, want %v", tt.input, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestWildcardMatch_EmptyInputAndPattern(t *testing.T) {
	if !WildcardMatch("", "") {
		t.Error("expected empty input to match empty pattern")
	}
}

func TestWildcardMatch_PathWildcardMiddle(t *testing.T) {
	if !WildcardMatch("/home/user/file.txt", "/home/*/file.txt") {
		t.Error("expected wildcard in middle of path to match")
	}
	if WildcardMatch("/home/user/other.txt", "/home/*/file.txt") {
		t.Error("expected non-matching filename to fail")
	}
}

func TestEvaluate_ReadEnvExact(t *testing.T) {
	result := Evaluate("read", ".env")
	if result.Action != ActionAsk {
		t.Errorf("expected .env to match .env* ask rule, got %s", result.Action)
	}
}

func TestEvaluate_WriteNoDefaultRule(t *testing.T) {
	result := Evaluate("write", "/some/file.go")
	if result.Action != ActionAsk {
		t.Errorf("expected write with no rules to default to ask, got %s", result.Action)
	}
}

func TestEvaluate_CustomWriteAllow(t *testing.T) {
	rules := Ruleset{{Permission: "write", Pattern: "*", Action: ActionAllow}}
	result := Evaluate("write", "/any/path", rules)
	if result.Action != ActionAllow {
		t.Errorf("expected custom write allow, got %s", result.Action)
	}
}

func TestEvaluate_CustomOverridesEnvDefault(t *testing.T) {
	rules := Ruleset{{Permission: "read", Pattern: ".env*", Action: ActionAllow}}
	result := Evaluate("read", ".env.local", rules)
	if result.Action != ActionAllow {
		t.Errorf("expected custom rule to override .env* ask default, got %s", result.Action)
	}
}

func TestEvaluate_UnknownPermissionFallback(t *testing.T) {
	result := Evaluate("completely_unknown", "/any/path")
	if result.Action != ActionAsk {
		t.Errorf("expected unknown permission to default to ask, got %s", result.Action)
	}
	if result.Permission != "completely_unknown" {
		t.Errorf("expected fallback to preserve permission, got %s", result.Permission)
	}
}

func TestDefaultRules_Structure(t *testing.T) {
	if len(DefaultRules) != 4 {
		t.Fatalf("expected 4 default rules, got %d", len(DefaultRules))
	}

	expected := []struct {
		permission string
		pattern    string
		action     Action
	}{
		{"read", "*", ActionAllow},
		{"read", ".env*", ActionAsk},
		{"webfetch", "*", ActionAsk},
		{"external_directory", "*", ActionAsk},
	}

	for i, want := range expected {
		got := DefaultRules[i]
		if got.Permission != want.permission || got.Pattern != want.pattern || got.Action != want.action {
			t.Errorf("DefaultRules[%d] = {%s, %s, %s}, want {%s, %s, %s}",
				i, got.Permission, got.Pattern, got.Action, want.permission, want.pattern, want.action)
		}
	}
}

func TestMerge_Empty(t *testing.T) {
	merged := Merge()
	if len(merged) != 0 {
		t.Errorf("expected empty merge to return 0 rules, got %d", len(merged))
	}
}

func TestMerge_PreservesOrder(t *testing.T) {
	a := Ruleset{
		{Permission: "first", Pattern: "*", Action: ActionAllow},
		{Permission: "second", Pattern: "*", Action: ActionDeny},
	}
	b := Ruleset{
		{Permission: "third", Pattern: "*", Action: ActionAsk},
	}
	merged := Merge(a, b)
	if len(merged) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(merged))
	}
	if merged[0].Permission != "first" || merged[1].Permission != "second" || merged[2].Permission != "third" {
		t.Error("merge did not preserve rule order across rulesets")
	}
}

func TestDisabled_EditAliases(t *testing.T) {
	rules := Ruleset{{Permission: "edit", Pattern: "*", Action: ActionDeny}}
	aliases := []string{"edit", "write", "apply_patch", "todowrite"}
	disabled := Disabled(aliases, rules)
	for _, tool := range aliases {
		if !disabled[tool] {
			t.Errorf("expected %s to be disabled (maps to edit permission)", tool)
		}
	}
}

func TestDisabled_BashShellAlias(t *testing.T) {
	rules := Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "bash", Pattern: "*", Action: ActionAllow},
	}
	disabled := Disabled([]string{"bash", "read"}, rules)
	if disabled["bash"] {
		t.Error("bash should be allowed when bash is allowed")
	}
	if !disabled["read"] {
		t.Error("read should remain denied")
	}
}

func TestDisabled_ListGlobAlias(t *testing.T) {
	rules := Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "list", Pattern: "*", Action: ActionAllow},
	}
	disabled := Disabled([]string{"glob", "read"}, rules)
	if disabled["glob"] {
		t.Error("glob should be allowed when list is allowed")
	}
}

func TestDisabled_EmptyRuleset(t *testing.T) {
	disabled := Disabled([]string{"bash", "edit", "read"}, Ruleset{})
	if len(disabled) != 0 {
		t.Errorf("expected no disabled tools with empty ruleset, got %d", len(disabled))
	}
}

func TestVisible_AskKeepsTool(t *testing.T) {
	rules := Ruleset{
		{Permission: "read", Pattern: "*", Action: ActionAllow},
		{Permission: "read", Pattern: ".env*", Action: ActionAsk},
		{Permission: "webfetch", Pattern: "*", Action: ActionAsk},
		{Permission: "edit", Pattern: "*", Action: ActionDeny},
	}
	got := map[string]bool{}
	for _, perm := range Visible(rules) {
		got[perm] = true
	}
	if !got["read"] {
		t.Error("read ask for .env should keep the read tool available")
	}
	if !got["webfetch"] {
		t.Error("webfetch ask should keep the webfetch tool available")
	}
	if got["edit"] {
		t.Error("edit deny should hide the edit tool")
	}
	if got["*"] {
		t.Error("wildcard should not be offered")
	}
}

func TestDefaultRules_ReadAllowed(t *testing.T) {
	result := Evaluate("read", "/some/file.go")
	if result.Action != ActionAllow {
		t.Errorf("expected read to be allowed by default rules, got %s", result.Action)
	}
}

func TestDefaultRules_ReadEnvAsk(t *testing.T) {
	result := Evaluate("read", ".env.local")
	if result.Action != ActionAsk {
		t.Errorf("expected read .env* to be ask, got %s", result.Action)
	}
}

func TestDefaultRules_ExternalDirectoryAsk(t *testing.T) {
	result := Evaluate("external_directory", "/usr/local")
	if result.Action != ActionAsk {
		t.Errorf("expected external_directory to be ask, got %s", result.Action)
	}
}

func TestDefaultRules_OverriddenByUserRules(t *testing.T) {
	// User rule denying read should override default allow
	userRules := Ruleset{{Permission: "read", Pattern: "*", Action: ActionDeny}}
	result := Evaluate("read", "/some/file", userRules)
	if result.Action != ActionDeny {
		t.Errorf("expected user deny to override default allow, got %s", result.Action)
	}
}

// mockRuleStore implements RuleStore for testing.
type mockRuleStore struct {
	saved map[string]Ruleset
}

func newMockRuleStore() *mockRuleStore {
	return &mockRuleStore{saved: make(map[string]Ruleset)}
}

func (m *mockRuleStore) SaveRules(projectID string, rules Ruleset) error {
	m.saved[projectID] = append(Ruleset{}, rules...)
	return nil
}

func (m *mockRuleStore) LoadRules(projectID string) (Ruleset, error) {
	return m.saved[projectID], nil
}

func TestService_AlwaysPersistsToStore(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	store := newMockRuleStore()
	svc.SetStore(store, "proj_test")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		svc.Ask(context.Background(), AskInput{
			SessionID:  "ses_001",
			Permission: "bash",
			Patterns:   []string{"/tmp/foo"},
			Metadata:   map[string]any{},
			Always:     []string{"/tmp/foo"},
			Ruleset:    Ruleset{},
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.List()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	pending := svc.List()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}

	err := svc.RespondToAsk(ReplyInput{
		RequestID: pending[0].ID,
		Reply:     ReplyAlways,
	})
	if err != nil {
		t.Fatalf("always reply failed: %v", err)
	}

	wg.Wait()

	// Verify rules were persisted
	saved := store.saved["proj_test"]
	if len(saved) != 1 {
		t.Fatalf("expected 1 persisted rule, got %d", len(saved))
	}
	if saved[0].Permission != "bash" || saved[0].Pattern != "/tmp/foo" || saved[0].Action != ActionAllow {
		t.Errorf("unexpected persisted rule: %+v", saved[0])
	}
}

func TestService_SetStoreLoadsExistingRules(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	store := newMockRuleStore()
	store.saved["proj_test"] = Ruleset{
		{Permission: "bash", Pattern: "/tmp/*", Action: ActionAllow},
	}

	svc.SetStore(store, "proj_test")

	// The loaded rule should auto-allow matching asks
	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
		Ruleset:    Ruleset{},
	})
	if err != nil {
		t.Fatalf("expected nil (auto-allowed by loaded rules), got %v", err)
	}
}

func TestFromConfig_AllowDeny(t *testing.T) {
	rules := FromConfig(
		[]string{"bash *", "read"},
		[]string{"edit /etc/*"},
	)
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].Permission != "bash" || rules[0].Pattern != "*" || rules[0].Action != ActionAllow {
		t.Errorf("unexpected first rule: %+v", rules[0])
	}
	if rules[1].Permission != "read" || rules[1].Pattern != "*" || rules[1].Action != ActionAllow {
		t.Errorf("unexpected second rule: %+v", rules[1])
	}
	if rules[2].Permission != "edit" || rules[2].Pattern != "/etc/*" || rules[2].Action != ActionDeny {
		t.Errorf("unexpected third rule: %+v", rules[2])
	}
}

func TestService_SetBaseRules_AllowsWithoutBlocking(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	svc.SetBaseRules(Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionAllow},
	})

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected nil (allowed by base rules), got %v", err)
	}
}

func TestService_SetBaseRules_DeniesWithError(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	svc.SetBaseRules(Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionDeny},
	})

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
	})
	var de *DeniedError
	if !errors.As(err, &de) {
		t.Fatalf("expected DeniedError from base rules deny, got %v", err)
	}
}

func TestService_BaseRulesOverriddenByAgentRules(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	// Base rules deny bash
	svc.SetBaseRules(Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionDeny},
	})

	// Agent rules (via AskInput.Ruleset) allow bash — should override base
	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
		Ruleset:    Ruleset{{Permission: "bash", Pattern: "*", Action: ActionAllow}},
	})
	if err != nil {
		t.Fatalf("expected nil (agent rules override base deny), got %v", err)
	}
}

func TestService_BaseRulesOverriddenByAlwaysApproved(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	// Base rules deny bash
	svc.SetBaseRules(Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionDeny},
	})

	// Simulate a prior "always" approval by loading into the store
	store := newMockRuleStore()
	store.saved["proj_test"] = Ruleset{
		{Permission: "bash", Pattern: "*", Action: ActionAllow},
	}
	svc.SetStore(store, "proj_test")

	// "Always" approved rules should override base deny
	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "bash",
		Patterns:   []string{"/tmp/foo"},
		Metadata:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected nil (always-approved overrides base deny), got %v", err)
	}
}

func TestEvaluate_BashShellAlias_ExploreLike(t *testing.T) {
	// Explore-like ruleset allows bash; tool Ask uses permission "shell".
	rules := Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "bash", Pattern: "*", Action: ActionAllow},
	}
	result := Evaluate("shell", "bash", rules)
	if result.Action != ActionAllow {
		t.Errorf("expected shell ask to match allow bash *, got %s", result.Action)
	}

	// Reverse: rule says shell, ask uses bash.
	rules = Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "shell", Pattern: "*", Action: ActionAllow},
	}
	result = Evaluate("bash", "ls", rules)
	if result.Action != ActionAllow {
		t.Errorf("expected bash ask to match allow shell *, got %s", result.Action)
	}
}

func TestEvaluate_ListGlobAlias(t *testing.T) {
	rules := Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "list", Pattern: "*", Action: ActionAllow},
	}
	result := Evaluate("glob", "*", rules)
	if result.Action != ActionAllow {
		t.Errorf("expected glob to match allow list *, got %s", result.Action)
	}
}

func TestService_Ask_EmptyPatterns_FailClosed(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "read",
		Patterns:   nil,
		Ruleset:    Ruleset{{Permission: "read", Pattern: "*", Action: ActionAllow}},
	})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("expected ErrDenied for empty patterns, got %v", err)
	}
}

func TestService_Ask_ShellAliasWithBashRule(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	err := svc.Ask(context.Background(), AskInput{
		SessionID:  "ses_001",
		Permission: "shell",
		Patterns:   []string{"bash"},
		Ruleset: Ruleset{
			{Permission: "*", Pattern: "*", Action: ActionDeny},
			{Permission: "bash", Pattern: "*", Action: ActionAllow},
		},
	})
	if err != nil {
		t.Fatalf("expected allow via bash↔shell alias, got %v", err)
	}
}

func TestService_Ask_ReadEnvWithDefaultRules(t *testing.T) {
	b := newTestBus()
	defer b.Close()
	svc := NewService(b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- svc.Ask(ctx, AskInput{
			SessionID:  "ses_001",
			Permission: "read",
			Patterns:   []string{".env"},
		})
	}()

	// DefaultRules mark read .env* as ask — should block, not allow.
	select {
	case err := <-done:
		t.Fatalf("expected Ask to block for .env, got immediate result: %v", err)
	case <-time.After(50 * time.Millisecond):
		cancel()
	}
}
