package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "ocp-context-injection" {
		t.Errorf("got %q, want %q", p.ID, "ocp-context-injection")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(p.Tools))
	}
	tool := p.Tools[0]
	if tool.Name != "cluster_context" {
		t.Errorf("got name %q, want %q", tool.Name, "cluster_context")
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
		name  string
		raw   map[string]any
		token string
	}{
		{
			name:  "extracts consoleOfflineToken",
			raw:   map[string]any{"consoleOfflineToken": "tok-123"},
			token: "tok-123",
		},
		{
			name:  "empty map returns zero options",
			raw:   map[string]any{},
			token: "",
		},
		{
			name:  "wrong type ignored",
			raw:   map[string]any{"consoleOfflineToken": 42},
			token: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.ConsoleOfflineToken != tt.token {
				t.Errorf("ConsoleOfflineToken = %q, want %q", opts.ConsoleOfflineToken, tt.token)
			}
		})
	}
}

func TestFormatAlertLine(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		entries  []alertEntry
		contains []string
	}{
		{
			name:     "single entry with namespace",
			label:    "firing-alerts-critical",
			entries:  []alertEntry{{Name: "HighCPU", Namespace: "openshift-monitoring"}},
			contains: []string{"firing-alerts-critical: 1", "HighCPU: openshift-monitoring"},
		},
		{
			name:     "entry with empty namespace defaults to cluster",
			label:    "firing-alerts-warning",
			entries:  []alertEntry{{Name: "DiskFull", Namespace: ""}},
			contains: []string{"firing-alerts-warning: 1", "DiskFull: cluster"},
		},
		{
			name:  "multiple entries",
			label: "alerts",
			entries: []alertEntry{
				{Name: "A", Namespace: "ns1"},
				{Name: "B", Namespace: "ns2"},
			},
			contains: []string{"alerts: 2", "A: ns1", "B: ns2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAlertLine(tt.label, tt.entries)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

func TestFormatContextBlock(t *testing.T) {
	t.Run("basic context", func(t *testing.T) {
		cc := &clusterContext{
			Cluster:   "my-cluster",
			Version:   "4.14.5",
			Nodes:     "6 (3 control-plane, 3 worker)",
			Namespace: "default",
			Operators: []string{"OLM", "Prometheus"},
		}
		got := formatContextBlock(cc)
		for _, want := range []string{
			"<cluster-context>",
			"</cluster-context>",
			"cluster: my-cluster",
			"version: 4.14.5",
			"nodes: 6 (3 control-plane, 3 worker)",
			"namespace: default",
			"OLM, Prometheus",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q", want)
			}
		}
	})

	t.Run("context with alerts", func(t *testing.T) {
		cc := &clusterContext{
			Cluster:   "test",
			Version:   "4.15.0",
			Nodes:     "3",
			Namespace: "ns1",
			Alerts: &alertSummary{
				Critical: []alertEntry{{Name: "CritAlert", Namespace: "ns1"}},
				Warning:  []alertEntry{{Name: "WarnAlert", Namespace: ""}},
				Info:     5,
			},
		}
		got := formatContextBlock(cc)
		for _, want := range []string{"firing-alerts-critical", "CritAlert", "firing-alerts-warning", "WarnAlert", "firing-alerts-info: 5"} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q", want)
			}
		}
	})
}

func TestFormatCostBlock(t *testing.T) {
	tests := []struct {
		name     string
		cost     *costContext
		contains []string
	}{
		{
			name: "USD with namespace and top resource",
			cost: &costContext{
				Namespace:       "prod",
				MonthlyEstimate: "123.45",
				TopResource:     "compute",
				TopResourceCost: "89.00",
				Currency:        "USD",
			},
			contains: []string{"<cost-context>", "</cost-context>", "namespace=prod", "monthly-cost=$123.45", "top-resource=compute ($89.00/mo)"},
		},
		{
			name: "non-USD currency no dollar prefix",
			cost: &costContext{
				MonthlyEstimate: "100.00",
				Currency:        "EUR",
			},
			contains: []string{"monthly-cost=100.00"},
		},
		{
			name: "no top resource",
			cost: &costContext{
				MonthlyEstimate: "50.00",
				Currency:        "USD",
			},
			contains: []string{"monthly-cost=$50.00"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCostBlock(tt.cost)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}
