package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "ocp-obs-logging" {
		t.Errorf("got %q, want %q", p.ID, "ocp-obs-logging")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 5 {
		t.Fatalf("got %d tools, want 5", len(p.Tools))
	}
	wantNames := []string{
		"obs_logs", "obs_traces", "obs_trace_detail",
		"obs_flow_collectors", "obs_dashboards",
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

func TestToolDefinitions_FullyConfigured(t *testing.T) {
	p := newPlugin(options{LokiURL: "http://loki:3100", TempoURL: "http://tempo:3200", Token: "tok"})
	if len(p.Tools) != 5 {
		t.Fatalf("got %d tools, want 5", len(p.Tools))
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
		name    string
		raw     map[string]any
		lokiURL string
		tempoURL string
		token   string
	}{
		{
			name:     "extracts all fields",
			raw:      map[string]any{"lokiUrl": "http://loki:3100", "tempoUrl": "http://tempo:3200", "token": "tok"},
			lokiURL:  "http://loki:3100",
			tempoURL: "http://tempo:3200",
			token:    "tok",
		},
		{
			name:     "empty map",
			raw:      map[string]any{},
			lokiURL:  "",
			tempoURL: "",
			token:    "",
		},
		{
			name:    "wrong types ignored",
			raw:     map[string]any{"lokiUrl": 123},
			lokiURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.LokiURL != tt.lokiURL {
				t.Errorf("LokiURL = %q, want %q", opts.LokiURL, tt.lokiURL)
			}
			if opts.TempoURL != tt.tempoURL {
				t.Errorf("TempoURL = %q, want %q", opts.TempoURL, tt.tempoURL)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
		})
	}
}

func TestFormatLogEntries(t *testing.T) {
	tests := []struct {
		name     string
		entries  []logEntry
		contains []string
	}{
		{
			name:     "empty entries",
			entries:  nil,
			contains: []string{"No log entries found"},
		},
		{
			name: "single entry",
			entries: []logEntry{
				{Timestamp: "1234567890", Line: "error happened", Labels: map[string]string{"namespace": "prod"}},
			},
			contains: []string{"Log entries: 1", "1234567890", "error happened", "namespace"},
		},
		{
			name: "multiple entries",
			entries: []logEntry{
				{Timestamp: "1", Line: "line1", Labels: map[string]string{}},
				{Timestamp: "2", Line: "line2", Labels: map[string]string{}},
			},
			contains: []string{"Log entries: 2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLogEntries(tt.entries)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestBuildLogQL(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		pod       string
		severity  string
		want      string
	}{
		{
			name: "no filters",
			want: `{job=~".+"}`,
		},
		{
			name:      "namespace only",
			namespace: "prod",
			want:      `{namespace="prod"}`,
		},
		{
			name:      "namespace and pod",
			namespace: "prod",
			pod:       "app-1",
			want:      `{namespace="prod", pod="app-1"}`,
		},
		{
			name:     "severity filter",
			severity: "error",
			want:     `{job=~".+"} |= "error"`,
		},
		{
			name:      "all filters",
			namespace: "ns",
			pod:       "pod-1",
			severity:  "warning",
			want:      `{namespace="ns", pod="pod-1"} |= "warning"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLogQL(tt.namespace, tt.pod, tt.severity)
			if got != tt.want {
				t.Errorf("buildLogQL(%q, %q, %q) = %q, want %q",
					tt.namespace, tt.pod, tt.severity, got, tt.want)
			}
		})
	}
}

func TestFormatSpanTree(t *testing.T) {
	t.Run("single span", func(t *testing.T) {
		spans := []span{
			{ServiceName: "api", OperationName: "GET /users", Duration: 150},
		}
		got := formatSpanTree(spans, 0)
		if !strings.Contains(got, "[150ms] api:GET /users") {
			t.Errorf("unexpected output: %q", got)
		}
	})

	t.Run("nested spans", func(t *testing.T) {
		spans := []span{
			{
				ServiceName:   "api",
				OperationName: "GET /users",
				Duration:      200,
				Children: []span{
					{ServiceName: "db", OperationName: "SELECT", Duration: 50},
				},
			},
		}
		got := formatSpanTree(spans, 0)
		if !strings.Contains(got, "[200ms] api:GET /users") {
			t.Errorf("missing parent span")
		}
		if !strings.Contains(got, "  [50ms] db:SELECT") {
			t.Errorf("missing indented child span")
		}
	})

	t.Run("indentation", func(t *testing.T) {
		spans := []span{
			{ServiceName: "svc", OperationName: "op", Duration: 10},
		}
		got := formatSpanTree(spans, 2)
		if !strings.HasPrefix(got, "    ") {
			t.Errorf("expected 4-space indent for level 2, got: %q", got)
		}
	})
}
