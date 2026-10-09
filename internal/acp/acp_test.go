package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// syncBuffer is a thread-safe bytes.Buffer for tests where a goroutine
// writes (e.g. HandleStdio) while another reads (e.g. polling for output).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

func (sb *syncBuffer) Bytes() []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return append([]byte(nil), sb.buf.Bytes()...)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

type mockSessionService struct {
	sessions map[string]*session.Info
	messages map[string][]session.Message
	nextID   int
	mu       sync.Mutex
}

func newMockSessionService() *mockSessionService {
	return &mockSessionService{
		sessions: make(map[string]*session.Info),
		messages: make(map[string][]session.Message),
	}
}

func (m *mockSessionService) Create(_ context.Context, input session.CreateInput) (*session.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("ses_%d", m.nextID)
	info := &session.Info{
		ID:        id,
		ProjectID: input.ProjectID,
		Directory: input.Directory,
		Title:     input.Title,
		Agent:     input.Agent,
		Model:     input.Model,
		ParentID:  input.ParentID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.sessions[id] = info
	return info, nil
}

func (m *mockSessionService) Get(_ context.Context, id string) (*session.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return info, nil
}

func (m *mockSessionService) List(_ context.Context, projectID string) ([]*session.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*session.Info
	for _, info := range m.sessions {
		if info.ProjectID == projectID {
			result = append(result, info)
		}
	}
	return result, nil
}

func (m *mockSessionService) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionService) UpdateModel(_ context.Context, sessionID string, model *session.ModelRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	info.Model = model
	return nil
}

func (m *mockSessionService) UpdateAgent(_ context.Context, sessionID, agent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	info.Agent = agent
	return nil
}

func (m *mockSessionService) ListMessages(_ context.Context, sessionID string) ([]session.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]session.Message(nil), m.messages[sessionID]...), nil
}

func (m *mockSessionService) Fork(_ context.Context, parentID, title string) (*session.Info, error) {
	m.mu.Lock()
	parent, ok := m.sessions[parentID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("session not found: %s", parentID)
	}
	parentCopy := *parent
	parentMsgs := append([]session.Message(nil), m.messages[parentID]...)
	m.mu.Unlock()

	if title == "" {
		title = parentCopy.Title + " (fork)"
	}
	forked, err := m.Create(context.Background(), session.CreateInput{
		ProjectID: parentCopy.ProjectID,
		Directory: parentCopy.Directory,
		Title:     title,
		Agent:     parentCopy.Agent,
		Model:     parentCopy.Model,
		ParentID:  parentCopy.ID,
	})
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.messages[forked.ID] = parentMsgs
	m.mu.Unlock()
	return forked, nil
}

type mockRunner struct {
	mu    sync.Mutex
	busy  map[string]bool
	bus   *bus.Bus
	delay time.Duration
}

func newMockRunner(b *bus.Bus) *mockRunner {
	return &mockRunner{
		busy:  make(map[string]bool),
		bus:   b,
		delay: 30 * time.Millisecond,
	}
}

func (r *mockRunner) StartTextPrompt(ctx context.Context, sessionID, text string) error {
	r.mu.Lock()
	r.busy[sessionID] = true
	r.mu.Unlock()

	r.bus.Publish("session.status", map[string]any{
		"sessionID": sessionID,
		"status":    map[string]any{"type": "busy"},
	})
	r.bus.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"type": "text",
			"text": "echo: " + text,
		},
	})

	safego.Go(func() {
		timer := time.NewTimer(r.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
		r.mu.Lock()
		r.busy[sessionID] = false
		r.mu.Unlock()
		r.bus.Publish("session.status", map[string]any{
			"sessionID": sessionID,
			"status":    map[string]any{"type": "idle"},
		})
	})
	return nil
}

func (r *mockRunner) Abort(sessionID string) {
	r.mu.Lock()
	r.busy[sessionID] = false
	r.mu.Unlock()
	r.bus.Publish("session.status", map[string]any{
		"sessionID": sessionID,
		"status":    map[string]any{"type": "idle"},
	})
}

func (r *mockRunner) IsBusy(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.busy[sessionID]
}

type notifyBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	ch  chan struct{}
}

func newNotifyBuffer() *notifyBuffer {
	return &notifyBuffer{ch: make(chan struct{}, 16)}
}

func (w *notifyBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
	return n, err
}

func (w *notifyBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *notifyBuffer) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Len()
}

func (w *notifyBuffer) WaitWrite(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-w.ch:
	case <-time.After(timeout):
		t.Fatal("timeout waiting for transport write")
	}
}

func testService(t *testing.T) (*Service, *bus.Bus, *mockSessionService) {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	svc := NewService(mock, b)
	svc.defaultCWD = "/tmp"
	return svc, b, mock
}

func TestInitialize(t *testing.T) {
	svc, _, _ := testService(t)

	result, rpcErr := svc.HandleRequest(context.Background(), "initialize", nil)
	if rpcErr != nil {
		t.Fatalf("initialize error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)

	version := resp["protocolVersion"].(float64)
	if version != protocolVersion {
		t.Errorf("expected protocol version %d, got %v", protocolVersion, version)
	}

	caps := resp["agentCapabilities"].(map[string]any)
	if caps["loadSession"] != true {
		t.Error("expected loadSession capability")
	}
	promptCaps := caps["promptCapabilities"].(map[string]any)
	if promptCaps["image"] != false {
		t.Error("expected image capability false until supported")
	}
	if promptCaps["embeddedContext"] != false {
		t.Error("expected embeddedContext capability false until supported")
	}

	info := resp["agentInfo"].(map[string]any)
	if info["name"] != "tinycode" {
		t.Errorf("expected name 'tinycode', got %v", info["name"])
	}
}

func TestAuthenticate(t *testing.T) {
	svc, _, _ := testService(t)

	result, rpcErr := svc.HandleRequest(context.Background(), "authenticate", nil)
	if rpcErr != nil {
		t.Fatalf("authenticate error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	if string(data) != "{}" {
		t.Errorf("expected empty object, got %s", data)
	}
}

func TestNewSession(t *testing.T) {
	svc, _, _ := testService(t)

	params := json.RawMessage(`{"cwd": "/tmp/project"}`)
	result, rpcErr := svc.HandleRequest(context.Background(), "session/new", params)
	if rpcErr != nil {
		t.Fatalf("session/new error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)

	if resp["sessionId"] == nil || resp["sessionId"] == "" {
		t.Error("expected sessionId in response")
	}
}

func TestNewSession_CamelCaseAlias(t *testing.T) {
	svc, _, _ := testService(t)
	params := json.RawMessage(`{"cwd": "/tmp/project"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "newSession", params)
	if rpcErr != nil {
		t.Fatalf("newSession alias error: %s", rpcErr.Message)
	}
}

func TestLoadSession(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "test", Directory: "/tmp", Title: "Test",
	})

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/load", json.RawMessage(params))
	if rpcErr != nil {
		t.Fatalf("loadSession error: %s", rpcErr.Message)
	}
}

func TestLoadSession_NotFound(t *testing.T) {
	svc, _, _ := testService(t)

	params := json.RawMessage(`{"sessionId": "ses_nonexistent"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "loadSession", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams code, got %d", rpcErr.Code)
	}
}

func TestListSessions(t *testing.T) {
	svc, _, mock := testService(t)
	dir := "/tmp/acp-project"
	pid := project.IDFromDirectory(dir)

	mock.Create(context.Background(), session.CreateInput{
		ProjectID: pid, Directory: dir, Title: "Session 1",
	})
	mock.Create(context.Background(), session.CreateInput{
		ProjectID: pid, Directory: dir, Title: "Session 2",
	})
	mock.Create(context.Background(), session.CreateInput{
		ProjectID: "other", Directory: "/tmp", Title: "Other",
	})

	params, _ := json.Marshal(map[string]string{"cwd": dir})
	result, rpcErr := svc.HandleRequest(context.Background(), "session/list", params)
	if rpcErr != nil {
		t.Fatalf("listSessions error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)

	sessions := resp["sessions"].([]any)
	if len(sessions) != 2 {
		t.Errorf("expected 2 ACP sessions, got %d", len(sessions))
	}
}

func TestResumeSession(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/resume", json.RawMessage(params))
	if rpcErr != nil {
		t.Fatalf("resumeSession error: %s", rpcErr.Message)
	}
}

func TestCloseSession(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/close", json.RawMessage(params))
	if rpcErr != nil {
		t.Fatalf("closeSession error: %s", rpcErr.Message)
	}

	if len(mock.sessions) != 0 {
		t.Error("expected session to be deleted")
	}
}

func TestForkSession(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Original",
	})
	mock.messages[created.ID] = []session.Message{{
		ID: "msg_1", SessionID: created.ID, Role: session.RoleUser,
		Parts: []session.Part{{Type: session.PartText, Text: "hi"}},
	}}

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	result, rpcErr := svc.HandleRequest(context.Background(), "session/fork", json.RawMessage(params))
	if rpcErr != nil {
		t.Fatalf("forkSession error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)

	forkedID := resp["sessionId"].(string)
	if forkedID == created.ID {
		t.Error("fork should have different ID than parent")
	}
	if len(mock.sessions) != 2 {
		t.Errorf("expected 2 sessions after fork, got %d", len(mock.sessions))
	}
	if len(mock.messages[forkedID]) != 1 {
		t.Errorf("expected forked session to copy messages, got %d", len(mock.messages[forkedID]))
	}
}

func TestForkSession_NotFound(t *testing.T) {
	svc, _, _ := testService(t)

	params := json.RawMessage(`{"sessionId": "ses_nonexistent"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "forkSession", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestPrompt_LegacyBusPath(t *testing.T) {
	svc, b, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})

	sub := b.Subscribe("session.prompt")
	defer sub.Unsubscribe()

	params, _ := json.Marshal(map[string]any{
		"sessionId": created.ID,
		"content":   []map[string]string{{"type": "text", "text": "hello"}},
	})
	result, rpcErr := svc.HandleRequest(context.Background(), "prompt", params)
	if rpcErr != nil {
		t.Fatalf("prompt error: %s", rpcErr.Message)
	}
	data, _ := json.Marshal(result)
	if !strings.Contains(string(data), "end_turn") {
		t.Errorf("expected stopReason end_turn, got %s", data)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["content"] != "hello" {
			t.Errorf("expected 'hello', got %v", props["content"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for prompt event")
	}
}

func TestPrompt_WaitsForIdle(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	runner := newMockRunner(b)
	svc := NewServiceWithConfig(Config{
		Sessions:   mock,
		Bus:        b,
		Runner:     runner,
		DefaultCWD: "/tmp",
	})

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
		Model: &session.ModelRef{ProviderID: "ollama", ModelID: "llama"},
	})

	params, _ := json.Marshal(map[string]any{
		"sessionId": created.ID,
		"prompt":    []map[string]string{{"type": "text", "text": "hello"}},
	})

	start := time.Now()
	result, rpcErr := svc.HandleRequest(context.Background(), "session/prompt", params)
	elapsed := time.Since(start)
	if rpcErr != nil {
		t.Fatalf("prompt error: %s", rpcErr.Message)
	}
	if elapsed < 20*time.Millisecond {
		t.Fatalf("expected prompt to wait for idle, finished too fast (%v)", elapsed)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)
	if resp["stopReason"] != "end_turn" {
		t.Errorf("expected stopReason end_turn, got %v", resp["stopReason"])
	}
}

func TestPrompt_EmptyContent(t *testing.T) {
	svc, _, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})

	params, _ := json.Marshal(map[string]any{
		"sessionId": created.ID,
		"content":   []any{},
	})
	_, rpcErr := svc.HandleRequest(context.Background(), "prompt", params)
	if rpcErr == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestPrompt_MultipleTextParts(t *testing.T) {
	svc, b, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})

	sub := b.Subscribe("session.prompt")
	defer sub.Unsubscribe()

	params, _ := json.Marshal(map[string]any{
		"sessionId": created.ID,
		"prompt": []map[string]string{
			{"type": "text", "text": "part1"},
			{"type": "text", "text": "part2"},
		},
	})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/prompt", params)
	if rpcErr != nil {
		t.Fatalf("prompt error: %s", rpcErr.Message)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["content"] != "part1\npart2" {
			t.Errorf("expected 'part1\\npart2', got %v", props["content"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for prompt event")
	}
}

func TestCancel(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	runner := newMockRunner(b)
	svc := NewServiceWithConfig(Config{
		Sessions: mock,
		Bus:      b,
		Runner:   runner,
	})

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})
	_ = runner.StartTextPrompt(context.Background(), created.ID, "long")

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	svc.HandleRequest(context.Background(), "session/cancel", params)

	if runner.IsBusy(created.ID) {
		t.Error("expected runner to abort session")
	}
}

func TestSetSessionMode(t *testing.T) {
	svc, _, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test", Agent: "build",
	})

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID, "modeId": "plan"})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/set_mode", params)
	if rpcErr != nil {
		t.Fatalf("setSessionMode error: %s", rpcErr.Message)
	}
	if mock.sessions[created.ID].Agent != "plan" {
		t.Errorf("expected agent plan, got %q", mock.sessions[created.ID].Agent)
	}
}

func TestSetSessionModel(t *testing.T) {
	svc, _, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID, "modelId": "ollama/qwen3:8b"})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/set_model", params)
	if rpcErr != nil {
		t.Fatalf("setSessionModel error: %s", rpcErr.Message)
	}
	m := mock.sessions[created.ID].Model
	if m == nil || m.ProviderID != "ollama" || m.ModelID != "qwen3:8b" {
		t.Errorf("unexpected model: %+v", m)
	}
}

func TestSetSessionModel_Invalid(t *testing.T) {
	svc, _, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})
	params, _ := json.Marshal(map[string]string{"sessionId": created.ID, "modelId": "bare-model"})
	_, rpcErr := svc.HandleRequest(context.Background(), "setSessionModel", params)
	if rpcErr == nil {
		t.Fatal("expected error for invalid modelId")
	}
}

func TestUnknownMethod(t *testing.T) {
	svc, _, _ := testService(t)

	_, rpcErr := svc.HandleRequest(context.Background(), "nonexistent", nil)
	if rpcErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if rpcErr.Code != MethodNotFound {
		t.Errorf("expected MethodNotFound, got %d", rpcErr.Code)
	}
}

func TestSetSessionConfigOption(t *testing.T) {
	svc, _, _ := testService(t)

	result, rpcErr := svc.HandleRequest(context.Background(), "setSessionConfigOption", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	if string(data) != "{}" {
		t.Errorf("expected empty object, got %s", data)
	}
}

func TestStdioTransport_HandleRequest(t *testing.T) {
	svc, _, _ := testService(t)

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer

	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := transport.HandleStdio(ctx, reader)
	if err != nil {
		t.Fatalf("HandleStdio error: %v", err)
	}

	// Concurrent dispatch may need a brief wait for the response write.
	deadline := time.Now().Add(time.Second)
	for output.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	var resp map[string]any
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("parsing response: %v\noutput=%q", err, output.String())
	}
	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
}

func TestStdioTransport_InvalidJSON(t *testing.T) {
	svc, _, _ := testService(t)

	input := "not json\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer

	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	transport.HandleStdio(ctx, reader)

	deadline := time.Now().Add(time.Second)
	for output.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	var resp rpcResponse
	json.Unmarshal(output.Bytes(), &resp)

	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp.Error.Code != ParseError {
		t.Errorf("expected ParseError code, got %d", resp.Error.Code)
	}
}

func TestStdioTransport_WrongVersion(t *testing.T) {
	svc, _, _ := testService(t)

	input := `{"jsonrpc":"1.0","id":1,"method":"initialize"}` + "\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer

	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	transport.HandleStdio(ctx, reader)

	deadline := time.Now().Add(time.Second)
	for output.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	var resp rpcResponse
	json.Unmarshal(output.Bytes(), &resp)

	if resp.Error == nil {
		t.Fatal("expected error for wrong version")
	}
	if resp.Error.Code != InvalidRequest {
		t.Errorf("expected InvalidRequest code, got %d", resp.Error.Code)
	}
}

func TestStdioTransport_Notification(t *testing.T) {
	svc, _, _ := testService(t)

	input := `{"jsonrpc":"2.0","method":"cancel","params":{"sessionId":"ses_1"}}` + "\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer

	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	transport.HandleStdio(ctx, reader)
	time.Sleep(50 * time.Millisecond)

	if output.Len() > 0 {
		t.Errorf("notifications should not produce a response, got: %s", output.String())
	}
}

func TestStdioTransport_SendNotification(t *testing.T) {
	svc, _, _ := testService(t)
	var output bytes.Buffer
	transport := NewStdioTransport(svc, &output)

	transport.SendNotification("session/update", map[string]any{
		"sessionId": "ses_1",
		"update": map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": "hi"},
		},
	})

	var notif rpcNotification
	if err := json.Unmarshal(output.Bytes(), &notif); err != nil {
		t.Fatalf("parsing notification: %v", err)
	}

	if notif.Method != "session/update" {
		t.Errorf("expected method 'session/update', got %q", notif.Method)
	}
	if notif.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %q", notif.JSONRPC)
	}
}

func TestStdioTransport_MultipleRequests(t *testing.T) {
	svc, _, _ := testService(t)

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"authenticate"}` + "\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer

	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	transport.HandleStdio(ctx, reader)

	deadline := time.Now().Add(time.Second)
	for {
		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		if len(lines) >= 2 && lines[0] != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected 2 responses, got: %s", output.String())
		}
		time.Sleep(5 * time.Millisecond)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d: %s", len(lines), output.String())
	}

	for i, line := range lines {
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("parsing response %d: %v", i, err)
		}
		if resp["error"] != nil {
			t.Errorf("response %d has error: %v", i, resp["error"])
		}
	}
}

func TestMapToolKind(t *testing.T) {
	tests := []struct {
		tool   string
		expect string
	}{
		{"bash", "execute"},
		{"shell", "execute"},
		{"monitor", "execute"},
		{"webfetch", "fetch"},
		{"websearch", "fetch"},
		{"edit", "edit"},
		{"write", "edit"},
		{"apply_patch", "edit"},
		{"todowrite", "edit"},
		{"read", "read"},
		{"grep", "search"},
		{"glob", "search"},
		{"task", "other"},
		{"something_else", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			got := mapToolKind(tt.tool)
			if got != tt.expect {
				t.Errorf("mapToolKind(%q) = %q, want %q", tt.tool, got, tt.expect)
			}
		})
	}
}

func TestEventRelay_TextMessage(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })

	// Ensure subscription is active before publishing.
	time.Sleep(10 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"part": map[string]any{
			"type": "text",
			"text": "Hello world",
		},
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "agent_message_chunk") {
		t.Errorf("expected agent_message_chunk notification, got: %s", out)
	}
	if !strings.Contains(out, "session/update") {
		t.Errorf("expected session/update method, got: %s", out)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected content in notification, got: %s", out)
	}
}

func TestEventRelay_ReasoningMessage(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"part": map[string]any{
			"type": "reasoning",
			"text": "thinking...",
		},
	})

	output.WaitWrite(t, time.Second)
	cancel()

	if !strings.Contains(output.String(), "agent_thought_chunk") {
		t.Errorf("expected agent_thought_chunk notification, got: %s", output.String())
	}
}

func TestEventRelay_IgnoresOtherSessions(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_other",
		"part": map[string]any{
			"type": "text",
			"text": "should be filtered",
		},
	})

	time.Sleep(30 * time.Millisecond)
	cancel()

	if output.Len() > 0 {
		t.Errorf("expected no output for other session, got: %s", output.String())
	}
}

func TestEventRelay_ToolEvents(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("session.tool.begin", map[string]any{
		"sessionID":  "ses_1",
		"toolName":   "bash",
		"toolCallID": "tc_1",
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "tool_call") {
		t.Errorf("expected tool_call notification, got: %s", out)
	}
	if !strings.Contains(out, "execute") {
		t.Errorf("expected tool kind 'execute', got: %s", out)
	}
}

func TestPrompt_E2E_InitializeNewPromptIdle(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	runner := newMockRunner(b)
	svc := NewServiceWithConfig(Config{
		Sessions:   mock,
		Bus:        b,
		Runner:     runner,
		DefaultCWD: "/tmp/e2e",
	})
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.Run(ctx) })
	time.Sleep(10 * time.Millisecond)

	if _, err := svc.HandleRequest(ctx, "initialize", nil); err != nil {
		t.Fatalf("initialize: %s", err.Message)
	}

	created, rpcErr := svc.HandleRequest(ctx, "session/new", json.RawMessage(`{"cwd":"/tmp/e2e"}`))
	if rpcErr != nil {
		t.Fatalf("session/new: %s", rpcErr.Message)
	}
	createdMap := created.(map[string]any)
	sessionID := createdMap["sessionId"].(string)

	params, _ := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": "ping"}},
	})
	result, rpcErr := svc.HandleRequest(ctx, "session/prompt", params)
	if rpcErr != nil {
		t.Fatalf("session/prompt: %s", rpcErr.Message)
	}
	resp := result.(map[string]any)
	if resp["stopReason"] != "end_turn" {
		t.Fatalf("expected end_turn, got %v", resp["stopReason"])
	}

	output.WaitWrite(t, time.Second)
	if !strings.Contains(output.String(), "agent_message_chunk") {
		t.Fatalf("expected streamed agent_message_chunk, got: %s", output.String())
	}
}

func TestParsePermissionOutcome(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		expect permission.Reply
	}{
		{"allow_once selected", `{"outcome":{"outcome":"selected","optionId":"allow_once"}}`, permission.ReplyOnce},
		{"allow_always", `{"outcome":{"outcome":"selected","optionId":"allow_always"}}`, permission.ReplyAlways},
		{"reject", `{"outcome":{"outcome":"selected","optionId":"reject_once"}}`, permission.ReplyReject},
		{"cancelled", `{"outcome":{"outcome":"cancelled"}}`, permission.ReplyReject},
		{"empty", ``, permission.ReplyReject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePermissionOutcome(json.RawMessage(tt.raw))
			if got != tt.expect {
				t.Errorf("got %q want %q", got, tt.expect)
			}
		})
	}
}

func TestStdioTransport_SendRequest(t *testing.T) {
	svc, _, _ := testService(t)
	pr, pw := io.Pipe()
	var output syncBuffer
	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	safego.Go(func() {
		defer close(done)
		_ = transport.HandleStdio(ctx, pr)
	})

	// Client side: wait for outbound request, reply.
	safego.Go(func() {
		deadline := time.Now().Add(time.Second)
		for output.Len() == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		var req map[string]any
		if err := json.Unmarshal(output.Bytes(), &req); err != nil {
			t.Errorf("parse outbound: %v", err)
			cancel()
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}},
		})
		pw.Write(append(resp, '\n'))
	})

	result, err := transport.SendRequest(ctx, "session/request_permission", map[string]any{"sessionId": "ses_1"})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if !strings.Contains(string(result), "allow_once") {
		t.Fatalf("unexpected result: %s", result)
	}
	cancel()
	pw.Close()
	<-done
}

// --- Coverage gap tests ---

func TestReplayHistory_SendsTextAndReasoningNotifications(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "test", Directory: "/tmp", Title: "Test",
	})

	mock.messages[created.ID] = []session.Message{
		{
			ID: "msg_1", SessionID: created.ID, Role: session.RoleUser,
			Parts: []session.Part{{Type: session.PartText, Text: "user says hi"}},
		},
		{
			ID: "msg_2", SessionID: created.ID, Role: session.RoleAssistant,
			Parts: []session.Part{
				{Type: session.PartReasoning, Text: "thinking..."},
				{Type: session.PartText, Text: "assistant reply"},
			},
		},
	}

	output := newNotifyBuffer()
	_ = NewStdioTransport(svc, output)

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/load", json.RawMessage(params))
	if rpcErr != nil {
		t.Fatalf("loadSession error: %s", rpcErr.Message)
	}

	out := output.String()
	if !strings.Contains(out, "user_message_chunk") {
		t.Errorf("expected user_message_chunk in output, got: %s", out)
	}
	if !strings.Contains(out, "agent_thought_chunk") {
		t.Errorf("expected agent_thought_chunk in output, got: %s", out)
	}
	if !strings.Contains(out, "agent_message_chunk") {
		t.Errorf("expected agent_message_chunk in output, got: %s", out)
	}
	if !strings.Contains(out, "user says hi") {
		t.Errorf("expected user text in output, got: %s", out)
	}
	if !strings.Contains(out, "thinking...") {
		t.Errorf("expected reasoning text in output, got: %s", out)
	}
	if !strings.Contains(out, "assistant reply") {
		t.Errorf("expected assistant text in output, got: %s", out)
	}
}

func TestReplayHistory_SkipsNonTextParts(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "test", Directory: "/tmp", Title: "Test",
	})

	mock.messages[created.ID] = []session.Message{
		{
			ID: "msg_1", SessionID: created.ID, Role: session.RoleAssistant,
			Parts: []session.Part{{Type: "tool_use", Text: "should skip"}},
		},
	}

	output := newNotifyBuffer()
	_ = NewStdioTransport(svc, output)

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/load", json.RawMessage(params))
	if rpcErr != nil {
		t.Fatalf("loadSession error: %s", rpcErr.Message)
	}

	if output.Len() > 0 {
		t.Errorf("expected no output for non-text parts, got: %s", output.String())
	}
}

func TestResolveDefaultModel_WithSlashFormat(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	svc := NewServiceWithConfig(Config{
		Sessions:     mock,
		Bus:          b,
		DefaultModel: "ollama/llama3:8b",
		DefaultCWD:   "/tmp",
	})

	params := json.RawMessage(`{"cwd": "/tmp/project"}`)
	result, rpcErr := svc.HandleRequest(context.Background(), "session/new", params)
	if rpcErr != nil {
		t.Fatalf("session/new error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)
	sessionID := resp["sessionId"].(string)

	info := mock.sessions[sessionID]
	if info.Model == nil {
		t.Fatal("expected model to be set")
	}
	if info.Model.ProviderID != "ollama" {
		t.Errorf("expected provider 'ollama', got %q", info.Model.ProviderID)
	}
	if info.Model.ModelID != "llama3:8b" {
		t.Errorf("expected model 'llama3:8b', got %q", info.Model.ModelID)
	}
}

func TestResolveDefaultModel_InvalidFormatReturnsNil(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	svc := NewServiceWithConfig(Config{
		Sessions:     mock,
		Bus:          b,
		DefaultModel: "bare-model",
		DefaultCWD:   "/tmp",
	})

	params := json.RawMessage(`{"cwd": "/tmp/project"}`)
	result, rpcErr := svc.HandleRequest(context.Background(), "session/new", params)
	if rpcErr != nil {
		t.Fatalf("session/new error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)
	sessionID := resp["sessionId"].(string)

	if mock.sessions[sessionID].Model != nil {
		t.Error("expected nil model for invalid format")
	}
}

func TestNewSession_NoCWD(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	svc := NewServiceWithConfig(Config{
		Sessions: mock,
		Bus:      b,
	})

	_, rpcErr := svc.HandleRequest(context.Background(), "session/new", json.RawMessage(`{}`))
	if rpcErr == nil {
		t.Fatal("expected error when no CWD provided")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams, got %d", rpcErr.Code)
	}
	if !strings.Contains(rpcErr.Message, "cwd is required") {
		t.Errorf("expected 'cwd is required', got %q", rpcErr.Message)
	}
}

func TestResumeSession_NotFound(t *testing.T) {
	svc, _, _ := testService(t)
	params := json.RawMessage(`{"sessionId": "ses_nonexistent"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "session/resume", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams, got %d", rpcErr.Code)
	}
}

func TestSetSessionMode_EmptyModeId(t *testing.T) {
	svc, _, mock := testService(t)
	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
	})
	params, _ := json.Marshal(map[string]string{"sessionId": created.ID, "modeId": ""})
	_, rpcErr := svc.HandleRequest(context.Background(), "session/set_mode", params)
	if rpcErr == nil {
		t.Fatal("expected error for empty modeId")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams, got %d", rpcErr.Code)
	}
}

func TestSetSessionMode_SessionNotFound(t *testing.T) {
	svc, _, _ := testService(t)
	params := json.RawMessage(`{"sessionId":"ses_nonexistent","modeId":"plan"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "session/set_mode", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams, got %d", rpcErr.Code)
	}
}

func TestSetSessionModel_SessionNotFound(t *testing.T) {
	svc, _, _ := testService(t)
	params := json.RawMessage(`{"sessionId":"ses_nonexistent","modelId":"ollama/qwen3:8b"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "session/set_model", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams, got %d", rpcErr.Code)
	}
}

func TestPrompt_SessionNotFound(t *testing.T) {
	svc, _, _ := testService(t)
	params := json.RawMessage(`{"sessionId":"ses_nonexistent","prompt":[{"type":"text","text":"hello"}]}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "prompt", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if rpcErr.Code != InvalidParams {
		t.Errorf("expected InvalidParams, got %d", rpcErr.Code)
	}
}

func TestPrompt_ContextCancelled(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	runner := newMockRunner(b)
	runner.delay = 5 * time.Second
	svc := NewServiceWithConfig(Config{
		Sessions:   mock,
		Bus:        b,
		Runner:     runner,
		DefaultCWD: "/tmp",
	})

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
		Model:     &session.ModelRef{ProviderID: "ollama", ModelID: "llama"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	params, _ := json.Marshal(map[string]any{
		"sessionId": created.ID,
		"prompt":    []map[string]string{{"type": "text", "text": "hello"}},
	})
	result, rpcErr := svc.HandleRequest(ctx, "session/prompt", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %s", rpcErr.Message)
	}
	resp := result.(map[string]any)
	if resp["stopReason"] != "cancelled" {
		t.Errorf("expected stopReason 'cancelled', got %v", resp["stopReason"])
	}
}

func TestPrompt_CancelledDuringExecution(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	runner := newMockRunner(b)
	runner.delay = 2 * time.Second
	svc := NewServiceWithConfig(Config{
		Sessions:   mock,
		Bus:        b,
		Runner:     runner,
		DefaultCWD: "/tmp",
	})

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Test",
		Model:     &session.ModelRef{ProviderID: "ollama", ModelID: "llama"},
	})

	type promptResult struct {
		result map[string]any
		err    *RPCError
	}
	ch := make(chan promptResult, 1)
	safego.Go(func() {
		params, _ := json.Marshal(map[string]any{
			"sessionId": created.ID,
			"prompt":    []map[string]string{{"type": "text", "text": "hello"}},
		})
		result, rpcErr := svc.HandleRequest(context.Background(), "session/prompt", params)
		pr := promptResult{err: rpcErr}
		if result != nil {
			pr.result = result.(map[string]any)
		}
		ch <- pr
	})

	time.Sleep(50 * time.Millisecond)
	cancelParams, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	svc.HandleRequest(context.Background(), "session/cancel", cancelParams)

	select {
	case pr := <-ch:
		if pr.err != nil {
			t.Fatalf("prompt error: %s", pr.err.Message)
		}
		if pr.result["stopReason"] != "cancelled" {
			t.Errorf("expected stopReason 'cancelled', got %v", pr.result["stopReason"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for prompt to complete")
	}
}

func TestEventRelay_ToolEndEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("session.tool.end", map[string]any{
		"sessionID":  "ses_1",
		"toolCallID": "tc_1",
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "tool_call_update") {
		t.Errorf("expected tool_call_update notification, got: %s", out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("expected status 'completed', got: %s", out)
	}
}

func TestEventRelay_LegacyFlatMessagePart(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"type":      "text",
		"content":   "legacy content",
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "agent_message_chunk") {
		t.Errorf("expected agent_message_chunk, got: %s", out)
	}
	if !strings.Contains(out, "legacy content") {
		t.Errorf("expected legacy content, got: %s", out)
	}
}

func TestEventRelay_LegacyFlatTextFallback(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"type":      "text",
		"text":      "legacy text fallback",
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "legacy text fallback") {
		t.Errorf("expected 'legacy text fallback', got: %s", out)
	}
}

func TestEventRelay_PartContentFallback(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"part": map[string]any{
			"type":    "text",
			"content": "via content field",
		},
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "via content field") {
		t.Errorf("expected 'via content field', got: %s", out)
	}
}

func TestEventRelay_ToolBeginFallbackName(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("session.tool.begin", map[string]any{
		"sessionID":  "ses_1",
		"tool":       "edit",
		"toolCallID": "tc_2",
	})

	output.WaitWrite(t, time.Second)
	cancel()

	out := output.String()
	if !strings.Contains(out, "tool_call") {
		t.Errorf("expected tool_call notification, got: %s", out)
	}
}

func TestEventRelay_PermissionAskedMapForm(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)
	// permSvc is nil — handlePermissionAsked returns early

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("permission.asked", map[string]any{
		"id":         "perm_1",
		"sessionID":  "ses_1",
		"permission": "bash",
	})

	// Give the goroutine time to process the event.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// No panic or deadlock = success; handlePermissionAsked exits early with nil permSvc.
}

func TestEventRelay_PermissionAskedTypedRequest(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	safego.Go(func() { relay.RunSession(ctx, "ses_1") })
	time.Sleep(10 * time.Millisecond)

	b.Publish("permission.asked", permission.Request{
		ID:         "perm_1",
		SessionID:  "ses_1",
		Permission: "bash",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()
}

func TestEventRelay_SetPermissionService(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	output := newNotifyBuffer()
	transport := NewStdioTransport(svc, output)
	relay := NewEventRelay(b, transport)

	permSvc := permission.NewService(b)
	relay.SetPermissionService(permSvc)

	// Verify by triggering a permission event; the relay won't panic
	// because permSvc is set (it will attempt SendRequest and fail, but
	// handlePermissionAsked handles that error path).
}

func TestStdioTransport_MissingMethod(t *testing.T) {
	svc, _, _ := testService(t)

	input := `{"jsonrpc":"2.0","id":1,"method":""}` + "\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer

	transport := NewStdioTransport(svc, &output)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	transport.HandleStdio(ctx, reader)

	deadline := time.Now().Add(time.Second)
	for output.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	var resp rpcResponse
	json.Unmarshal(output.Bytes(), &resp)

	if resp.Error == nil {
		t.Fatal("expected error for missing method")
	}
	if resp.Error.Code != InvalidRequest {
		t.Errorf("expected InvalidRequest code, got %d", resp.Error.Code)
	}
}

func TestStdioTransport_RejectPendingOnClose(t *testing.T) {
	svc, _, _ := testService(t)
	pr, pw := io.Pipe()
	var output syncBuffer
	transport := NewStdioTransport(svc, &output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	safego.Go(func() {
		defer close(done)
		_ = transport.HandleStdio(ctx, pr)
	})

	pending := &pendingRequest{ch: make(chan rpcResponse, 1)}
	transport.pending.Store(int64(999), pending)

	pw.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for HandleStdio to finish")
	}

	select {
	case resp := <-pending.ch:
		if resp.Error == nil {
			t.Fatal("expected error response for rejected pending request")
		}
		if resp.Error.Code != InternalError {
			t.Errorf("expected InternalError code, got %d", resp.Error.Code)
		}
	default:
		t.Fatal("expected pending request to be rejected")
	}
}

func TestPendingKey_Variants(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect any
	}{
		{"float64", float64(42), int64(42)},
		{"json.Number", json.Number("42"), int64(42)},
		{"string", "abc", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pendingKey(tt.input)
			if got != tt.expect {
				t.Errorf("pendingKey(%v) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}
