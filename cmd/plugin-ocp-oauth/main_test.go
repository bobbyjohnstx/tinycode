package main

import (
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "ocp-oauth" {
		t.Errorf("got %q, want %q", p.ID, "ocp-oauth")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(p.Tools))
	}
	tool := p.Tools[0]
	if tool.Name != "oc-login" {
		t.Errorf("got name %q, want %q", tool.Name, "oc-login")
	}
	if tool.Description == "" {
		t.Error("tool description is empty")
	}
	if tool.Execute == nil {
		t.Error("tool Execute is nil")
	}
}

func TestToolSchema(t *testing.T) {
	p := newPlugin(options{})
	tool := p.Tools[0]
	params := tool.Parameters
	if params["type"] != "object" {
		t.Errorf("got type %v, want %q", params["type"], "object")
	}
	if _, ok := params["properties"].(map[string]any); !ok {
		t.Fatal("properties is not map[string]any")
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name               string
		raw                map[string]any
		server          string
		insecureSkipTLS bool
	}{
		{
			name:            "extracts all fields",
			raw:             map[string]any{"server": "https://api.test:6443", "insecureSkipTlsVerify": true},
			server:          "https://api.test:6443",
			insecureSkipTLS: true,
		},
		{
			name:            "empty map returns defaults",
			raw:             map[string]any{},
			server:          "",
			insecureSkipTLS: false,
		},
		{
			name:            "wrong type ignored",
			raw:             map[string]any{"server": 42, "insecureSkipTlsVerify": "yes"},
			server:          "",
			insecureSkipTLS: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.Server != tt.server {
				t.Errorf("Server = %q, want %q", opts.Server, tt.server)
			}
			if opts.InsecureSkipTLS != tt.insecureSkipTLS {
				t.Errorf("InsecureSkipTLS = %v, want %v", opts.InsecureSkipTLS, tt.insecureSkipTLS)
			}
		})
	}
}
