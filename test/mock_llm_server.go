package test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

type MockToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type MockResponse struct {
	Content      string
	ToolCalls    []MockToolCall
	FinishReason string
	StatusCode   int
	Delay        time.Duration
	RawBody      string
}

type CapturedRequest struct {
	Model    string            `json:"model"`
	Messages []json.RawMessage `json:"messages"`
	Tools    []json.RawMessage `json:"tools"`
	Stream   bool              `json:"stream"`
}

type MockLLMServer struct {
	mu        sync.Mutex
	server    *http.Server
	listener  net.Listener
	responses []MockResponse
	callIndex int
	requests  []CapturedRequest
	t         *testing.T
}

func NewMockLLMServer(t *testing.T) *MockLLMServer {
	t.Helper()

	m := &MockLLMServer{t: t}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", m.handleCompletions)
	mux.HandleFunc("GET /v1/models", m.handleModels)
	mux.HandleFunc("GET /api/tags", m.handleOllamaTags)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock server listen: %v", err)
	}
	m.listener = ln
	m.server = &http.Server{Handler: mux}

	go func() {
		if err := m.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Errorf("mock server serve: %v", err)
		}
	}()

	return m
}

func (m *MockLLMServer) URL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *MockLLMServer) ModelName() string {
	return "mock/test-model"
}

func (m *MockLLMServer) AddResponse(r MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.FinishReason == "" {
		if len(r.ToolCalls) > 0 {
			r.FinishReason = "tool_calls"
		} else {
			r.FinishReason = "stop"
		}
	}
	m.responses = append(m.responses, r)
}

func (m *MockLLMServer) Requests() []CapturedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CapturedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

func (m *MockLLMServer) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func (m *MockLLMServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = nil
	m.callIndex = 0
	m.requests = nil
}

func (m *MockLLMServer) Close() {
	m.server.Close()
}

func (m *MockLLMServer) nextResponse() MockResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.responses) == 0 {
		return MockResponse{Content: "No mock response configured", FinishReason: "stop"}
	}

	idx := m.callIndex
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	m.callIndex++
	return m.responses[idx]
}

func (m *MockLLMServer) handleCompletions(w http.ResponseWriter, r *http.Request) {
	var req CapturedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()

	resp := m.nextResponse()

	if resp.Delay > 0 {
		time.Sleep(resp.Delay)
	}

	if resp.StatusCode != 0 && resp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"mock error","type":"error","code":%d}}`, resp.StatusCode), resp.StatusCode)
		return
	}

	if resp.RawBody != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp.RawBody))
		return
	}

	if req.Stream {
		m.streamResponse(w, resp)
		return
	}

	m.jsonResponse(w, resp)
}

func (m *MockLLMServer) streamResponse(w http.ResponseWriter, resp MockResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	if resp.Content != "" {
		chunk := map[string]any{
			"id":      "mock-1",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"role":    "assistant",
						"content": resp.Content,
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	if len(resp.ToolCalls) > 0 {
		toolCallDeltas := make([]map[string]any, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			toolCallDeltas[i] = map[string]any{
				"index": i,
				"id":    fmt.Sprintf("call_%d", i),
				"type":  "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			}
		}

		chunk := map[string]any{
			"id":      "mock-1",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"role":       "assistant",
						"tool_calls": toolCallDeltas,
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	finishChunk := map[string]any{
		"id":      "mock-1",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   "test-model",
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": resp.FinishReason,
			},
		},
	}
	data, _ := json.Marshal(finishChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (m *MockLLMServer) jsonResponse(w http.ResponseWriter, resp MockResponse) {
	w.Header().Set("Content-Type", "application/json")

	message := map[string]any{
		"role":    "assistant",
		"content": resp.Content,
	}

	if len(resp.ToolCalls) > 0 {
		tcs := make([]map[string]any, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			tcs[i] = map[string]any{
				"id":   fmt.Sprintf("call_%d", i),
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			}
		}
		message["tool_calls"] = tcs
	}

	result := map[string]any{
		"id":      "mock-1",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "test-model",
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": resp.FinishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}

	json.NewEncoder(w).Encode(result)
}

func (m *MockLLMServer) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       "test-model",
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "mock",
			},
		},
	})
}

func (m *MockLLMServer) handleOllamaTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"models": []map[string]any{
			{
				"name":        "test-model",
				"model":       "test-model",
				"modified_at": time.Now().Format(time.RFC3339),
				"size":        1000000,
			},
		},
	})
}
