package main

import (
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "aap-bridge" {
		t.Errorf("got %q, want %q", p.ID, "aap-bridge")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 7 {
		t.Fatalf("got %d tools, want 7 (6 stub + lint)", len(p.Tools))
	}
	wantNames := []string{
		"aap_list_templates", "aap_launch_job", "aap_job_status",
		"aap_job_output", "aap_list_inventories", "aap_hub_search",
		"aap_lint_playbook",
	}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] Execute is nil", i)
		}
	}
}

func TestToolDefinitions_Configured(t *testing.T) {
	p := newPlugin(options{ControllerURL: "http://aap:8080", OAuthToken: "tok"})
	if len(p.Tools) != 7 {
		t.Fatalf("got %d tools, want 7", len(p.Tools))
	}
	if p.Hooks.ShellEnv == nil {
		t.Error("expected ShellEnv hook when configured")
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{ControllerURL: "http://aap:8080", OAuthToken: "tok"})
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
		name          string
		raw           map[string]any
		controllerURL string
		oauthToken    string
	}{
		{
			name:          "extracts all fields",
			raw:           map[string]any{"controllerUrl": "http://aap:8080", "oauthToken": "tok-abc"},
			controllerURL: "http://aap:8080",
			oauthToken:    "tok-abc",
		},
		{
			name:          "empty map",
			raw:           map[string]any{},
			controllerURL: "",
			oauthToken:    "",
		},
		{
			name:          "wrong types ignored",
			raw:           map[string]any{"controllerUrl": 123},
			controllerURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.ControllerURL != tt.controllerURL {
				t.Errorf("ControllerURL = %q, want %q", opts.ControllerURL, tt.controllerURL)
			}
			if opts.OAuthToken != tt.oauthToken {
				t.Errorf("OAuthToken = %q, want %q", opts.OAuthToken, tt.oauthToken)
			}
		})
	}
}

func TestFormatTemplates(t *testing.T) {
	tests := []struct {
		name      string
		templates []jobTemplate
		contains  []string
	}{
		{
			name:      "empty list",
			templates: nil,
			contains:  []string{"No job templates found"},
		},
		{
			name: "single template",
			templates: []jobTemplate{
				{ID: 1, Name: "Deploy App", Description: "deploys the app", Status: "successful"},
			},
			contains: []string{"Job templates: 1", "#1", "Deploy App", "deploys the app", "successful"},
		},
		{
			name: "no name falls back to unknown",
			templates: []jobTemplate{
				{ID: 2},
			},
			contains: []string{"unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTemplates(tt.templates)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatJob(t *testing.T) {
	j := &job{
		ID:       42,
		Name:     "deploy-prod",
		Status:   "successful",
		Started:  "2026-01-01T00:00:00Z",
		Finished: "2026-01-01T00:05:00Z",
		Elapsed:  300.5,
	}
	got := formatJob(j)
	for _, want := range []string{"Job #42", "deploy-prod", "successful", "2026-01-01T00:00:00Z", "300.5s"} {
		if !stringContains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatInventories(t *testing.T) {
	tests := []struct {
		name     string
		invs     []inventory
		contains []string
	}{
		{
			name:     "empty list",
			invs:     nil,
			contains: []string{"No inventories found"},
		},
		{
			name: "single inventory",
			invs: []inventory{
				{ID: 1, Name: "prod", Description: "production hosts", TotalHosts: 10, HostsWithActiveFailures: 2},
			},
			contains: []string{"Inventories: 1", "#1", "prod", "production hosts", "10 hosts", "2 failures"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatInventories(tt.invs)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatCollections(t *testing.T) {
	tests := []struct {
		name     string
		colls    []collection
		contains []string
	}{
		{
			name:     "empty list",
			colls:    nil,
			contains: []string{"No collections found"},
		},
		{
			name: "with namespace and version",
			colls: []collection{
				{
					Namespace:     &struct{ Name string `json:"name"` }{Name: "ansible"},
					Name:          "netcommon",
					Description:   "network common",
					LatestVersion: &struct{ Version string `json:"version"` }{Version: "5.1.0"},
				},
			},
			contains: []string{"Collections: 1", "ansible.netcommon", "v5.1.0", "network common"},
		},
		{
			name: "nil namespace and version",
			colls: []collection{
				{Name: "test"},
			},
			contains: []string{"unknown.test", "v?"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCollections(tt.colls)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
