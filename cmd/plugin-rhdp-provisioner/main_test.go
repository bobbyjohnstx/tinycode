package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhdp-provisioner" {
		t.Errorf("got %q, want %q", p.ID, "rhdp-provisioner")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(p.Tools))
	}
	wantNames := []string{"rhdp_search", "rhdp_provision", "rhdp_status", "rhdp_list_active"}
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
		name       string
		raw        map[string]any
		token      string
		rhdpApiURL string
	}{
		{
			name:       "extracts all fields",
			raw:        map[string]any{"consoleOfflineToken": "tok-abc", "rhdpApiUrl": "http://custom:8080"},
			token:      "tok-abc",
			rhdpApiURL: "http://custom:8080",
		},
		{
			name:  "empty map",
			raw:   map[string]any{},
			token: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.ConsoleOfflineToken != tt.token {
				t.Errorf("ConsoleOfflineToken = %q, want %q", opts.ConsoleOfflineToken, tt.token)
			}
			if opts.RHDPApiURL != tt.rhdpApiURL {
				t.Errorf("RHDPApiURL = %q, want %q", opts.RHDPApiURL, tt.rhdpApiURL)
			}
		})
	}
}

func TestFormatCatalogItem(t *testing.T) {
	item := catalogItem{
		ID:            "item-1",
		Name:          "OpenShift Workshop",
		Description:   "Hands-on OpenShift workshop",
		Category:      "workshop",
		Provider:      "Red Hat",
		EstimatedTime: "90 minutes",
	}
	got := formatCatalogItem(item)
	for _, want := range []string{"OpenShift Workshop", "Hands-on OpenShift workshop", "workshop", "90 minutes"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %q", want, got)
		}
	}
}

func TestFormatProvisionStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   *provisionStatus
		contains []string
	}{
		{
			name: "basic status",
			status: &provisionStatus{
				OrderID:   "ord-123",
				Status:    "provisioning",
				StartedAt: "2026-01-01T00:00:00Z",
			},
			contains: []string{"ord-123", "provisioning", "2026-01-01T00:00:00Z"},
		},
		{
			name: "ready with credentials",
			status: &provisionStatus{
				OrderID:    "ord-456",
				Status:     "ready",
				StartedAt:  "2026-01-01T00:00:00Z",
				ConsoleURL: "https://console.example.com",
				APIURL:     "https://api.example.com:6443",
				Credentials: &provisionCredentials{
					Username: "admin",
					Password: "secret123",
				},
				ExpiresAt: "2026-01-03T00:00:00Z",
			},
			contains: []string{"ord-456", "ready", "https://console.example.com", "admin", "REDACTED", "Expires"},
		},
		{
			name: "with error",
			status: &provisionStatus{
				OrderID:   "ord-789",
				Status:    "failed",
				StartedAt: "2026-01-01T00:00:00Z",
				Error:     "quota exceeded",
			},
			contains: []string{"failed", "quota exceeded"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatProvisionStatus(tt.status)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

func TestFormatActiveEnvironment(t *testing.T) {
	env := activeEnvironment{
		OrderID:         "ord-123",
		CatalogItemName: "OpenShift Workshop",
		Status:          "running",
		ConsoleURL:      "https://console.example.com",
		ExpiresAt:       "2026-01-03T00:00:00Z",
		StartedAt:       "2026-01-01T00:00:00Z",
	}
	got := formatActiveEnvironment(env)
	for _, want := range []string{"OpenShift Workshop", "running", "https://console.example.com", "Expires"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %q", want, got)
		}
	}
}
