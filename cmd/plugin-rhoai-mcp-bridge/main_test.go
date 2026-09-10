package main

import (
	"testing"
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
