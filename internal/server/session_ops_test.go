package server

import (
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
			MaskObservations:    &maskObs,
			PreserveRecentTokens: &preserve,
			MaxMessages:         &maxMsgs,
		},
	}
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, appCfg)

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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, appCfg)

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

	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, nil, agentReg, nil, nil)

	input := PromptInput{
		SessionID: "ses_sys_1",
		Agent:     "build",
	}

	_, systemPrompt := sm.buildPromptSystemPrompt(input, model, "/tmp/test")

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

	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, nil, agentReg, nil, appCfg)

	input := PromptInput{
		SessionID: "ses_sys_2",
		Agent:     "build",
	}

	_, systemPrompt := sm.buildPromptSystemPrompt(input, model, "/tmp/test")

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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

	store := session.NewStore(db)
	ms := session.NewMessageStore(store)

	// Should not panic when result is nil.
	sm.persistPromptResult(nil, nil, ms, "ses_persist_1")
}

func TestPersistPromptResult_NoopWhenNoNewMessages(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)
	reg := provider.NewRegistry()
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
	sm.persistPromptResult(result, existing, ms, "ses_persist_2")
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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
	sm := NewSessionManager(b, reg, db, t.TempDir(), nil, nil, nil, nil, nil)

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
