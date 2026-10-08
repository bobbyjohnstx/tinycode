package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func TestBtw_ReturnsAnswer(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: "The answer is 42"},
	})
	sessionID := h.createSession("Btw Test", "build")

	body := `{"question":"What is the meaning of life?"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/btw", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["answer"] != "The answer is 42" {
		t.Errorf("expected 'The answer is 42', got %q", result["answer"])
	}
}

func TestBtw_EmptyQuestionReturnsError(t *testing.T) {
	h := newTestHarness(t, nil)
	sessionID := h.createSession("Btw Empty", "build")

	body := `{"question":""}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/btw", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBtw_InvalidSessionReturnsNotFound(t *testing.T) {
	h := newTestHarness(t, nil)

	body := `{"question":"hello"}`
	resp, err := http.Post(h.baseURL()+"/session/nonexistent/btw", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestBtw_DoesNotModifyConversationHistory(t *testing.T) {
	// Use two scenarios: one for the initial prompt, one for the btw side question.
	h := newTestHarness(t, []mockScenario{
		{textResponse: "Hello from main conversation"},
		{textResponse: "Side answer here"},
	})
	sessionID := h.createSession("Btw No History", "build")

	// Subscribe to session.status to detect idle after prompt.
	statusSub := h.bus.Subscribe("session.status")
	defer statusSub.Unsubscribe()

	// Send a normal prompt first.
	h.sendPromptAsync(sessionID, "Hello")

	// Wait for session to go idle after processing the prompt.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt := <-statusSub.C:
			props := evt.Properties.(map[string]any)
			if props["sessionID"] != sessionID {
				continue
			}
			status, _ := props["status"].(map[string]any)
			if status["type"] == "idle" {
				goto promptDone
			}
		case <-deadline:
			t.Fatal("timeout waiting for prompt to complete")
		}
	}
promptDone:

	// Count messages after normal prompt.
	messagesBefore := h.listMessages(sessionID)

	// Send a btw side question.
	btwBody := `{"question":"What is Go?"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/btw", "application/json", strings.NewReader(btwBody))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify message count is unchanged.
	messagesAfter := h.listMessages(sessionID)
	if len(messagesAfter) != len(messagesBefore) {
		t.Errorf("message count changed: before=%d, after=%d", len(messagesBefore), len(messagesAfter))
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		input      string
		wantProv   string
		wantModel  string
	}{
		{"ollama/qwen3.5:9b", "ollama", "qwen3.5:9b"},
		{"gpt-4o", "", "gpt-4o"},
		{"anthropic/claude-opus-4-20250514", "anthropic", "claude-opus-4-20250514"},
		{"", "", ""},
	}
	for _, tt := range tests {
		pid, mid := parseModelRef(tt.input)
		if pid != tt.wantProv || mid != tt.wantModel {
			t.Errorf("parseModelRef(%q) = (%q, %q), want (%q, %q)", tt.input, pid, mid, tt.wantProv, tt.wantModel)
		}
	}
}

func TestBtw_UsesSmallModelWhenConfigured(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: "small model answer"},
	})

	// Register a small model provider.
	h.server.deps.Registry.Register(&provider.Info{
		ID:     "ollama",
		Name:   "Ollama",
		Source: "test",
		Models: map[string]*provider.Model{
			"qwen3.5:9b": {
				ID:           "qwen3.5:9b",
				ProviderID:   "ollama",
				Name:         "Qwen 3.5 9B",
				API:          provider.ModelAPI{ID: "qwen3.5:9b"},
				Status:       "available",
				Capabilities: provider.ModelCaps{Input: provider.ModalityCaps{Text: true}, Output: provider.ModalityCaps{Text: true}},
			},
		},
	})

	// Configure small_model.
	h.server.sessionManager.cfg.SmallModel = "ollama/qwen3.5:9b"

	sessionID := h.createSession("Btw Small", "build")

	body := `{"question":"What is Go?"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/btw", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["answer"] != "small model answer" {
		t.Errorf("expected 'small model answer', got %q", result["answer"])
	}
}

func TestBtw_FallsBackWhenSmallModelEmpty(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: "session model answer"},
	})
	// SmallModel is empty by default — should use session model.
	sessionID := h.createSession("Btw Fallback Empty", "build")

	body := `{"question":"What is Go?"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/btw", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["answer"] != "session model answer" {
		t.Errorf("expected 'session model answer', got %q", result["answer"])
	}
}

func TestBtw_FallsBackWhenSmallModelUnknown(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: "session model answer"},
	})

	// Configure a small_model that doesn't exist in the registry.
	h.server.sessionManager.cfg.SmallModel = "nonexistent/fake-model"

	sessionID := h.createSession("Btw Fallback Unknown", "build")

	body := `{"question":"What is Go?"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/btw", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("btw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["answer"] != "session model answer" {
		t.Errorf("expected 'session model answer', got %q", result["answer"])
	}
}

func TestBuildBtwContext_EmptyMessages(t *testing.T) {
	result := buildBtwContext(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildBtwContext_TruncatesLongMessages(t *testing.T) {
	longText := strings.Repeat("x", 600)
	messages := []session.Message{
		{Role: session.RoleUser, Parts: []session.Part{{Type: session.PartText, Text: longText}}},
	}
	result := buildBtwContext(messages)
	if !strings.Contains(result, "...") {
		t.Error("expected truncation marker in context")
	}
	// The text should be truncated to 500 + "..."
	if strings.Contains(result, strings.Repeat("x", 501)) {
		t.Error("message text was not truncated")
	}
}

func TestBuildBtwContext_SkipsToolMessages(t *testing.T) {
	messages := []session.Message{
		{Role: session.RoleUser, Parts: []session.Part{{Type: session.PartText, Text: "user message"}}},
		{Role: session.RoleTool, Parts: []session.Part{{Type: session.PartToolResult, ToolResult: "tool output"}}},
		{Role: session.RoleAssistant, Parts: []session.Part{{Type: session.PartText, Text: "assistant reply"}}},
	}
	result := buildBtwContext(messages)
	if strings.Contains(result, "tool") {
		t.Error("context should not contain tool messages")
	}
	if !strings.Contains(result, "[user]") {
		t.Error("context should contain user message")
	}
	if !strings.Contains(result, "[assistant]") {
		t.Error("context should contain assistant message")
	}
}

func TestBuildBtwContext_LimitsToMaxMessages(t *testing.T) {
	var messages []session.Message
	for i := 0; i < 20; i++ {
		messages = append(messages, session.Message{
			Role:  session.RoleUser,
			Parts: []session.Part{{Type: session.PartText, Text: "msg"}},
		})
	}
	result := buildBtwContext(messages)
	// Should contain at most btwMaxContextMessages entries.
	count := strings.Count(result, "[user]")
	if count > btwMaxContextMessages {
		t.Errorf("expected at most %d messages, got %d", btwMaxContextMessages, count)
	}
}
