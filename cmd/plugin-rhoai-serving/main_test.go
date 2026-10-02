package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-serving" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-serving")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 12 {
		t.Fatalf("got %d tools, want 12", len(p.Tools))
	}
	wantNames := []string{
		"rhoai_list_models",
		"rhoai_model_status",
		"rhoai_list_runtimes",
		"rhoai_sandbox_status",
		"rhoai_sandbox_provision",
		"rhoai_eval_run",
		"rhoai_eval_status",
		"rhoai_eval_compare",
		"rhoai_trusty_metrics",
		"rhoai_trusty_alerts",
		"rhoai_workbench_list",
		"rhoai_serving_health",
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
		name      string
		raw       map[string]any
		namespace string
		routeHost string
		token     string
		sandboxURL string
		evalURL   string
		trustyURL string
	}{
		{
			name: "extracts all fields",
			raw: map[string]any{
				"namespace":           "my-ns",
				"routeHost":           "model.example.com",
				"consoleOfflineToken": "tok-123",
				"sandboxUrl":          "https://sandbox.example.com",
				"evalApiUrl":          "https://eval.example.com",
				"trustyaiUrl":         "https://trusty.example.com",
				"token":               "bearer-tok",
			},
			namespace:  "my-ns",
			routeHost:  "model.example.com",
			token:      "bearer-tok",
			sandboxURL: "https://sandbox.example.com",
			evalURL:    "https://eval.example.com",
			trustyURL:  "https://trusty.example.com",
		},
		{
			name:       "empty map defaults sandboxURL",
			raw:        map[string]any{},
			sandboxURL: defaultSandboxAPIBaseURL,
		},
		{
			name:       "wrong types ignored",
			raw:        map[string]any{"namespace": 42, "routeHost": true},
			sandboxURL: defaultSandboxAPIBaseURL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.Namespace != tt.namespace {
				t.Errorf("Namespace = %q, want %q", opts.Namespace, tt.namespace)
			}
			if opts.RouteHost != tt.routeHost {
				t.Errorf("RouteHost = %q, want %q", opts.RouteHost, tt.routeHost)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
			if opts.SandboxURL != tt.sandboxURL {
				t.Errorf("SandboxURL = %q, want %q", opts.SandboxURL, tt.sandboxURL)
			}
			if opts.EvalAPIURL != tt.evalURL {
				t.Errorf("EvalAPIURL = %q, want %q", opts.EvalAPIURL, tt.evalURL)
			}
			if opts.TrustyAIURL != tt.trustyURL {
				t.Errorf("TrustyAIURL = %q, want %q", opts.TrustyAIURL, tt.trustyURL)
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	tests := []struct {
		name       string
		conditions []condition
		want       bool
	}{
		{"empty", nil, false},
		{"ready", []condition{{Type: "Ready", Status: "True"}}, true},
		{"not ready", []condition{{Type: "Ready", Status: "False"}}, false},
		{"other condition", []condition{{Type: "Available", Status: "True"}}, false},
		{"ready among many", []condition{
			{Type: "Available", Status: "True"},
			{Type: "Ready", Status: "True"},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReady(tt.conditions); got != tt.want {
				t.Errorf("isReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatModelList(t *testing.T) {
	tests := []struct {
		name     string
		items    []inferenceServiceItem
		contains []string
	}{
		{
			name:     "empty",
			items:    nil,
			contains: []string{"No inference services found"},
		},
		{
			name: "single model",
			items: []inferenceServiceItem{
				func() inferenceServiceItem {
					var item inferenceServiceItem
					item.Metadata.Name = "llama"
					item.Metadata.Namespace = "ml"
					item.Status.URL = "https://llama.example.com"
					item.Status.Conditions = []condition{{Type: "Ready", Status: "True"}}
					item.Spec.Predictor.Model.ModelFormat.Name = "pytorch"
					item.Spec.Predictor.Model.Runtime = "vllm"
					return item
				}(),
			},
			contains: []string{"Inference Services: 1", "ml/llama", "Ready", "pytorch", "vllm", "https://llama.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatModelList(tt.items)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatModelStatus(t *testing.T) {
	var item inferenceServiceItem
	item.Metadata.Name = "gpt"
	item.Metadata.Namespace = "prod"
	item.Status.URL = "https://gpt.example.com"
	item.Status.Conditions = []condition{{Type: "Ready", Status: "True"}}

	pods := []podItem{
		func() podItem {
			var p podItem
			p.Metadata.Name = "gpt-predictor-abc"
			p.Status.Phase = "Running"
			return p
		}(),
	}

	got := formatModelStatus(item, pods)
	for _, want := range []string{"prod/gpt", "Ready", "https://gpt.example.com", "gpt-predictor-abc", "Running"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatRuntimeList(t *testing.T) {
	tests := []struct {
		name     string
		items    []servingRuntimeItem
		contains []string
	}{
		{
			name:     "empty",
			items:    nil,
			contains: []string{"No serving runtimes found"},
		},
		{
			name: "single runtime",
			items: []servingRuntimeItem{
				func() servingRuntimeItem {
					var item servingRuntimeItem
					item.Metadata.Name = "vllm"
					item.Metadata.Namespace = "ml"
					item.Spec.SupportedModelFormats = []struct {
						Name string `json:"name"`
					}{{Name: "pytorch"}, {Name: "onnx"}}
					item.Spec.Containers = []struct {
						Image string `json:"image"`
					}{{Image: "quay.io/vllm:latest"}}
					return item
				}(),
			},
			contains: []string{"Serving Runtimes: 1", "ml/vllm", "pytorch", "onnx", "quay.io/vllm:latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRuntimeList(tt.items)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatSandboxStatus(t *testing.T) {
	t.Run("not ready", func(t *testing.T) {
		resp := sandboxSignupResponse{}
		resp.Status.Ready = false
		resp.Status.Reason = "PendingApproval"
		got := formatSandboxStatus(resp)
		if !strings.Contains(got, "pending") {
			t.Errorf("expected pending state, got %q", got)
		}
	})

	t.Run("not signed up", func(t *testing.T) {
		resp := sandboxSignupResponse{}
		resp.Status.Ready = false
		resp.Status.Reason = "NotSignedUp"
		got := formatSandboxStatus(resp)
		if !strings.Contains(got, "not-registered") {
			t.Errorf("expected not-registered, got %q", got)
		}
	})

	t.Run("ready", func(t *testing.T) {
		resp := sandboxSignupResponse{
			ClusterName:      "cluster-1",
			CompliantUsername: "user1",
			ConsoleURL:       "https://console.example.com",
			APIEndpoint:      "https://api.example.com",
			StartDate:        "2026-01-01",
		}
		resp.Status.Ready = true
		got := formatSandboxStatus(resp)
		for _, want := range []string{"ready", "cluster-1", "user1-dev", "https://console.example.com"} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q in:\n%s", want, got)
			}
		}
	})
}

func TestFormatEvalResult(t *testing.T) {
	r := evalResult{
		EvalID:    "eval-1",
		Model:     "llama-3",
		Provider:  "vllm",
		Status:    "completed",
		CreatedAt: "2026-01-01",
		Results: []struct {
			Metric string  `json:"metric"`
			Score  float64 `json:"score"`
			Detail string  `json:"detail,omitempty"`
		}{
			{Metric: "accuracy", Score: 0.95, Detail: "high"},
		},
	}
	got := formatEvalResult(r)
	for _, want := range []string{"eval-1", "llama-3", "vllm", "completed", "accuracy", "0.9500", "high"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatAlert(t *testing.T) {
	a := trustyAlert{
		ID:           "a-1",
		Type:         "drift",
		Model:        "my-model",
		Metric:       "psi",
		Threshold:    0.1,
		CurrentValue: 0.25,
		Severity:     "high",
	}
	got := formatAlert(a)
	for _, want := range []string{"HIGH", "drift", "my-model", "psi", "0.2500", "0.1000"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

// --- httptest mock-based tests ---

func TestSandboxStatus_Mock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":{"ready":true},"clusterName":"cluster-a","compliantUsername":"dev1","consoleURL":"https://console.test","apiEndpoint":"https://api.test","startDate":"2026-01-01"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL})
	tools := buildSandboxTools(client, "/api/v1")

	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"ready", "cluster-a", "dev1-dev"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestSandboxProvision_Mock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":{"ready":false,"reason":"PendingApproval"}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL})
	tools := buildSandboxTools(client, "/api/v1")

	result, err := tools[1].Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "provisioning initiated") {
		t.Errorf("expected provisioning message, got %q", result)
	}
}

func TestEvalRun_Mock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/evaluations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"eval_id":"eval-abc"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL})
	tools := buildEvalTools(client)

	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{"model":"llama","provider":"vllm"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "eval-abc") {
		t.Errorf("expected eval ID in output, got %q", result)
	}
}

func TestEvalStatus_Mock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/evaluations/eval-abc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"eval_id":"eval-abc","model":"llama","provider":"vllm","status":"completed","created_at":"2026-01-01","results":[{"metric":"accuracy","score":0.95}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL})
	tools := buildEvalTools(client)

	result, err := tools[1].Execute(context.Background(), json.RawMessage(`{"evalId":"eval-abc"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"eval-abc", "llama", "completed", "accuracy"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestSandboxTools_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer srv.Close()

	client := redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL})
	tools := buildSandboxTools(client, "/api/v1")

	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Failed") {
		t.Errorf("expected failure message, got %q", result)
	}
}
