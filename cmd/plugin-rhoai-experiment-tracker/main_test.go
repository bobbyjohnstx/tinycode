package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-experiment-tracker" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-experiment-tracker")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(p.Tools))
	}
	tool := p.Tools[0]
	if tool.Name != "experiment_last_session" {
		t.Errorf("got name %q, want %q", tool.Name, "experiment_last_session")
	}
	if tool.Execute == nil {
		t.Error("Execute is nil")
	}
}

func TestToolSchema(t *testing.T) {
	p := newPlugin(options{})
	tool := p.Tools[0]
	params := tool.Parameters
	if params["type"] != "object" {
		t.Errorf("params type = %v, want %q", params["type"], "object")
	}
	if _, ok := params["properties"].(map[string]any); !ok {
		t.Fatal("properties is not map[string]any")
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name           string
		raw            map[string]any
		mlflowURL      string
		experimentName string
	}{
		{
			name:           "extracts all fields",
			raw:            map[string]any{"mlflowUrl": "http://mlflow:5000", "experimentName": "my-exp"},
			mlflowURL:      "http://mlflow:5000",
			experimentName: "my-exp",
		},
		{
			name:      "empty map",
			raw:       map[string]any{},
			mlflowURL: "",
		},
		{
			name:      "wrong types ignored",
			raw:       map[string]any{"mlflowUrl": 42},
			mlflowURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.MlflowURL != tt.mlflowURL {
				t.Errorf("MlflowURL = %q, want %q", opts.MlflowURL, tt.mlflowURL)
			}
			if opts.ExperimentName != tt.experimentName {
				t.Errorf("ExperimentName = %q, want %q", opts.ExperimentName, tt.experimentName)
			}
		})
	}
}

func TestFormatLastSession(t *testing.T) {
	tests := []struct {
		name     string
		info     *lastRunInfo
		contains []string
	}{
		{
			name:     "nil info",
			info:     nil,
			contains: []string{"No previous session data"},
		},
		{
			name: "basic run info",
			info: &lastRunInfo{
				RunID:  "run-123",
				Status: "FINISHED",
			},
			contains: []string{"<last-session>", "run=run-123", "status=FINISHED", "</last-session>"},
		},
		{
			name: "with duration",
			info: &lastRunInfo{
				RunID:    "run-456",
				Status:   "FINISHED",
				Duration: floatPtr(120.0),
			},
			contains: []string{"duration=2m"},
		},
		{
			name: "with metrics and params",
			info: &lastRunInfo{
				RunID:   "run-789",
				Status:  "FINISHED",
				Metrics: map[string]float64{"accuracy": 0.95},
				Params:  map[string]string{"lr": "0.001"},
			},
			contains: []string{"metrics:", "accuracy=0.95", "params:", "lr=0.001"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLastSession(tt.info)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

func floatPtr(f float64) *float64 {
	return &f
}
