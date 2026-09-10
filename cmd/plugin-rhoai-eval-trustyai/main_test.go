package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-eval-trustyai" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-eval-trustyai")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 6 {
		t.Fatalf("got %d tools, want 6", len(p.Tools))
	}
	wantNames := []string{
		"rhoai_eval_run", "rhoai_eval_status", "rhoai_eval_compare",
		"rhoai_trusty_metrics", "rhoai_trusty_alerts", "rhoai_workbench_list",
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
		evalAPIURL string
		trustyURL  string
		namespace  string
		token      string
	}{
		{
			name: "extracts all fields",
			raw: map[string]any{
				"evalApiUrl":  "http://eval:8080",
				"trustyaiUrl": "http://trusty:8080",
				"namespace":   "ns1",
				"token":       "tok",
			},
			evalAPIURL: "http://eval:8080",
			trustyURL:  "http://trusty:8080",
			namespace:  "ns1",
			token:      "tok",
		},
		{
			name:       "empty map",
			raw:        map[string]any{},
			evalAPIURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.EvalAPIURL != tt.evalAPIURL {
				t.Errorf("EvalAPIURL = %q, want %q", opts.EvalAPIURL, tt.evalAPIURL)
			}
			if opts.TrustyAIURL != tt.trustyURL {
				t.Errorf("TrustyAIURL = %q, want %q", opts.TrustyAIURL, tt.trustyURL)
			}
			if opts.Namespace != tt.namespace {
				t.Errorf("Namespace = %q, want %q", opts.Namespace, tt.namespace)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
		})
	}
}

func TestFormatEvalResult(t *testing.T) {
	tests := []struct {
		name     string
		result   evalResult
		contains []string
	}{
		{
			name: "basic result",
			result: evalResult{
				EvalID:    "eval-1",
				Model:     "llama-3",
				Provider:  "vllm",
				Status:    "completed",
				CreatedAt: "2026-01-01",
			},
			contains: []string{"eval-1", "llama-3", "vllm", "completed", "2026-01-01"},
		},
		{
			name: "with results metrics",
			result: evalResult{
				EvalID:    "eval-2",
				Model:     "mistral",
				Provider:  "tgis",
				Status:    "completed",
				CreatedAt: "2026-01-01",
				Results: []struct {
					Metric string  `json:"metric"`
					Score  float64 `json:"score"`
					Detail string  `json:"detail,omitempty"`
				}{
					{Metric: "accuracy", Score: 0.95, Detail: "high"},
				},
			},
			contains: []string{"accuracy", "0.95", "high", "METRIC", "SCORE"},
		},
		{
			name: "with error",
			result: evalResult{
				EvalID:    "eval-3",
				Model:     "bad",
				Provider:  "x",
				Status:    "failed",
				CreatedAt: "2026-01-01",
				Error:     "model not found",
			},
			contains: []string{"Error: model not found"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEvalResult(tt.result)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatAlert(t *testing.T) {
	alert := trustyAlert{
		ID:           "a-1",
		Type:         "drift",
		Model:        "llama-3",
		Metric:       "accuracy",
		Threshold:    0.1,
		CurrentValue: 0.25,
		Severity:     "warning",
		TriggeredAt:  "2026-01-01",
	}
	got := formatAlert(alert)
	for _, want := range []string{"WARNING", "drift", "llama-3", "accuracy", "0.2500", "0.1000"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %q", want, got)
		}
	}
}
