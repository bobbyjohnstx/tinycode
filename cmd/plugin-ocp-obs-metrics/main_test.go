package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "ocp-obs-metrics" {
		t.Errorf("got %q, want %q", p.ID, "ocp-obs-metrics")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(p.Tools))
	}
	wantNames := []string{"obs_promql", "obs_alerts", "obs_alert_silence"}
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
		name            string
		raw             map[string]any
		prometheusURL   string
		alertManagerURL string
		token           string
		namespace       string
	}{
		{
			name: "extracts all fields",
			raw: map[string]any{
				"prometheusUrl":   "http://prom:9090",
				"alertManagerUrl": "http://am:9093",
				"token":           "tok",
				"namespace":       "monitoring",
			},
			prometheusURL:   "http://prom:9090",
			alertManagerURL: "http://am:9093",
			token:           "tok",
			namespace:       "monitoring",
		},
		{
			name:          "empty map",
			raw:           map[string]any{},
			prometheusURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.PrometheusURL != tt.prometheusURL {
				t.Errorf("PrometheusURL = %q, want %q", opts.PrometheusURL, tt.prometheusURL)
			}
			if opts.AlertManagerURL != tt.alertManagerURL {
				t.Errorf("AlertManagerURL = %q, want %q", opts.AlertManagerURL, tt.alertManagerURL)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
			if opts.Namespace != tt.namespace {
				t.Errorf("Namespace = %q, want %q", opts.Namespace, tt.namespace)
			}
		})
	}
}

func TestFormatVectorResult(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		contains []string
	}{
		{
			name:     "empty results",
			data:     `[]`,
			contains: []string{"no results"},
		},
		{
			name:     "single vector",
			data:     `[{"metric":{"__name__":"up","instance":"localhost:9090"},"value":[1234567890,"1"]}]`,
			contains: []string{"Results: 1 vectors", "=> 1"},
		},
		{
			name:     "invalid JSON",
			data:     `not-json`,
			contains: []string{"Failed to parse"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatVectorResult(json.RawMessage(tt.data))
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatMatrixResult(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		contains []string
	}{
		{
			name:     "empty results",
			data:     `[]`,
			contains: []string{"no results"},
		},
		{
			name:     "single series",
			data:     `[{"metric":{"__name__":"up"},"values":[[1234567890,"1"],[1234567900,"0"]]}]`,
			contains: []string{"Results: 1 series"},
		},
		{
			name:     "invalid JSON",
			data:     `{bad}`,
			contains: []string{"Failed to parse"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMatrixResult(json.RawMessage(tt.data))
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatAlertSummary(t *testing.T) {
	tests := []struct {
		name     string
		alerts   []redhat.Alert
		contains []string
	}{
		{
			name:     "no alerts returns none",
			alerts:   nil,
			contains: []string{"none"},
		},
		{
			name: "groups by severity",
			alerts: []redhat.Alert{
				{Labels: map[string]string{"severity": "critical", "alertname": "HighCPU"}},
				{Labels: map[string]string{"severity": "warning", "alertname": "DiskLow"}},
				{Labels: map[string]string{"severity": "critical", "alertname": "OOMKilled"}},
			},
			contains: []string{"2 critical", "HighCPU", "OOMKilled", "1 warning", "DiskLow"},
		},
		{
			name: "missing severity drops to unknown bucket and is excluded from output",
			alerts: []redhat.Alert{
				{Labels: map[string]string{"alertname": "TestAlert"}},
			},
			contains: []string{"none"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAlertSummary(tt.alerts)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}
