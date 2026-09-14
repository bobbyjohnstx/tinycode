package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-eval-trustyai" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-eval-trustyai")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 7 {
		t.Fatalf("got %d tools, want 7", len(p.Tools))
	}
	wantNames := []string{
		"rhoai_eval_run", "rhoai_eval_status", "rhoai_eval_compare",
		"rhoai_trusty_metrics", "rhoai_trusty_alerts", "rhoai_workbench_list",
		"rhoai_eval_health",
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

func TestHealthToolExists_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	tool := findTool(&p, "rhoai_eval_health")
	if tool == nil {
		t.Fatal("rhoai_eval_health tool not found")
	}
	if tool.Execute == nil {
		t.Error("rhoai_eval_health Execute is nil")
	}
}

func TestHealthToolExists_Configured(t *testing.T) {
	p := newPlugin(options{EvalAPIURL: "http://eval:8080", TrustyAIURL: "http://trusty:8080", Token: "t"})
	tool := findTool(&p, "rhoai_eval_health")
	if tool == nil {
		t.Fatal("rhoai_eval_health tool not found")
	}
	if tool.Execute == nil {
		t.Error("rhoai_eval_health Execute is nil")
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

// --- httptest mock tests ---

func findTool(p *plugin.Plugin, name string) *plugin.ToolDef {
	for i := range p.Tools {
		if p.Tools[i].Name == name {
			return &p.Tools[i]
		}
	}
	return nil
}

func TestMock_EvalRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/evaluations" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "llama-3" {
			t.Errorf("expected model=llama-3, got %v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"eval_id":"eval-abc123"}`)
	}))
	defer srv.Close()

	p := newPlugin(options{EvalAPIURL: srv.URL, Token: "test-token"})
	tool := findTool(&p, "rhoai_eval_run")
	if tool == nil {
		t.Fatal("rhoai_eval_run tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"model":"llama-3","provider":"vllm"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "eval-abc123") {
		t.Errorf("expected eval ID in output, got: %s", result)
	}
	if !strings.Contains(result, "Evaluation started") {
		t.Errorf("expected 'Evaluation started' in output, got: %s", result)
	}
}

func TestMock_EvalStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/evaluations/eval-42" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"eval_id":"eval-42",
			"model":"mistral-7b",
			"provider":"vllm",
			"status":"completed",
			"created_at":"2026-09-14T12:00:00Z",
			"completed_at":"2026-09-14T12:05:00Z",
			"results":[
				{"metric":"accuracy","score":0.92,"detail":"good"},
				{"metric":"latency_p99","score":1.5,"detail":"ms"}
			]
		}`)
	}))
	defer srv.Close()

	p := newPlugin(options{EvalAPIURL: srv.URL, Token: "test-token"})
	tool := findTool(&p, "rhoai_eval_status")
	if tool == nil {
		t.Fatal("rhoai_eval_status tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"evalId":"eval-42"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"eval-42", "mistral-7b", "completed", "accuracy", "0.9200", "latency_p99", "1.5000"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestMock_EvalCompare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/evaluations/e1":
			fmt.Fprint(w, `{"eval_id":"e1","model":"llama","provider":"vllm","status":"completed","created_at":"2026-01-01","results":[{"metric":"acc","score":0.9}]}`)
		case "/api/v1/evaluations/e2":
			fmt.Fprint(w, `{"eval_id":"e2","model":"mistral","provider":"tgis","status":"completed","created_at":"2026-01-02","results":[{"metric":"acc","score":0.85}]}`)
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer srv.Close()

	p := newPlugin(options{EvalAPIURL: srv.URL, Token: "test-token"})
	tool := findTool(&p, "rhoai_eval_compare")
	if tool == nil {
		t.Fatal("rhoai_eval_compare tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"evalIds":["e1","e2"]}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Comparison of 2", "e1", "e2", "llama", "mistral", "0.9000", "0.8500"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestMock_EvalCompare_TooFew(t *testing.T) {
	p := newPlugin(options{EvalAPIURL: "http://unused", Token: "t"})
	tool := findTool(&p, "rhoai_eval_compare")
	if tool == nil {
		t.Fatal("rhoai_eval_compare tool not found")
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"evalIds":["e1"]}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "At least 2") {
		t.Errorf("expected minimum count message, got: %s", result)
	}
}

func TestMock_TrustyMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/models/llama-3/metrics" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"model":"llama-3",
			"driftScore":0.042,
			"biasMetrics":{"gender":0.03,"age":0.01},
			"featureDistributions":{"input_length":{"mean":128.5,"stddev":42.3}}
		}`)
	}))
	defer srv.Close()

	p := newPlugin(options{TrustyAIURL: srv.URL, Token: "test-token"})
	tool := findTool(&p, "rhoai_trusty_metrics")
	if tool == nil {
		t.Fatal("rhoai_trusty_metrics tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"model":"llama-3"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"llama-3", "0.0420", "gender", "0.0300", "age", "0.0100", "input_length", "128.5000", "42.3000"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestMock_TrustyAlerts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/alerts" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"id":"a1","type":"drift","model":"llama-3","metric":"accuracy","threshold":0.1,"currentValue":0.25,"severity":"critical","triggeredAt":"2026-09-14"},
			{"id":"a2","type":"bias","model":"mistral","metric":"gender","threshold":0.05,"currentValue":0.08,"severity":"warning","triggeredAt":"2026-09-14"}
		]`)
	}))
	defer srv.Close()

	p := newPlugin(options{TrustyAIURL: srv.URL, Token: "test-token"})
	tool := findTool(&p, "rhoai_trusty_alerts")
	if tool == nil {
		t.Fatal("rhoai_trusty_alerts tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Active Alerts: 2", "CRITICAL", "drift", "llama-3", "WARNING", "bias", "mistral"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestMock_TrustyAlerts_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	p := newPlugin(options{TrustyAIURL: srv.URL, Token: "test-token"})
	tool := findTool(&p, "rhoai_trusty_alerts")
	if tool == nil {
		t.Fatal("rhoai_trusty_alerts tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "No active TrustyAI alerts") {
		t.Errorf("expected no alerts message, got: %s", result)
	}
}
