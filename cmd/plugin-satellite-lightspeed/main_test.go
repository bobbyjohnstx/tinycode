package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "satellite-lightspeed" {
		t.Errorf("got %q, want %q", p.ID, "satellite-lightspeed")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(p.Tools))
	}
	wantNames := []string{"satellite_query", "satellite_hosts", "satellite_errata", "satellite_content_views"}
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
	p := newPlugin(options{SatelliteURL: "http://sat:8080", Token: "tok"})
	if len(p.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(p.Tools))
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
		satelliteURL string
		token        string
	}{
		{
			name:         "extracts all fields",
			raw:          map[string]any{"satelliteUrl": "http://sat:8080", "token": "tok-abc"},
			satelliteURL: "http://sat:8080",
			token:        "tok-abc",
		},
		{
			name:         "empty map",
			raw:          map[string]any{},
			satelliteURL: "",
			token:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.SatelliteURL != tt.satelliteURL {
				t.Errorf("SatelliteURL = %q, want %q", opts.SatelliteURL, tt.satelliteURL)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
		})
	}
}

func TestFormatHosts(t *testing.T) {
	tests := []struct {
		name     string
		hosts    []host
		contains []string
	}{
		{
			name:     "empty list",
			hosts:    nil,
			contains: []string{"No hosts found"},
		},
		{
			name: "single host",
			hosts: []host{
				{ID: 1, Name: "web01.example.com", OperatingsystemName: "RHEL 9.2", EnvironmentName: "production", GlobalStatusLabel: "OK"},
			},
			contains: []string{"Hosts: 1", "web01.example.com", "RHEL 9.2", "production", "OK"},
		},
		{
			name: "host with empty name",
			hosts: []host{
				{ID: 2},
			},
			contains: []string{"unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHosts(tt.hosts)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatErrata(t *testing.T) {
	tests := []struct {
		name     string
		errata   []erratum
		contains []string
	}{
		{
			name:     "empty list",
			errata:   nil,
			contains: []string{"No errata found"},
		},
		{
			name: "single erratum",
			errata: []erratum{
				{ErrataID: "RHSA-2026:0001", Title: "Critical kernel update", Type: "security", Severity: "Critical"},
			},
			contains: []string{"Errata: 1", "RHSA-2026:0001", "Critical kernel update", "security", "Critical"},
		},
		{
			name: "empty ID and title",
			errata: []erratum{
				{},
			},
			contains: []string{"unknown", "untitled"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatErrata(tt.errata)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatContentViews(t *testing.T) {
	tests := []struct {
		name     string
		views    []contentView
		contains []string
	}{
		{
			name:     "empty list",
			views:    nil,
			contains: []string{"No content views found"},
		},
		{
			name: "single view",
			views: []contentView{
				{ID: 1, Name: "RHEL-Base", Label: "rhel-base", Composite: false, LastPublished: "2026-01-01"},
			},
			contains: []string{"Content views: 1", "RHEL-Base", "rhel-base", "last published: 2026-01-01"},
		},
		{
			name: "composite view",
			views: []contentView{
				{ID: 2, Name: "Combined", Label: "combined", Composite: true},
			},
			contains: []string{"Combined", "composite"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatContentViews(tt.views)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}
