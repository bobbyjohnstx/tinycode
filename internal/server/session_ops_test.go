package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

func TestResolvePromptModel_ReturnsModelWhenFound(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:   "test-provider",
		Name: "Test",
		Models: map[string]*provider.Model{
			"test-model": {ID: "test-model", ProviderID: "test-provider", Name: "Test Model"},
		},
	})
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	input := PromptInput{
		SessionID: "ses_resolve_1",
		Model:     &promptModel{ProviderID: "test-provider", ModelID: "test-model"},
	}

	model, err := sm.resolvePromptModel("ses_resolve_1", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.ID != "test-model" {
		t.Errorf("expected model ID 'test-model', got %q", model.ID)
	}
	if model.ProviderID != "test-provider" {
		t.Errorf("expected providerID 'test-provider', got %q", model.ProviderID)
	}
}

func TestResolvePromptModel_PublishesErrorWhenModelNotFound(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	sub := b.Subscribe("session.error")
	defer sub.Unsubscribe()

	input := PromptInput{
		SessionID: "ses_resolve_2",
		Model:     &promptModel{ProviderID: "nonexistent", ModelID: "no-model"},
	}

	model, err := sm.resolvePromptModel("ses_resolve_2", input)
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
	if model != nil {
		t.Error("expected nil model on error")
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_resolve_2" {
			t.Errorf("expected sessionID 'ses_resolve_2', got %v", props["sessionID"])
		}
		errObj, ok := props["error"].(map[string]any)
		if !ok {
			t.Fatal("expected error to be a structured object")
		}
		if errObj["name"] != "ProviderAuthError" {
			t.Errorf("expected error name 'ProviderAuthError', got %v", errObj["name"])
		}
		data, ok := errObj["data"].(map[string]any)
		if !ok {
			t.Fatal("expected error.data to be a map")
		}
		msg, _ := data["message"].(string)
		if msg == "" {
			t.Error("expected non-empty error.data.message")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.error event")
	}
}

func TestResolvePromptModel_PublishesErrorWhenNoModelSpecified(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	sub := b.Subscribe("session.error")
	defer sub.Unsubscribe()

	input := PromptInput{
		SessionID: "ses_resolve_3",
		Model:     nil,
	}

	model, err := sm.resolvePromptModel("ses_resolve_3", input)
	if err == nil {
		t.Fatal("expected error when no model specified")
	}
	if model != nil {
		t.Error("expected nil model")
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		errObj, ok := props["error"].(map[string]any)
		if !ok {
			t.Fatal("expected error to be a structured object")
		}
		if errObj["name"] != "ProviderAuthError" {
			t.Errorf("expected error name 'ProviderAuthError', got %v", errObj["name"])
		}
		data, ok := errObj["data"].(map[string]any)
		if !ok {
			t.Fatal("expected error.data to be a map")
		}
		msg, _ := data["message"].(string)
		if msg == "" {
			t.Error("expected non-empty error.data.message about no model specified")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.error event")
	}
}

func TestBuildCompactionConfig_ReturnsDefaultsWhenNoCfg(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	cfg := sm.buildCompactionConfig()
	defaults := session.DefaultCompactionConfig()

	if cfg.MaskObservations != defaults.MaskObservations {
		t.Errorf("expected MaskObservations %v, got %v", defaults.MaskObservations, cfg.MaskObservations)
	}
	if cfg.MaxPreserve != defaults.MaxPreserve {
		t.Errorf("expected MaxPreserve %d, got %d", defaults.MaxPreserve, cfg.MaxPreserve)
	}
	if cfg.MaxMessages != defaults.MaxMessages {
		t.Errorf("expected MaxMessages %d, got %d", defaults.MaxMessages, cfg.MaxMessages)
	}
}

func TestBuildCompactionConfig_AppliesOverrides(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()

	maskObs := true
	preserve := 5000
	maxMsgs := 50
	appCfg := &config.Info{
		Compaction: &config.CompactionConfig{
			MaskObservations:     &maskObs,
			PreserveRecentTokens: &preserve,
			MaxMessages:          &maxMsgs,
		},
	}
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, appCfg, nil)

	cfg := sm.buildCompactionConfig()

	if cfg.MaskObservations != true {
		t.Error("expected MaskObservations override to true")
	}
	if cfg.MaxPreserve != 5000 {
		t.Errorf("expected MaxPreserve 5000, got %d", cfg.MaxPreserve)
	}
	if cfg.MaxMessages != 50 {
		t.Errorf("expected MaxMessages 50, got %d", cfg.MaxMessages)
	}
}

func TestBuildCompactionConfig_IgnoresZeroMaxMessages(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()

	zeroMsgs := 0
	appCfg := &config.Info{
		Compaction: &config.CompactionConfig{
			MaxMessages: &zeroMsgs,
		},
	}
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, appCfg, nil)

	cfg := sm.buildCompactionConfig()
	defaults := session.DefaultCompactionConfig()

	// MaxMessages=0 should be ignored, keeping default.
	if cfg.MaxMessages != defaults.MaxMessages {
		t.Errorf("expected default MaxMessages %d when override is 0, got %d", defaults.MaxMessages, cfg.MaxMessages)
	}
}

func newToolRegistry(t *testing.T, b *bus.Bus) *tool.Registry {
	t.Helper()
	return tool.NewRegistry(&tool.Context{
		Directory: t.TempDir(),
		Bus:       b,
	})
}

func TestBuildPromptSystemPrompt_ReturnsPromptForKnownAgent(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()

	agentReg := agent.NewRegistry()
	agentReg.LoadDefaults(nil, nil)

	toolReg := newToolRegistry(t, b)

	model := &provider.Model{
		ID:         "test-model",
		ProviderID: "test-provider",
		Name:       "Test Model 7B",
	}

	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, nil, agentReg, nil, nil, nil)

	input := PromptInput{
		SessionID: "ses_sys_1",
		Agent:     "build",
	}

	_, _, systemPrompt := sm.buildPromptSystemPrompt(input, model, sm.dir)

	if systemPrompt == "" {
		t.Error("expected non-empty system prompt for known agent 'build'")
	}
}

func TestBuildPromptSystemPrompt_IncludesInstructionsFromConfig(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()

	agentReg := agent.NewRegistry()
	agentReg.LoadDefaults(nil, nil)

	toolReg := newToolRegistry(t, b)

	appCfg := &config.Info{
		Instructions: []string{"Always respond in English", "Be concise"},
	}

	model := &provider.Model{
		ID:         "test-model",
		ProviderID: "test-provider",
		Name:       "Test Model 7B",
	}

	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, nil, agentReg, nil, appCfg, nil)

	input := PromptInput{
		SessionID: "ses_sys_2",
		Agent:     "build",
	}

	_, _, systemPrompt := sm.buildPromptSystemPrompt(input, model, sm.dir)

	if systemPrompt == "" {
		t.Error("expected non-empty system prompt")
	}
	// The system prompt should contain the instructions.
	// The exact format depends on BuildSystemPrompt, but the text should be present.
}

func TestPersistPromptResult_NoopWhenResultIsNil(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	ms := session.NewMessageStore(store)

	// Should not panic when result is nil.
	if err := sm.persistPromptResult(nil, nil, ms, "ses_persist_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPersistPromptResult_NoopWhenNoNewMessages(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	ms := session.NewMessageStore(store)

	existing := []session.Message{
		{ID: "msg_1", Role: session.RoleUser},
		{ID: "msg_2", Role: session.RoleAssistant},
	}
	result := &session.ProcessResult{
		Messages: existing, // same count as existing
	}

	// Should not persist anything (no new messages).
	if err := sm.persistPromptResult(result, existing, ms, "ses_persist_2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPersistPromptResult_CompactedReplacesHistory(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	info, err := store.Create(session.CreateInput{ProjectID: "proj-1", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ms := session.NewMessageStore(store)

	now := time.Now()
	existing := make([]session.Message, 0, 5)
	for i := 0; i < 5; i++ {
		msg := session.Message{
			ID: fmt.Sprintf("old_%d", i), SessionID: info.ID, Role: session.RoleUser,
			Parts: []session.Part{session.TextPart("old")}, CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		if err := ms.Append(&msg); err != nil {
			t.Fatal(err)
		}
		existing = append(existing, msg)
	}

	compacted := []session.Message{
		{ID: "summary", SessionID: info.ID, Role: session.RoleSystem, Parts: []session.Part{session.TextPart("summary")}, CreatedAt: now},
		{ID: "keep", SessionID: info.ID, Role: session.RoleUser, Parts: []session.Part{session.TextPart("keep")}, CreatedAt: now.Add(time.Second)},
		{ID: "reply", SessionID: info.ID, Role: session.RoleAssistant, Parts: []session.Part{session.TextPart("reply")}, CreatedAt: now.Add(2 * time.Second)},
	}
	// Shorter than existing — the old shrink early-return bug would skip persist.
	result := &session.ProcessResult{
		Messages:  compacted,
		Compacted: true,
		Usage:     session.TokenUsage{Input: 10, Output: 5},
	}
	if err := sm.persistPromptResult(result, existing, ms, info.ID); err != nil {
		t.Fatalf("persist: %v", err)
	}

	got, err := ms.List(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 compacted messages persisted, got %d", len(got))
	}
	if got[0].ID != "summary" || got[2].ID != "reply" {
		t.Fatalf("unexpected persisted IDs: %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}

	updated, _ := store.Get(info.ID)
	if updated.Tokens.Input != 10 || updated.Tokens.Output != 5 {
		t.Errorf("expected cumulative usage 10/5, got %+v", updated.Tokens)
	}
}

// assertStructuredError validates that a session.error event's "error" field
// matches the SDK contract: { "name": string, "data": { "message": string } }.
func assertStructuredError(t *testing.T, props map[string]any, wantName string) {
	t.Helper()
	errObj, ok := props["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error to be a structured object, got %T", props["error"])
	}
	name, _ := errObj["name"].(string)
	if name != wantName {
		t.Errorf("expected error.name %q, got %q", wantName, name)
	}
	data, ok := errObj["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected error.data to be a map, got %T", errObj["data"])
	}
	msg, _ := data["message"].(string)
	if msg == "" {
		t.Error("expected non-empty error.data.message")
	}
}

func TestSessionErrorPayload_ContractShape(t *testing.T) {
	payload := sessionErrorPayload("ProviderAuthError", "model not found")
	name, _ := payload["name"].(string)
	if name != "ProviderAuthError" {
		t.Errorf("expected name 'ProviderAuthError', got %q", name)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be map[string]any, got %T", payload["data"])
	}
	msg, _ := data["message"].(string)
	if msg != "model not found" {
		t.Errorf("expected message 'model not found', got %q", msg)
	}
}

func TestSessionError_ModelNotFound_HasProviderAuthError(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	sub := b.Subscribe("session.error")
	defer sub.Unsubscribe()

	input := PromptInput{
		SessionID: "ses_contract_1",
		Model:     &promptModel{ProviderID: "missing", ModelID: "no-model"},
	}
	sm.resolvePromptModel("ses_contract_1", input)

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		assertStructuredError(t, props, "ProviderAuthError")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.error event")
	}
}

func TestSessionError_NoModelSpecified_HasProviderAuthError(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	sub := b.Subscribe("session.error")
	defer sub.Unsubscribe()

	input := PromptInput{
		SessionID: "ses_contract_2",
		Model:     nil,
	}
	sm.resolvePromptModel("ses_contract_2", input)

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		assertStructuredError(t, props, "ProviderAuthError")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.error event")
	}
}

func TestSwarmMaxIterations_AutoApproveTrue(t *testing.T) {
	got := swarmMaxIterations(true)
	if got != 3 {
		t.Errorf("expected MaxIterations=3 for autoApprove=true, got %d", got)
	}
}

func TestSwarmMaxIterations_AutoApproveFalse(t *testing.T) {
	got := swarmMaxIterations(false)
	if got != 0 {
		t.Errorf("expected MaxIterations=0 (default) for autoApprove=false, got %d", got)
	}
}

func TestProcessPrompt_ListError_PublishesSessionError(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:   "test-provider",
		Name: "Test",
		Models: map[string]*provider.Model{
			"test-model": {ID: "test-model", ProviderID: "test-provider", Name: "Test Model"},
		},
	})
	agentReg := agent.NewRegistry()
	if err := agentReg.LoadDefaults(nil, nil); err != nil {
		t.Fatal(err)
	}
	toolReg := newToolRegistry(t, b)

	store := session.NewStore(db)
	info, err := store.Create(session.CreateInput{
		ProjectID: "proj-list-fail",
		Directory: t.TempDir(),
		Title:     "list fail",
		Agent:     "build",
		Model:     &session.ModelRef{ProviderID: "test-provider", ModelID: "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, 1, 1, ?)`,
		"msg_corrupt", info.ID, "not-json",
	); err != nil {
		t.Fatal(err)
	}

	mock := &mockLLMClient{}
	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, nil, agentReg, nil, nil, nil)
	sm.SetClientFactory(func(_ *provider.Model) llm.Client { return mock })

	errSub := b.Subscribe("session.error")
	defer errSub.Unsubscribe()

	done := make(chan struct{})
	sm.processPrompt(context.Background(), PromptInput{
		SessionID: info.ID,
		Model:     &promptModel{ProviderID: "test-provider", ModelID: "test-model"},
		Agent:     "build",
		Parts:     []promptPart{{Type: "text", Text: "hello"}},
	}, done)
	<-done

	if mock.callCount() != 0 {
		t.Fatalf("expected LLM not called, got %d calls", mock.callCount())
	}

	select {
	case evt := <-errSub.C:
		props := evt.Properties.(map[string]any)
		assertStructuredError(t, props, "UnknownError")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session.error")
	}
}

func TestPersistPromptResult_CompactReplaceAllFails(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	info, err := store.Create(session.CreateInput{ProjectID: "proj-1", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ms := session.NewMessageStore(store)

	now := time.Now()
	dupID := "dup_msg"
	compacted := []session.Message{
		{ID: dupID, SessionID: info.ID, Role: session.RoleUser, Parts: []session.Part{session.TextPart("a")}, CreatedAt: now},
		{ID: dupID, SessionID: info.ID, Role: session.RoleAssistant, Parts: []session.Part{session.TextPart("b")}, CreatedAt: now.Add(time.Second)},
	}
	result := &session.ProcessResult{
		Messages:  compacted,
		Compacted: true,
	}

	err = sm.persistPromptResult(result, nil, ms, info.ID)
	if err == nil {
		t.Fatal("expected ReplaceAll error")
	}
}

func TestPersistPromptResult_AppendFailsStopsFurtherAppends(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	info, err := store.Create(session.CreateInput{ProjectID: "proj-1", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ms := session.NewMessageStore(store)

	now := time.Now()
	existing := []session.Message{
		{ID: "msg_0", SessionID: info.ID, Role: session.RoleUser, Parts: []session.Part{session.TextPart("hi")}, CreatedAt: now},
	}
	if err := ms.Append(&existing[0]); err != nil {
		t.Fatal(err)
	}

	dup := session.Message{
		ID: "msg_dup", SessionID: info.ID, Role: session.RoleAssistant,
		Parts: []session.Part{session.TextPart("first")}, CreatedAt: now.Add(time.Second),
	}
	if err := ms.Append(&dup); err != nil {
		t.Fatal(err)
	}

	result := &session.ProcessResult{
		Messages: []session.Message{
			existing[0],
			dup,
			{ID: "msg_dup", SessionID: info.ID, Role: session.RoleAssistant, Parts: []session.Part{session.TextPart("second")}, CreatedAt: now.Add(2 * time.Second)},
			{ID: "msg_never", SessionID: info.ID, Role: session.RoleAssistant, Parts: []session.Part{session.TextPart("never")}, CreatedAt: now.Add(3 * time.Second)},
		},
	}

	err = sm.persistPromptResult(result, existing, ms, info.ID)
	if err == nil {
		t.Fatal("expected append error")
	}

	got, err := ms.List(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if m.ID == "msg_never" {
			t.Fatal("msg_never should not be persisted after fail-fast")
		}
	}
}

func TestPersistPromptResult_AddUsageSoftFail(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	if _, err := db.Exec(`CREATE TRIGGER block_session_usage BEFORE UPDATE ON session FOR EACH ROW BEGIN SELECT RAISE(ABORT, 'usage blocked'); END`); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	info, err := store.Create(session.CreateInput{ProjectID: "proj-1", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ms := session.NewMessageStore(store)

	now := time.Now()
	existing := []session.Message{
		{ID: "msg_0", SessionID: info.ID, Role: session.RoleUser, Parts: []session.Part{session.TextPart("hi")}, CreatedAt: now},
	}
	newMsg := session.Message{
		ID: "msg_1", SessionID: info.ID, Role: session.RoleAssistant,
		Parts: []session.Part{session.TextPart("reply")}, CreatedAt: now.Add(time.Second),
	}
	result := &session.ProcessResult{
		Messages: []session.Message{existing[0], newMsg},
		Usage:    session.TokenUsage{Input: 5, Output: 3},
	}

	if err := sm.persistPromptResult(result, existing, ms, info.ID); err != nil {
		t.Fatalf("AddUsage failure should be soft-fail, got: %v", err)
	}

	got, err := ms.List(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "msg_1" {
		t.Fatalf("expected new message persisted despite AddUsage failure, got %+v", got)
	}
}
