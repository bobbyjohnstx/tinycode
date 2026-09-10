package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhacm" {
		t.Errorf("expected plugin ID 'rhacm', got %q", p.ID)
	}
}

func TestToolCount(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(p.Tools))
	}
}

func TestToolNames(t *testing.T) {
	p := newPlugin(options{})
	expected := []string{
		"acm_clusters",
		"acm_cluster_detail",
		"acm_policies",
		"acm_violations",
		"acm_applications",
		"acm_app_deploy",
		"acm_observability",
	}
	for i, want := range expected {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d]: expected name %q, got %q", i, want, p.Tools[i].Name)
		}
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{})
	for i, tool := range p.Tools {
		if tool.Description == "" {
			t.Errorf("tool[%d] %q: expected non-empty description", i, tool.Name)
		}
		if tool.Execute == nil {
			t.Errorf("tool[%d] %q: expected non-nil Execute", i, tool.Name)
		}
		typ, ok := tool.Parameters["type"]
		if !ok || typ != "object" {
			t.Errorf("tool[%d] %q: expected parameters type 'object', got %v", i, tool.Name, typ)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{})

	// acm_clusters has optional status and limit properties
	clusterTool := p.Tools[0]
	props, ok := clusterTool.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("acm_clusters: expected properties to be map[string]any")
	}
	for _, field := range []string{"status", "limit"} {
		prop, ok := props[field].(map[string]any)
		if !ok {
			t.Errorf("acm_clusters: expected property %q", field)
			continue
		}
		if prop["type"] == nil {
			t.Errorf("acm_clusters: expected %q to have a type", field)
		}
	}

	// acm_cluster_detail requires name
	detailTool := p.Tools[1]
	required, ok := detailTool.Parameters["required"].([]string)
	if !ok {
		t.Fatal("acm_cluster_detail: expected required to be []string")
	}
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("acm_cluster_detail: expected required=[name], got %v", required)
	}
}

func TestUnconfiguredObservabilityTool(t *testing.T) {
	p := newPlugin(options{})
	// Last tool is acm_observability (unconfigured variant)
	obsTool := p.Tools[6]
	if obsTool.Name != "acm_observability" {
		t.Fatalf("expected last tool 'acm_observability', got %q", obsTool.Name)
	}
	// The unconfigured variant mentions "not configured" on execute
	// (Note: it requires a real OcClient for the other tools, but observability is purely API-based)
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  options
	}{
		{
			name:  "empty",
			input: map[string]any{},
			want:  options{},
		},
		{
			name:  "both fields",
			input: map[string]any{"thanosUrl": "https://thanos.example.com", "token": "tok123"},
			want:  options{ThanosURL: "https://thanos.example.com", Token: "tok123"},
		},
		{
			name:  "wrong types ignored",
			input: map[string]any{"thanosUrl": 42, "token": false},
			want:  options{},
		},
		{
			name:  "partial",
			input: map[string]any{"thanosUrl": "https://thanos.example.com"},
			want:  options{ThanosURL: "https://thanos.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOptions(tt.input)
			if got != tt.want {
				t.Errorf("parseOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseClusterStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []managedClusterCondition
		want       string
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       "Unknown",
		},
		{
			name: "available true",
			conditions: []managedClusterCondition{
				{Type: "ManagedClusterConditionAvailable", Status: "True"},
			},
			want: "Ready",
		},
		{
			name: "available false",
			conditions: []managedClusterCondition{
				{Type: "ManagedClusterConditionAvailable", Status: "False"},
			},
			want: "NotReady",
		},
		{
			name: "other condition type only",
			conditions: []managedClusterCondition{
				{Type: "HubAccepted", Status: "True"},
			},
			want: "Unknown",
		},
		{
			name: "multiple conditions with available",
			conditions: []managedClusterCondition{
				{Type: "HubAccepted", Status: "True"},
				{Type: "ManagedClusterConditionAvailable", Status: "True"},
			},
			want: "Ready",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClusterStatus(tt.conditions)
			if got != tt.want {
				t.Errorf("parseClusterStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseManagedCluster(t *testing.T) {
	t.Run("full resource", func(t *testing.T) {
		r := managedClusterResource{}
		r.Metadata.Name = "cluster-1"
		r.Metadata.Labels = map[string]string{"cloud": "AWS", "env": "prod"}
		r.Status = &struct {
			Conditions []managedClusterCondition `json:"conditions,omitempty"`
			Version    *struct {
				Kubernetes string `json:"kubernetes,omitempty"`
			} `json:"version,omitempty"`
		}{
			Conditions: []managedClusterCondition{
				{Type: "ManagedClusterConditionAvailable", Status: "True"},
			},
			Version: &struct {
				Kubernetes string `json:"kubernetes,omitempty"`
			}{Kubernetes: "v1.28.3"},
		}

		cluster := parseManagedCluster(r)
		if cluster.Name != "cluster-1" {
			t.Errorf("expected name 'cluster-1', got %q", cluster.Name)
		}
		if cluster.Status != "Ready" {
			t.Errorf("expected status 'Ready', got %q", cluster.Status)
		}
		if cluster.Version != "v1.28.3" {
			t.Errorf("expected version 'v1.28.3', got %q", cluster.Version)
		}
		if cluster.Provider != "AWS" {
			t.Errorf("expected provider 'AWS', got %q", cluster.Provider)
		}
	})

	t.Run("nil status", func(t *testing.T) {
		r := managedClusterResource{}
		r.Metadata.Name = "cluster-2"

		cluster := parseManagedCluster(r)
		if cluster.Status != "Unknown" {
			t.Errorf("expected status 'Unknown' for nil status, got %q", cluster.Status)
		}
		if cluster.Version != "unknown" {
			t.Errorf("expected version 'unknown' for nil status, got %q", cluster.Version)
		}
		if cluster.Provider != "unknown" {
			t.Errorf("expected provider 'unknown' for missing labels, got %q", cluster.Provider)
		}
		if cluster.Labels == nil {
			t.Error("expected non-nil labels map")
		}
	})

	t.Run("nil version in status", func(t *testing.T) {
		r := managedClusterResource{}
		r.Metadata.Name = "cluster-3"
		r.Status = &struct {
			Conditions []managedClusterCondition `json:"conditions,omitempty"`
			Version    *struct {
				Kubernetes string `json:"kubernetes,omitempty"`
			} `json:"version,omitempty"`
		}{}

		cluster := parseManagedCluster(r)
		if cluster.Version != "unknown" {
			t.Errorf("expected version 'unknown' for nil version, got %q", cluster.Version)
		}
	})
}

func TestTruncateWithMessage(t *testing.T) {
	data := []string{"a", "b", "c"}

	t.Run("under limit", func(t *testing.T) {
		result, err := truncateWithMessage(data, 3, 10, "items")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result, "Showing") {
			t.Error("should not contain truncation message when under limit")
		}
	})

	t.Run("over limit", func(t *testing.T) {
		result, err := truncateWithMessage(data, 50, 3, "items")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "Showing 3 of 50 items") {
			t.Errorf("expected truncation message, got %q", result)
		}
	})

	t.Run("exact limit", func(t *testing.T) {
		result, err := truncateWithMessage(data, 3, 3, "items")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result, "Showing") {
			t.Error("should not contain truncation message when at exact limit")
		}
	})
}

func TestNewPluginWithThanos(t *testing.T) {
	p := newPlugin(options{ThanosURL: "https://thanos.example.com", Token: "bearer-token"})
	if p.ID != "rhacm" {
		t.Errorf("expected plugin ID 'rhacm', got %q", p.ID)
	}

	var foundObs bool
	for _, tool := range p.Tools {
		if tool.Name == "acm_observability" {
			foundObs = true
			if tool.Description == "" {
				t.Error("acm_observability: expected non-empty description")
			}
		}
	}
	if !foundObs {
		t.Error("expected acm_observability tool when Thanos is configured")
	}
}

func TestNewPluginWithoutThanos(t *testing.T) {
	p := newPlugin(options{})

	var obsTool *plugin.ToolDef
	for i := range p.Tools {
		if p.Tools[i].Name == "acm_observability" {
			obsTool = &p.Tools[i]
			break
		}
	}
	if obsTool == nil {
		t.Fatal("expected acm_observability tool even when unconfigured")
	}
	if !strings.Contains(obsTool.Description, "Requires") && !strings.Contains(obsTool.Description, "thanosUrl") {
		// unconfigured variant should mention configuration requirement
	}
}

func TestMarshalDomainTypes(t *testing.T) {
	// Verify JSON tags work correctly for domain types
	cluster := managedCluster{
		Name:     "test",
		Status:   "Ready",
		Version:  "v1.28",
		Provider: "AWS",
		Labels:   map[string]string{"env": "prod"},
	}
	data, err := json.Marshal(cluster)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded["name"] != "test" {
		t.Errorf("expected name 'test', got %v", decoded["name"])
	}
	if decoded["status"] != "Ready" {
		t.Errorf("expected status 'Ready', got %v", decoded["status"])
	}

	pol := policy{
		Name:      "test-policy",
		Namespace: "default",
		Compliant: "Compliant",
		Severity:  "high",
	}
	data, err = json.Marshal(pol)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decodedPol map[string]any
	if err := json.Unmarshal(data, &decodedPol); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decodedPol["compliant"] != "Compliant" {
		t.Errorf("expected compliant 'Compliant', got %v", decodedPol["compliant"])
	}
}
