package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

type mockSessionService struct {
	sessions map[string]*session.Info
	nextID   int
}

func newMockSessionService() *mockSessionService {
	return &mockSessionService{
		sessions: make(map[string]*session.Info),
	}
}

func (m *mockSessionService) Create(_ context.Context, input session.CreateInput) (*session.Info, error) {
	m.nextID++
	id := fmt.Sprintf("ses_%d", m.nextID)
	info := &session.Info{
		ID:        id,
		ProjectID: input.ProjectID,
		Directory: input.Directory,
		Title:     input.Title,
		Agent:     input.Agent,
		ParentID:  input.ParentID,
		CreatedAt: time.Now(),
	}
	m.sessions[id] = info
	return info, nil
}

func (m *mockSessionService) Get(_ context.Context, id string) (*session.Info, error) {
	info, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return info, nil
}

func (m *mockSessionService) List(_ context.Context, projectID string) ([]*session.Info, error) {
	var result []*session.Info
	for _, info := range m.sessions {
		if info.ProjectID == projectID {
			result = append(result, info)
		}
	}
	return result, nil
}

func (m *mockSessionService) Delete(_ context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func testService(t *testing.T) (*Service, *bus.Bus, *mockSessionService) {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	mock := newMockSessionService()
	svc := NewService(mock, b)
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
	result, rpcErr := svc.HandleRequest(context.Background(), "newSession", params)
	if rpcErr != nil {
		t.Fatalf("newSession error: %s", rpcErr.Message)
	}

	data, _ := json.Marshal(result)
	var resp map[string]any
	json.Unmarshal(data, &resp)

	if resp["sessionId"] == nil || resp["sessionId"] == "" {
		t.Error("expected sessionId in response")
	}
}

func TestLoadSession(t *testing.T) {
	svc, _, mock := testService(t)

	created, _ := mock.Create(context.Background(), session.CreateInput{
		ProjectID: "test", Directory: "/tmp", Title: "Test",
	})

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	_, rpcErr := svc.HandleRequest(context.Background(), "loadSession", json.RawMessage(params))
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

	mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Session 1",
	})
	mock.Create(context.Background(), session.CreateInput{
		ProjectID: "acp", Directory: "/tmp", Title: "Session 2",
	})
	mock.Create(context.Background(), session.CreateInput{
		ProjectID: "other", Directory: "/tmp", Title: "Other",
	})

	result, rpcErr := svc.HandleRequest(context.Background(), "listSessions", nil)
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
	_, rpcErr := svc.HandleRequest(context.Background(), "resumeSession", json.RawMessage(params))
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
	_, rpcErr := svc.HandleRequest(context.Background(), "closeSession", json.RawMessage(params))
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

	params, _ := json.Marshal(map[string]string{"sessionId": created.ID})
	result, rpcErr := svc.HandleRequest(context.Background(), "forkSession", json.RawMessage(params))
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
}

func TestForkSession_NotFound(t *testing.T) {
	svc, _, _ := testService(t)

	params := json.RawMessage(`{"sessionId": "ses_nonexistent"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "forkSession", params)
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestPrompt(t *testing.T) {
	svc, b, _ := testService(t)

	sub := b.Subscribe("session.prompt")
	defer sub.Unsubscribe()

	params := json.RawMessage(`{"sessionId": "ses_1", "content": [{"type": "text", "text": "hello"}]}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "prompt", params)
	if rpcErr != nil {
		t.Fatalf("prompt error: %s", rpcErr.Message)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["content"] != "hello" {
			t.Errorf("expected 'hello', got %v", props["content"])
		}
		if props["source"] != "acp" {
			t.Errorf("expected source 'acp', got %v", props["source"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for prompt event")
	}
}

func TestPrompt_EmptyContent(t *testing.T) {
	svc, _, _ := testService(t)

	params := json.RawMessage(`{"sessionId": "ses_1", "content": []}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "prompt", params)
	if rpcErr == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestPrompt_MultipleTextParts(t *testing.T) {
	svc, b, _ := testService(t)

	sub := b.Subscribe("session.prompt")
	defer sub.Unsubscribe()

	params := json.RawMessage(`{"sessionId": "ses_1", "content": [{"type": "text", "text": "part1"}, {"type": "text", "text": "part2"}]}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "prompt", params)
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
	svc, b, _ := testService(t)

	sub := b.Subscribe("session.abort")
	defer sub.Unsubscribe()

	params := json.RawMessage(`{"sessionId": "ses_1"}`)
	svc.HandleRequest(context.Background(), "cancel", params)

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["sessionID"] != "ses_1" {
			t.Errorf("expected ses_1, got %v", props["sessionID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for abort event")
	}
}

func TestSetSessionMode(t *testing.T) {
	svc, b, _ := testService(t)

	sub := b.Subscribe("session.mode")
	defer sub.Unsubscribe()

	params := json.RawMessage(`{"sessionId": "ses_1", "modeId": "plan"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "setSessionMode", params)
	if rpcErr != nil {
		t.Fatalf("setSessionMode error: %s", rpcErr.Message)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["mode"] != "plan" {
			t.Errorf("expected 'plan', got %v", props["mode"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for mode event")
	}
}

func TestSetSessionModel(t *testing.T) {
	svc, b, _ := testService(t)

	sub := b.Subscribe("session.model")
	defer sub.Unsubscribe()

	params := json.RawMessage(`{"sessionId": "ses_1", "modelId": "qwen3:8b"}`)
	_, rpcErr := svc.HandleRequest(context.Background(), "setSessionModel", params)
	if rpcErr != nil {
		t.Fatalf("setSessionModel error: %s", rpcErr.Message)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["model"] != "qwen3:8b" {
			t.Errorf("expected 'qwen3:8b', got %v", props["model"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for model event")
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

// Transport tests

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

	var resp rpcResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("parsing response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
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

	if output.Len() > 0 {
		t.Errorf("notifications should not produce a response, got: %s", output.String())
	}
}

func TestStdioTransport_SendNotification(t *testing.T) {
	svc, _, _ := testService(t)
	var output bytes.Buffer
	transport := NewStdioTransport(svc, &output)

	transport.SendNotification("sessionUpdate", map[string]any{
		"sessionId": "ses_1",
		"type":      "status",
		"status":    "idle",
	})

	var notif rpcNotification
	if err := json.Unmarshal(output.Bytes(), &notif); err != nil {
		t.Fatalf("parsing notification: %v", err)
	}

	if notif.Method != "sessionUpdate" {
		t.Errorf("expected method 'sessionUpdate', got %q", notif.Method)
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

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d: %s", len(lines), output.String())
	}

	for i, line := range lines {
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("parsing response %d: %v", i, err)
		}
		if resp.Error != nil {
			t.Errorf("response %d has error: %s", i, resp.Error.Message)
		}
	}
}

// Event relay tests

func TestMapToolKind(t *testing.T) {
	tests := []struct {
		tool   string
		expect string
	}{
		{"bash", "execute"},
		{"shell", "execute"},
		{"webfetch", "fetch"},
		{"websearch", "fetch"},
		{"edit", "edit"},
		{"write", "edit"},
		{"read", "read"},
		{"grep", "search"},
		{"glob", "search"},
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
	var output bytes.Buffer
	transport := NewStdioTransport(svc, &output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx, "ses_1")

	time.Sleep(50 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"type":      "text",
		"content":   "Hello world",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	if !strings.Contains(output.String(), "agent_message_chunk") {
		t.Errorf("expected agent_message_chunk notification, got: %s", output.String())
	}
	if !strings.Contains(output.String(), "Hello world") {
		t.Errorf("expected content in notification, got: %s", output.String())
	}
}

func TestEventRelay_ReasoningMessage(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	var output bytes.Buffer
	transport := NewStdioTransport(svc, &output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx, "ses_1")

	time.Sleep(50 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_1",
		"type":      "reasoning",
		"content":   "thinking...",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	if !strings.Contains(output.String(), "agent_thought_chunk") {
		t.Errorf("expected agent_thought_chunk notification, got: %s", output.String())
	}
}

func TestEventRelay_IgnoresOtherSessions(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	var output bytes.Buffer
	transport := NewStdioTransport(svc, &output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx, "ses_1")

	time.Sleep(50 * time.Millisecond)

	b.Publish("message.part.updated", map[string]any{
		"sessionID": "ses_other",
		"type":      "text",
		"content":   "should be filtered",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	if output.Len() > 0 {
		t.Errorf("expected no output for other session, got: %s", output.String())
	}
}

func TestEventRelay_ToolEvents(t *testing.T) {
	b := bus.New()
	defer b.Close()

	svc := NewService(newMockSessionService(), b)
	var output bytes.Buffer
	transport := NewStdioTransport(svc, &output)
	relay := NewEventRelay(b, transport)

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx, "ses_1")

	time.Sleep(50 * time.Millisecond)

	b.Publish("tool.running", map[string]any{
		"sessionID":  "ses_1",
		"name":       "bash",
		"toolCallID": "tc_1",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	if !strings.Contains(output.String(), "tool_call") {
		t.Errorf("expected tool_call notification, got: %s", output.String())
	}
	if !strings.Contains(output.String(), "execute") {
		t.Errorf("expected tool kind 'execute', got: %s", output.String())
	}
}
