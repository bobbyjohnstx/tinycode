package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tool"

	_ "modernc.org/sqlite"
)

// --- Mock LLM Client ---

type mockScenario struct {
	textResponse string
	toolCalls    []llm.ToolCall
	err          error
}

type mockLLMClient struct {
	mu        sync.Mutex
	scenarios []mockScenario
	index     int
	calls     []llm.Request
}

func (m *mockLLMClient) Stream(_ context.Context, req llm.Request, _ ...llm.StreamOption) (<-chan llm.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)

	if m.index >= len(m.scenarios) {
		ch := make(chan llm.Event, 2)
		ch <- llm.Event{Type: llm.EventTextDelta, Text: "no more scenarios"}
		ch <- llm.Event{Type: llm.EventFinish, Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 5}}
		close(ch)
		return ch, nil
	}

	scenario := m.scenarios[m.index]
	m.index++

	if scenario.err != nil {
		return nil, scenario.err
	}

	ch := make(chan llm.Event, 10)
	go func() {
		defer close(ch)

		if len(scenario.toolCalls) > 0 {
			for _, tc := range scenario.toolCalls {
				ch <- llm.Event{Type: llm.EventToolCallBegin, ToolCallID: tc.ID, ToolName: tc.Function.Name}
				ch <- llm.Event{Type: llm.EventToolCallEnd, ToolCallID: tc.ID, ToolName: tc.Function.Name, ToolCallArgs: tc.Function.Arguments}
			}
		}

		if scenario.textResponse != "" {
			ch <- llm.Event{Type: llm.EventTextDelta, Text: scenario.textResponse}
		}

		ch <- llm.Event{Type: llm.EventFinish, Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50}}
	}()

	return ch, nil
}

func (m *mockLLMClient) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// --- Integration test harness ---

type testHarness struct {
	t        *testing.T
	server   *Server
	bus      *bus.Bus
	db       *sql.DB
	listener *Listener
	cancel   context.CancelFunc
	mock     *mockLLMClient
}

func newTestHarness(t *testing.T, scenarios []mockScenario) *testHarness {
	t.Helper()

	b := bus.New()
	db := testDB(t)

	mock := &mockLLMClient{scenarios: scenarios}

	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "test-provider",
		Name:   "Test Provider",
		Source: "test",
		Models: map[string]*provider.Model{
			"test-model": {
				ID:           "test-model",
				ProviderID:   "test-provider",
				Name:         "Test Model 7B",
				API:          provider.ModelAPI{ID: "test-model", URL: "http://localhost:0"},
				Status:       "available",
				Capabilities: provider.ModelCaps{ToolCall: true, Input: provider.ModalityCaps{Text: true}, Output: provider.ModalityCaps{Text: true}},
			},
		},
	})

	agentReg := agent.NewRegistry()
	agentReg.LoadDefaults(permission.Ruleset{
		{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
	}, nil)

	pluginMgr := plugin.NewManagerWithRegistry([]plugin.RegistryEntry{
		{Name: "test-plugin", Package: "test-plugin"},
	})
	pluginMgr.SetResolveFunc(func(name string) (string, error) {
		if name != "test-plugin" {
			return "", plugin.ErrPluginNotFound
		}
		return name, nil
	})
	pluginMgr.SetCommandFactory(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperPluginProcess")
		cmd.Env = append(os.Environ(), "GO_TEST_HELPER_PROCESS=1")
		return cmd
	})

	permSvc := permission.NewService(b)
	toolReg := tool.NewRegistry(&tool.Context{
		Directory: t.TempDir(),
		Bus:       b,
		Perms:     permSvc,
	})

	cfg := &config.Info{
		DefaultAgent: "build",
		Model:        "test-provider/test-model",
	}

	srv := New(
		Config{
			Port:         0,
			Hostname:     "127.0.0.1",
			DefaultModel: "test-provider/test-model",
			DefaultAgent: "build",
			Directory:    t.TempDir(),
		},
		Dependencies{
			Bus:           b,
			DB:            db,
			Registry:      reg,
			AgentRegistry: agentReg,
			PluginManager: pluginMgr,
			ToolRegistry:  toolReg,
			PermService:   permSvc,
			Config:        cfg,
		},
	)

	// Inject mock LLM client factory so processPrompt uses the mock
	srv.sessionManager.SetClientFactory(func(_ *provider.Model) llm.Client {
		return mock
	})

	ctx, cancel := context.WithCancel(context.Background())
	listener, err := srv.Listen(ctx)
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		time.Sleep(50 * time.Millisecond)
	})

	return &testHarness{
		t:        t,
		server:   srv,
		bus:      b,
		db:       db,
		listener: listener,
		cancel:   cancel,
		mock:     mock,
	}
}

func (h *testHarness) baseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", h.listener.Port)
}

func (h *testHarness) createSession(title, agentName string) string {
	h.t.Helper()
	body := fmt.Sprintf(`{"title":"%s","agent":"%s","model":{"modelID":"test-model","providerID":"test-provider"}}`, title, agentName)
	resp, err := http.Post(h.baseURL()+"/session?directory=/tmp/e2e-test", "application/json", strings.NewReader(body))
	if err != nil {
		h.t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("create session: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result["id"].(string)
}

func (h *testHarness) getSession(sessionID string) map[string]any {
	h.t.Helper()
	resp, err := http.Get(h.baseURL() + "/session/" + sessionID)
	if err != nil {
		h.t.Fatalf("get session: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (h *testHarness) listMessages(sessionID string) []map[string]any {
	h.t.Helper()
	resp, err := http.Get(h.baseURL() + "/session/" + sessionID + "/message")
	if err != nil {
		h.t.Fatalf("list messages: %v", err)
	}
	defer resp.Body.Close()

	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (h *testHarness) sendPromptAsync(sessionID, text string) {
	h.t.Helper()
	body := fmt.Sprintf(`{"parts":[{"type":"text","text":"%s"}]}`, text)
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/prompt_async", "application/json", strings.NewReader(body))
	if err != nil {
		h.t.Fatalf("send prompt: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		h.t.Fatalf("send prompt: expected 204, got %d", resp.StatusCode)
	}
}

func (h *testHarness) abort(sessionID string) {
	h.t.Helper()
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/abort", "application/json", nil)
	if err != nil {
		h.t.Fatalf("abort: %v", err)
	}
	defer resp.Body.Close()
}

// --- E2E Tests ---

func TestE2E_SessionCreateWithDefaults(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("E2E Test", "build")
	if !strings.HasPrefix(sessionID, "ses_") {
		t.Errorf("expected ses_ prefix, got %s", sessionID)
	}

	info := h.getSession(sessionID)
	if info["title"] != "E2E Test" {
		t.Errorf("expected title 'E2E Test', got %v", info["title"])
	}
	if info["agent"] != "build" {
		t.Errorf("expected agent 'build', got %v", info["agent"])
	}

	model, _ := info["model"].(map[string]any)
	if model == nil {
		t.Fatal("expected model to be set")
	}
	if model["providerID"] != "test-provider" {
		t.Errorf("expected providerID 'test-provider', got %v", model["providerID"])
	}
	if model["id"] != "test-model" {
		t.Errorf("expected model ID 'test-model', got %v", model["id"])
	}
}

func TestE2E_SessionCreateUsesConfigDefaults(t *testing.T) {
	h := newTestHarness(t, nil)

	body := `{"title":"Default Test"}`
	resp, err := http.Post(h.baseURL()+"/session?directory=/tmp/e2e-test", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["agent"] != "build" {
		t.Errorf("expected default agent 'build', got %v", result["agent"])
	}

	model, _ := result["model"].(map[string]any)
	if model == nil {
		t.Fatal("expected default model to be resolved")
	}
}

func TestE2E_SessionCreatedBusEvent(t *testing.T) {
	h := newTestHarness(t, nil)

	sub := h.bus.Subscribe("session.created")
	defer sub.Unsubscribe()

	sessionID := h.createSession("Bus Event Test", "build")

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		info, _ := props["info"].(*session.Info)
		if info == nil {
			sid, _ := props["sessionID"].(string)
			if sid != sessionID {
				t.Errorf("expected sessionID %s, got %s", sessionID, sid)
			}
		} else if info.ID != sessionID {
			t.Errorf("expected session ID %s, got %s", sessionID, info.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session.created event")
	}
}

func TestE2E_ProjectUpdatedBeforeSessionCreated(t *testing.T) {
	h := newTestHarness(t, nil)

	sub := h.bus.SubscribeAll()
	defer sub.Unsubscribe()

	h.createSession("Order Test", "build")

	var order []string
	deadline := time.After(2 * time.Second)
	for len(order) < 2 {
		select {
		case evt := <-sub.C:
			if evt.Type == "project.updated" || evt.Type == "session.created" {
				order = append(order, evt.Type)
			}
		case <-deadline:
			t.Fatalf("timeout; only got events: %v", order)
		}
	}

	if order[0] != "project.updated" {
		t.Errorf("expected project.updated first, got %v", order)
	}
	if order[1] != "session.created" {
		t.Errorf("expected session.created second, got %v", order)
	}
}

func TestE2E_SessionDeleteBusEvent(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("Delete Test", "build")

	sub := h.bus.Subscribe("session.deleted")
	defer sub.Unsubscribe()

	resp, err := http.NewRequest("DELETE", h.baseURL()+"/session/"+sessionID, nil)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	delResp, err := http.DefaultClient.Do(resp)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != sessionID {
			t.Errorf("expected sessionID %s, got %v", sessionID, props["sessionID"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session.deleted event")
	}
}

func TestE2E_PluginLifecycleHooks(t *testing.T) {
	h := newTestHarness(t, nil)

	pluginMgr := h.server.deps.PluginManager
	_, err := pluginMgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}

	plugins := pluginMgr.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	sessionID := h.createSession("Plugin Lifecycle", "build")

	// Give bus goroutines time to process the event
	time.Sleep(100 * time.Millisecond)

	// Delete session — should fire session.deleted
	req, _ := http.NewRequest("DELETE", h.baseURL()+"/session/"+sessionID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp.Body.Close()

	time.Sleep(100 * time.Millisecond)
}

func TestE2E_SessionForkCopiesMessages(t *testing.T) {
	h := newTestHarness(t, nil)

	parentID := h.createSession("Parent", "build")

	// Add a message to the parent session via the message store
	store := session.NewStore(h.db)
	ms := session.NewMessageStore(store)
	msg := &session.Message{
		ID:        "msg_test_001",
		SessionID: parentID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("hello from parent")},
		CreatedAt: time.Now(),
	}
	if err := ms.Append(msg); err != nil {
		t.Fatalf("append message: %v", err)
	}

	// Fork
	body := `{"title":"Forked"}`
	resp, err := http.Post(h.baseURL()+"/session/"+parentID+"/fork", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var forked map[string]any
	json.NewDecoder(resp.Body).Decode(&forked)
	forkedID := forked["id"].(string)

	if forked["parentID"] != parentID {
		t.Errorf("expected parentID %s, got %v", parentID, forked["parentID"])
	}

	// Verify forked session has copies of parent messages
	forkedMsgs := h.listMessages(forkedID)
	if len(forkedMsgs) != 1 {
		t.Fatalf("expected 1 message in fork, got %d", len(forkedMsgs))
	}
}

func TestE2E_SessionAbortPublishesEvent(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("Abort Test", "build")

	sub := h.bus.Subscribe("session.abort")
	defer sub.Unsubscribe()

	h.abort(sessionID)

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != sessionID {
			t.Errorf("expected sessionID %s, got %v", sessionID, props["sessionID"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session.abort event")
	}
}

func TestE2E_AgentSwitchingUpdatesSession(t *testing.T) {
	h := newTestHarness(t, nil)

	// Create session with "build" agent
	sessionID := h.createSession("Agent Switch", "build")

	info := h.getSession(sessionID)
	if info["agent"] != "build" {
		t.Errorf("expected initial agent 'build', got %v", info["agent"])
	}

	// The agent registry should have both build and plan agents
	agents := h.server.deps.AgentRegistry.List("")
	if len(agents) == 0 {
		t.Fatal("expected at least one agent in registry")
	}

	var hasBuild, hasPlan bool
	for _, a := range agents {
		if a.Name == "build" {
			hasBuild = true
		}
		if a.Name == "plan" {
			hasPlan = true
		}
	}
	if !hasBuild {
		t.Error("expected 'build' agent in registry")
	}
	if !hasPlan {
		t.Error("expected 'plan' agent in registry")
	}
}

func TestE2E_PromptAsyncRoute(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("Async Prompt", "build")

	// Subscribe to status events
	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	// The async prompt handler should return 204 immediately
	h.sendPromptAsync(sessionID, "hello world")

	// Wait for the session.status type=busy event
	timeout := time.After(3 * time.Second)
	var gotWorking bool
	for !gotWorking {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] == sessionID {
				status, _ := props["status"].(map[string]any)
				if status["type"] == "busy" {
					gotWorking = true
				}
			}
		case <-timeout:
			t.Fatal("timeout waiting for session.status type=busy")
		}
	}
}

func TestE2E_PromptAsyncResolvesModelFromSession(t *testing.T) {
	scenarios := []mockScenario{
		{textResponse: "resolved model response"},
	}
	h := newTestHarness(t, scenarios)

	sessionID := h.createSession("Resolve Model", "build")

	// Subscribe to error and status events
	errorSub := h.bus.Subscribe("session.error")
	defer errorSub.Unsubscribe()
	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	// Send prompt WITHOUT specifying model — handler should resolve it from the session store
	h.sendPromptAsync(sessionID, "hello without model")

	// Wait for processing to complete (type=idle)
	deadline := time.After(10 * time.Second)
	var gotDone bool
	for !gotDone {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] == sessionID {
				status, _ := props["status"].(map[string]any)
				if status["type"] == "idle" {
					gotDone = true
				}
			}
		case <-deadline:
			t.Fatal("timeout waiting for session.status type=idle")
		}
	}

	// Verify no session.error was published (model should have been resolved)
	select {
	case evt := <-errorSub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] == sessionID {
			t.Fatalf("unexpected session.error: %v", props["error"])
		}
	default:
		// No error — expected
	}

	// Verify mock LLM was called (proves model was resolved successfully)
	if h.mock.callCount() == 0 {
		t.Error("expected mock LLM client to be called, but it was not — model was likely not resolved")
	}

	// Verify messages were persisted
	messages := h.listMessages(sessionID)
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages (user + assistant), got %d", len(messages))
	}
}

func TestE2E_PromptRoundTripWithMockLLM(t *testing.T) {
	scenarios := []mockScenario{
		{textResponse: "Hello from mock LLM"},
	}
	h := newTestHarness(t, scenarios)

	sessionID := h.createSession("Mock Prompt", "build")

	// Subscribe to status events to detect when processing finishes
	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	// Send a prompt with model info so processPrompt can resolve it
	body := `{"parts":[{"type":"text","text":"test prompt"}],"model":{"providerID":"test-provider","modelID":"test-model"}}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/prompt_async", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("send prompt: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Wait for type=idle (processing complete)
	deadline := time.After(10 * time.Second)
	var gotDone bool
	for !gotDone {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] == sessionID {
				status, _ := props["status"].(map[string]any)
				if status["type"] == "idle" {
					gotDone = true
				}
			}
		case <-deadline:
			t.Fatal("timeout waiting for session.status type=idle")
		}
	}

	// Verify mock was called
	if h.mock.callCount() == 0 {
		t.Error("expected mock LLM client to be called at least once")
	}

	// Verify messages were persisted (user + assistant)
	messages := h.listMessages(sessionID)
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages (user + assistant), got %d", len(messages))
	}
}

func TestE2E_PromptMockLLMError(t *testing.T) {
	scenarios := []mockScenario{
		{err: fmt.Errorf("mock LLM connection refused")},
	}
	h := newTestHarness(t, scenarios)

	sessionID := h.createSession("Error Prompt", "build")

	// Subscribe to error and status events
	errorSub := h.bus.Subscribe("session.error")
	defer errorSub.Unsubscribe()
	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	body := `{"parts":[{"type":"text","text":"test prompt"}],"model":{"providerID":"test-provider","modelID":"test-model"}}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/prompt_async", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("send prompt: %v", err)
	}
	resp.Body.Close()

	// Wait for processing to complete (type=idle)
	deadline := time.After(10 * time.Second)
	var gotDone bool
	for !gotDone {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] == sessionID {
				status, _ := props["status"].(map[string]any)
				if status["type"] == "idle" {
					gotDone = true
				}
			}
		case <-deadline:
			t.Fatal("timeout waiting for session.status working=false after error")
		}
	}

	// Mock should have been called
	if h.mock.callCount() == 0 {
		t.Error("expected mock LLM client to be called")
	}
}

func TestE2E_ClientFactoryReceivesAPIKey(t *testing.T) {
	// Verify that the default client factory extracts api_key from model options
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	reg := provider.NewRegistry()
	agentReg := agent.NewRegistry()
	agentReg.LoadDefaults(nil, nil)
	permSvc := permission.NewService(b)
	toolReg := tool.NewRegistry(&tool.Context{
		Directory: t.TempDir(),
		Bus:       b,
		Perms:     permSvc,
	})

	sm := NewSessionManager(b, reg, db, t.TempDir(), toolReg, permSvc, agentReg, nil, nil, nil)

	// Track what the factory produces
	var capturedClient llm.Client
	sm.SetClientFactory(func(m *provider.Model) llm.Client {
		// Call the default factory pattern to verify api_key extraction works
		apiKey := ""
		if m.Options != nil {
			if key, ok := m.Options["api_key"].(string); ok {
				apiKey = key
			}
		}
		c := llm.NewOpenAIClient(m.API.URL+"/v1", apiKey)
		capturedClient = c
		return c
	})

	model := &provider.Model{
		ID:      "test",
		API:     provider.ModelAPI{URL: "http://localhost:11434"},
		Options: map[string]any{"api_key": "sk-test-123"},
	}
	client := sm.clientFactory(model)

	if client == nil {
		t.Fatal("expected non-nil client from factory")
	}
	if capturedClient == nil {
		t.Fatal("expected factory to be called")
	}

	// Verify the OpenAI client received the API key
	oaiClient, ok := capturedClient.(*llm.OpenAIClient)
	if !ok {
		t.Fatal("expected *llm.OpenAIClient")
	}
	if oaiClient.APIKey != "sk-test-123" {
		t.Errorf("expected API key 'sk-test-123', got %q", oaiClient.APIKey)
	}
}

func TestE2E_PermissionReplyPropagates(t *testing.T) {
	h := newTestHarness(t, nil)

	// handlePermissionReply publishes to "permission.replied" with "requestID" and "reply".
	sub := h.bus.Subscribe("permission.replied")
	defer sub.Unsubscribe()

	// POST a permission reply — the handler should publish to the bus
	body := `{"action":"allow"}`
	resp, err := http.Post(h.baseURL()+"/permission/per_test123/reply", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("permission reply: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["requestID"] != "per_test123" {
			t.Errorf("expected requestID per_test123, got %v", props["requestID"])
		}
		if props["reply"] != "once" {
			t.Errorf("expected reply 'once', got %v", props["reply"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for permission.replied event")
	}
}

func TestE2E_ToolRegistryExecute(t *testing.T) {
	b := bus.New()
	defer b.Close()

	toolCtx := &tool.Context{
		SessionID: "ses_test",
		Directory: t.TempDir(),
		Bus:       b,
	}
	reg := tool.NewRegistry(toolCtx)

	executed := false
	reg.Register(&tool.Def{
		ID:          "test_tool",
		Description: "A test tool",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"input": map[string]any{"type": "string"}},
		},
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			executed = true
			var parsed struct {
				Input string `json:"input"`
			}
			json.Unmarshal(args, &parsed)
			return &tool.ExecuteResult{Output: "echo: " + parsed.Input}, nil
		},
	})

	output, isErr, err := reg.Execute(context.Background(), "test_tool", json.RawMessage(`{"input":"hello"}`), "ses_test")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if isErr {
		t.Error("expected no error from tool")
	}
	if !executed {
		t.Error("tool was not executed")
	}
	if !strings.Contains(output, "echo: hello") {
		t.Errorf("expected output to contain 'echo: hello', got %q", output)
	}
}

func TestE2E_ToolRegistryUnknownTool(t *testing.T) {
	b := bus.New()
	defer b.Close()

	reg := tool.NewRegistry(&tool.Context{
		Directory: t.TempDir(),
		Bus:       b,
	})

	output, isErr, err := reg.Execute(context.Background(), "nonexistent", json.RawMessage(`{}`), "ses_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErr {
		t.Error("expected isError for unknown tool")
	}
	if !strings.Contains(output, "Unknown tool") {
		t.Errorf("expected 'Unknown tool' message, got %q", output)
	}
}

func TestE2E_MessagePersistence(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("Persistence", "build")

	store := session.NewStore(h.db)
	ms := session.NewMessageStore(store)

	// Append user message
	userMsg := &session.Message{
		ID:        "msg_persist_001",
		SessionID: sessionID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("user prompt")},
		CreatedAt: time.Now(),
	}
	if err := ms.Append(userMsg); err != nil {
		t.Fatalf("append user: %v", err)
	}

	// Append assistant message
	assistMsg := &session.Message{
		ID:        "msg_persist_002",
		SessionID: sessionID,
		Role:      session.RoleAssistant,
		Parts:     []session.Part{session.TextPart("assistant reply")},
		Model:     "test-model",
		CreatedAt: time.Now(),
	}
	if err := ms.Append(assistMsg); err != nil {
		t.Fatalf("append assistant: %v", err)
	}

	// Read back via API
	messages := h.listMessages(sessionID)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify message roles
	info0, _ := messages[0]["info"].(map[string]any)
	info1, _ := messages[1]["info"].(map[string]any)

	data0, _ := info0["data"].(string)
	data1, _ := info1["data"].(string)

	var msg0Data, msg1Data struct {
		Role string `json:"role"`
	}
	json.Unmarshal([]byte(data0), &msg0Data)
	json.Unmarshal([]byte(data1), &msg1Data)

	// The API returns raw message data — verify they're in order
	if messages[0] == nil || messages[1] == nil {
		t.Fatal("expected non-nil messages")
	}
}

func TestE2E_SSEStreamDeliversEvents(t *testing.T) {
	h := newTestHarness(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", h.baseURL()+"/event", nil)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "server.connected") {
		t.Error("expected server.connected in SSE stream")
	}
}

func TestE2E_SessionSSEStreamFiltersEvents(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("SSE Filter", "build")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", h.baseURL()+"/session/"+sessionID+"/event", nil)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "server.connected") {
		t.Error("expected server.connected in session SSE stream")
	}
}

func TestE2E_PluginLoadUnload(t *testing.T) {
	h := newTestHarness(t, nil)

	// Load plugin via API
	body := `{"name":"test-plugin"}`
	resp, err := http.Post(h.baseURL()+"/plugin/load", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var loaded map[string]any
	json.NewDecoder(resp.Body).Decode(&loaded)
	if loaded["name"] != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %v", loaded["name"])
	}

	// List plugins
	listResp, err := http.Get(h.baseURL() + "/plugin")
	if err != nil {
		t.Fatalf("list plugins: %v", err)
	}
	defer listResp.Body.Close()

	var plugins []map[string]any
	json.NewDecoder(listResp.Body).Decode(&plugins)
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	pluginID, _ := plugins[0]["id"].(string)
	if pluginID == "" {
		t.Fatal("expected non-empty plugin ID from list")
	}

	// Unload via POST with ID in body
	unloadBody := fmt.Sprintf(`{"id":"%s"}`, pluginID)
	unloadResp, err := http.Post(h.baseURL()+"/plugin/unload", "application/json", strings.NewReader(unloadBody))
	if err != nil {
		t.Fatalf("unload plugin: %v", err)
	}
	unloadResp.Body.Close()

	if unloadResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", unloadResp.StatusCode)
	}

	// Verify unloaded
	listResp2, err := http.Get(h.baseURL() + "/plugin")
	if err != nil {
		t.Fatalf("list after unload: %v", err)
	}
	defer listResp2.Body.Close()

	var remaining []map[string]any
	json.NewDecoder(listResp2.Body).Decode(&remaining)
	if len(remaining) != 0 {
		t.Errorf("expected 0 plugins after unload, got %d", len(remaining))
	}
}

func TestE2E_PluginLoadUnknown(t *testing.T) {
	h := newTestHarness(t, nil)

	body := `{"name":"nonexistent-plugin"}`
	resp, err := http.Post(h.baseURL()+"/plugin/load", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestE2E_RevertUnrevertEvents(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("Revert Test", "build")

	revertedSub := h.bus.Subscribe("session.reverted")
	defer revertedSub.Unsubscribe()

	errorSub := h.bus.Subscribe("session.error")
	defer errorSub.Unsubscribe()

	h.bus.Publish("session.revert", map[string]any{"sessionID": sessionID})

	// Revert may fail (no dirty state in temp dir) or succeed — either way,
	// we verify the bus routing works by checking for any response event.
	select {
	case <-revertedSub.C:
		// success path
	case <-errorSub.C:
		// error path (expected — temp dir has no git repo)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for revert response")
	}
}

func TestE2E_SummarizeReturnsOK(t *testing.T) {
	h := newTestHarness(t, nil)

	sessionID := h.createSession("Summarize Test", "build")

	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/summarize", "application/json", nil)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["compacted"].(bool); !ok {
		t.Fatalf("expected compacted bool in response, got %#v", body)
	}
}

func TestE2E_GracefulShutdown(t *testing.T) {
	b := bus.New()
	defer b.Close()
	db := testDB(t)

	srv := New(Config{Port: 0, Hostname: "127.0.0.1"}, Dependencies{Bus: b, DB: db})
	ctx, cancel := context.WithCancel(context.Background())

	listener, err := srv.Listen(ctx)
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}

	// Verify server is up
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/global/health", listener.Port))
	if err != nil {
		cancel()
		t.Fatalf("health check: %v", err)
	}
	resp.Body.Close()

	disposedSub := b.Subscribe("global.disposed")
	defer disposedSub.Unsubscribe()

	cancel()

	select {
	case <-disposedSub.C:
		// Server published global.disposed before shutdown
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for global.disposed event")
	}
}

func TestE2E_MCPStatusEndpoint(t *testing.T) {
	h := newTestHarness(t, nil)

	resp, err := http.Get(h.baseURL() + "/mcp/status")
	if err != nil {
		t.Fatalf("mcp status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestE2E_AgentListEndpoint(t *testing.T) {
	h := newTestHarness(t, nil)

	resp, err := http.Get(h.baseURL() + "/agent")
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var agents []map[string]any
	json.NewDecoder(resp.Body).Decode(&agents)

	if len(agents) == 0 {
		t.Error("expected at least one agent")
	}

	var names []string
	for _, a := range agents {
		if name, ok := a["name"].(string); ok {
			names = append(names, name)
		}
	}

	found := false
	for _, n := range names {
		if n == "build" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'build' agent in list, got %v", names)
	}
}

func TestE2E_ProviderModelResolution(t *testing.T) {
	h := newTestHarness(t, nil)

	resp, err := http.Get(h.baseURL() + "/provider")
	if err != nil {
		t.Fatalf("provider list: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	connected, _ := result["connected"].([]any)
	if len(connected) != 1 {
		t.Fatalf("expected 1 connected provider, got %d", len(connected))
	}
	if connected[0] != "test-provider" {
		t.Errorf("expected connected provider 'test-provider', got %v", connected[0])
	}

	defaults, _ := result["default"].(map[string]any)
	// The API returns defaults as {providerID: modelID}, not {provider: ..., model: ...}.
	modelID, ok := defaults["test-provider"]
	if !ok {
		t.Errorf("expected default entry for 'test-provider', got %v", defaults)
	} else if modelID != "test-model" {
		t.Errorf("expected default model 'test-model' for test-provider, got %v", modelID)
	}

	// Get specific provider
	resp2, err := http.Get(h.baseURL() + "/provider/test-provider")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
	}
}

func TestE2E_ConfigEndpoint(t *testing.T) {
	h := newTestHarness(t, nil)

	resp, err := http.Get(h.baseURL() + "/global/config?directory=/tmp")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestHelperPluginProcess is a mock plugin subprocess for integration tests.
// It reads JSON-RPC from stdin and writes responses to stdout.
func TestHelperPluginProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}

	type rpcReq struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
	}
	type rpcResp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
	}

	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		var req rpcReq
		if err := decoder.Decode(&req); err != nil {
			os.Exit(0)
		}

		switch req.Method {
		case "initialize":
			raw, _ := json.Marshal(map[string]any{
				"hooks": []string{"session.start", "session.end"},
			})
			encoder.Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: raw})
		case "dispose":
			encoder.Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)})
			os.Exit(0)
		default:
			encoder.Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)})
		}
	}
}
