package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-model-serving" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-model-serving")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 5 {
		t.Fatalf("got %d tools, want 5", len(p.Tools))
	}
	wantNames := []string{
		"rhoai_list_models", "rhoai_model_status", "rhoai_list_runtimes",
		"rhoai_sandbox_status", "rhoai_sandbox_provision",
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
	}{
		{
			name: "extracts all fields",
			raw: map[string]any{
				"namespace":           "ns1",
				"routeHost":           "https://route.example.com",
				"consoleOfflineToken": "tok-abc",
			},
			namespace: "ns1",
			routeHost: "https://route.example.com",
			token:     "tok-abc",
		},
		{
			name:      "empty map",
			raw:       map[string]any{},
			namespace: "",
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
			if opts.ConsoleOfflineToken != tt.token {
				t.Errorf("ConsoleOfflineToken = %q, want %q", opts.ConsoleOfflineToken, tt.token)
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
		{
			name:       "empty conditions",
			conditions: nil,
			want:       false,
		},
		{
			name: "ready condition true",
			conditions: []condition{
				{Type: "Ready", Status: "True"},
			},
			want: true,
		},
		{
			name: "ready condition false",
			conditions: []condition{
				{Type: "Ready", Status: "False"},
			},
			want: false,
		},
		{
			name: "no ready condition",
			conditions: []condition{
				{Type: "Available", Status: "True"},
			},
			want: false,
		},
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
			name:     "empty list",
			items:    nil,
			contains: []string{"No inference services found"},
		},
		{
			name: "single model",
			items: []inferenceServiceItem{
				{
					Metadata: struct {
						Name      string `json:"name"`
						Namespace string `json:"namespace"`
					}{Name: "llama-3", Namespace: "ml"},
					Spec: struct {
						Predictor struct {
							Model struct {
								ModelFormat struct {
									Name string `json:"name"`
								} `json:"modelFormat"`
								Runtime string `json:"runtime"`
							} `json:"model"`
						} `json:"predictor"`
					}{Predictor: struct {
						Model struct {
							ModelFormat struct {
								Name string `json:"name"`
							} `json:"modelFormat"`
							Runtime string `json:"runtime"`
						} `json:"model"`
					}{Model: struct {
						ModelFormat struct {
							Name string `json:"name"`
						} `json:"modelFormat"`
						Runtime string `json:"runtime"`
					}{ModelFormat: struct {
						Name string `json:"name"`
					}{Name: "onnx"}, Runtime: "vllm"}}},
					Status: struct {
						URL        string      `json:"url"`
						Conditions []condition `json:"conditions"`
					}{
						URL:        "https://model.example.com",
						Conditions: []condition{{Type: "Ready", Status: "True"}},
					},
				},
			},
			contains: []string{"Inference Services: 1", "ml/llama-3", "Ready", "onnx", "vllm", "https://model.example.com"},
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

func TestFormatRuntimeList(t *testing.T) {
	tests := []struct {
		name     string
		items    []servingRuntimeItem
		contains []string
	}{
		{
			name:     "empty list",
			items:    nil,
			contains: []string{"No serving runtimes found"},
		},
		{
			name: "single runtime",
			items: []servingRuntimeItem{
				{
					Metadata: struct {
						Name      string `json:"name"`
						Namespace string `json:"namespace"`
					}{Name: "vllm-runtime", Namespace: "ml"},
					Spec: struct {
						SupportedModelFormats []struct {
							Name string `json:"name"`
						} `json:"supportedModelFormats"`
						Containers []struct {
							Image string `json:"image"`
						} `json:"containers"`
					}{
						SupportedModelFormats: []struct {
							Name string `json:"name"`
						}{{Name: "onnx"}, {Name: "pytorch"}},
						Containers: []struct {
							Image string `json:"image"`
						}{{Image: "quay.io/opendatahub/vllm:latest"}},
					},
				},
			},
			contains: []string{"Serving Runtimes: 1", "ml/vllm-runtime", "onnx, pytorch", "quay.io/opendatahub/vllm:latest"},
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
	tests := []struct {
		name     string
		resp     sandboxSignupResponse
		contains []string
	}{
		{
			name: "not signed up",
			resp: sandboxSignupResponse{
				Status: struct {
					Ready              bool   `json:"ready"`
					Reason             string `json:"reason"`
					VerificationDigits string `json:"verificationDigits"`
				}{Ready: false, Reason: "NotSignedUp"},
			},
			contains: []string{"not-registered", "NotSignedUp"},
		},
		{
			name: "ready",
			resp: sandboxSignupResponse{
				Status: struct {
					Ready              bool   `json:"ready"`
					Reason             string `json:"reason"`
					VerificationDigits string `json:"verificationDigits"`
				}{Ready: true},
				ClusterName:      "cluster-1",
				CompliantUsername: "user1",
				ConsoleURL:       "https://console.example.com",
				APIEndpoint:      "https://api.example.com:6443",
				StartDate:        "2026-01-01",
			},
			contains: []string{"ready", "cluster-1", "user1-dev", "https://console.example.com", "2026-01-01"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSandboxStatus(tt.resp)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}
