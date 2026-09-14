package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-mcp-bridge" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-mcp-bridge")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(p.Tools))
	}
	wantNames := []string{"rhoai_mcp_list", "rhoai_mcp_call"}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] Execute is nil", i)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{})
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
		if _, ok := params["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties is not map[string]any", tool.Name)
		}
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name         string
		raw          map[string]any
		mcpServerURL string
		oauthToken   string
	}{
		{
			name:         "extracts all fields",
			raw:          map[string]any{"mcpServerUrl": "http://mcp:8080", "oauthToken": "tok-abc"},
			mcpServerURL: "http://mcp:8080",
			oauthToken:   "tok-abc",
		},
		{
			name:         "empty map",
			raw:          map[string]any{},
			mcpServerURL: "",
			oauthToken:   "",
		},
		{
			name:         "wrong types ignored",
			raw:          map[string]any{"mcpServerUrl": 42},
			mcpServerURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.McpServerURL != tt.mcpServerURL {
				t.Errorf("McpServerURL = %q, want %q", opts.McpServerURL, tt.mcpServerURL)
			}
			if opts.OAuthToken != tt.oauthToken {
				t.Errorf("OAuthToken = %q, want %q", opts.OAuthToken, tt.oauthToken)
			}
		})
	}
}

// --- httptest mock helpers ---

func newMockMcpClient(handler http.Handler) (*mcpClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &mcpClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestListTools_Mock(t *testing.T) {
	client, srv := newMockMcpClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["method"] != "tools/list" {
			t.Errorf("expected method tools/list, got %v", req["method"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"tools": []map[string]any{
					{"name": "get_weather", "description": "Get weather forecast", "inputSchema": map[string]any{"type": "object"}},
					{"name": "run_query", "description": "Run a database query", "inputSchema": map[string]any{"type": "object"}},
				},
			},
		})
	}))
	defer srv.Close()

	tools, err := client.listTools(context.Background())
	if err != nil {
		t.Fatalf("listTools failed: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Name != "get_weather" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "get_weather")
	}
	if tools[1].Description != "Run a database query" {
		t.Errorf("tools[1].Description = %q, want %q", tools[1].Description, "Run a database query")
	}
}

func TestCallTool_Mock(t *testing.T) {
	client, srv := newMockMcpClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["method"] != "tools/call" {
			t.Errorf("expected method tools/call, got %v", req["method"])
		}
		params := req["params"].(map[string]any)
		if params["name"] != "get_weather" {
			t.Errorf("expected tool name get_weather, got %v", params["name"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Temperature: 72°F"},
					{"type": "text", "text": "Conditions: Sunny"},
				},
				"isError": false,
			},
		})
	}))
	defer srv.Close()

	result, err := client.callTool(context.Background(), "get_weather", map[string]any{"city": "Austin"})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}
	if !strings.Contains(result, "Temperature: 72°F") {
		t.Errorf("result missing temperature, got: %s", result)
	}
	if !strings.Contains(result, "Conditions: Sunny") {
		t.Errorf("result missing conditions, got: %s", result)
	}
}

func TestCallTool_ToolError_Mock(t *testing.T) {
	client, srv := newMockMcpClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "City not found"},
				},
				"isError": true,
			},
		})
	}))
	defer srv.Close()

	_, err := client.callTool(context.Background(), "get_weather", map[string]any{"city": "Nowhere"})
	if err == nil {
		t.Fatal("expected error for isError:true response")
	}
	if !strings.Contains(err.Error(), "City not found") {
		t.Errorf("error should contain tool error message, got: %v", err)
	}
}

func TestRpcCall_RPCError_Mock(t *testing.T) {
	client, srv := newMockMcpClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]any{
				"message": "Method not found",
			},
		})
	}))
	defer srv.Close()

	_, err := client.rpcCall(context.Background(), "nonexistent/method", nil)
	if err == nil {
		t.Fatal("expected error for RPC error response")
	}
	if !strings.Contains(err.Error(), "Method not found") {
		t.Errorf("error should contain RPC error message, got: %v", err)
	}
}

func TestListTools_Empty_Mock(t *testing.T) {
	client, srv := newMockMcpClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"tools": []map[string]any{},
			},
		})
	}))
	defer srv.Close()

	tools, err := client.listTools(context.Background())
	if err != nil {
		t.Fatalf("listTools failed: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("got %d tools, want 0", len(tools))
	}
}
