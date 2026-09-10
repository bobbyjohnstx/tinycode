package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "rh-ecosystem-catalog" {
		t.Errorf("got %q, want %q", p.ID, "rh-ecosystem-catalog")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(p.Tools))
	}
	wantNames := []string{"ecosystem_search", "ecosystem_operator", "ecosystem_browse"}
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
	p := newPlugin()
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

func TestFormatRepo(t *testing.T) {
	tests := []struct {
		name     string
		repo     pyxisRepo
		contains []string
	}{
		{
			name: "full repo",
			repo: pyxisRepo{
				Repository: "ubi9/ubi",
				Registry:   "registry.access.redhat.com",
				DisplayData: &pyxisDisplayData{
					Name:             "UBI 9",
					ShortDescription: "Universal Base Image 9",
				},
				ApplicationCategories: []string{"base-image"},
				LastUpdateDate:        "2026-01-15T00:00:00Z",
			},
			contains: []string{"registry.access.redhat.com/ubi9/ubi", "UBI 9", "Universal Base Image 9", "base-image", "2026-01-15"},
		},
		{
			name: "minimal repo",
			repo: pyxisRepo{
				Repository: "test/app",
			},
			contains: []string{"registry.redhat.com/test/app"},
		},
		{
			name:     "empty repo",
			repo:     pyxisRepo{},
			contains: []string{"registry.redhat.com/unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRepo(tt.repo)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

func TestFormatOperator(t *testing.T) {
	tests := []struct {
		name     string
		op       pyxisOperatorBundle
		contains []string
	}{
		{
			name: "full operator",
			op: pyxisOperatorBundle{
				CSVDisplayName: "AMQ Streams",
				Package:        "amq-streams",
				Version:        "2.5.0",
				OCPVersion:     "4.14",
				Organization:   "Red Hat",
				ChannelName:    "stable",
			},
			contains: []string{"AMQ Streams", "amq-streams", "2.5.0", "OCP: 4.14", "Red Hat", "stable"},
		},
		{
			name: "minimal operator",
			op: pyxisOperatorBundle{
				Package: "test-op",
			},
			contains: []string{"test-op", "unknown"},
		},
		{
			name:     "empty operator",
			op:       pyxisOperatorBundle{},
			contains: []string{"unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatOperator(tt.op)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}
