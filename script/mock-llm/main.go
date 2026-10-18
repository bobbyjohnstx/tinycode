// Command mock-llm is a tiny OpenAI-compatible chat completions server for
// fresh-env / pre-release refinement. It plays a scripted sequence of responses
// (including tool_calls) so silent tool failures are visible.
//
// tinycode always streams (Accept: text/event-stream), so responses are SSE.
//
// Usage:
//
//	go run ./script/mock-llm -addr 0.0.0.0:18080 -script scenario.json
//
// Scenario JSON is an array of objects:
//
//	[{"content":"…"}, {"tool_calls":[{"name":"lsp_diagnostics","arguments":"{\"file_path\":\"/x\"}"}]}]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type toolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type scriptedResponse struct {
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

func main() {
	addr := flag.String("addr", "0.0.0.0:0", "listen address (use 0.0.0.0 so containers can reach the host)")
	scriptPath := flag.String("script", "", "path to JSON array of responses (required)")
	flag.Parse()
	if *scriptPath == "" {
		fmt.Fprintln(os.Stderr, "usage: mock-llm -script scenario.json [-addr host:port]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*scriptPath)
	if err != nil {
		log.Fatal(err)
	}
	var responses []scriptedResponse
	if err := json.Unmarshal(raw, &responses); err != nil {
		log.Fatalf("parse script: %v", err)
	}
	if len(responses) == 0 {
		log.Fatal("script must contain at least one response")
	}

	s := &server{responses: responses}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.handleCompletions)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("GET /api/tags", s.handleOllamaTags)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("MOCK_LLM_URL=http://%s\n", ln.Addr().String())
	log.Fatal(http.Serve(ln, mux))
}

type server struct {
	mu        sync.Mutex
	responses []scriptedResponse
	idx       int
}

func (s *server) next() scriptedResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.idx
	if i >= len(s.responses) {
		i = len(s.responses) - 1
	}
	s.idx++
	return s.responses[i]
}

func (s *server) handleModels(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":[{"id":"test-model","object":"model"}]}`))
}

func (s *server) handleOllamaTags(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"models":[{"name":"test-model"}]}`))
}

func (s *server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stream bool `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := s.next()
	log.Printf("completions stream=%v tool_calls=%d content_len=%d", req.Stream, len(resp.ToolCalls), len(resp.Content))

	// tinycode always streams; also honour explicit stream=true.
	if req.Stream || r.Header.Get("Accept") == "text/event-stream" {
		writeSSE(w, resp)
		return
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, resp scriptedResponse) {
	finish := "stop"
	if len(resp.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	toolCalls := make([]map[string]any, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		toolCalls[i] = map[string]any{
			"id":   fmt.Sprintf("call_%d", i),
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		}
	}
	msg := map[string]any{"role": "assistant", "content": resp.Content}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	body := map[string]any{
		"id":      "mock-1",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "test-model",
		"choices": []map[string]any{{
			"index":         0,
			"message":       msg,
			"finish_reason": finish,
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func writeSSE(w http.ResponseWriter, resp scriptedResponse) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	write := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	if len(resp.ToolCalls) > 0 {
		for i, tc := range resp.ToolCalls {
			write(map[string]any{
				"id": "mock-1", "object": "chat.completion.chunk", "model": "test-model",
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"index": i,
							"id":    fmt.Sprintf("call_%d", i),
							"type":  "function",
							"function": map[string]any{
								"name":      tc.Name,
								"arguments": "",
							},
						}},
					},
					"finish_reason": nil,
				}},
			})
			write(map[string]any{
				"id": "mock-1", "object": "chat.completion.chunk", "model": "test-model",
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": i,
							"function": map[string]any{
								"arguments": tc.Arguments,
							},
						}},
					},
					"finish_reason": nil,
				}},
			})
		}
		write(map[string]any{
			"id": "mock-1", "object": "chat.completion.chunk", "model": "test-model",
			"choices": []map[string]any{{
				"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
			}},
		})
	} else {
		content := resp.Content
		if content == "" {
			content = "ok"
		}
		write(map[string]any{
			"id": "mock-1", "object": "chat.completion.chunk", "model": "test-model",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{"role": "assistant", "content": content},
				"finish_reason": nil,
			}},
		})
		write(map[string]any{
			"id": "mock-1", "object": "chat.completion.chunk", "model": "test-model",
			"choices": []map[string]any{{
				"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
			}},
		})
	}

	write(map[string]any{
		"id": "mock-1", "object": "chat.completion.chunk", "model": "test-model",
		"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
