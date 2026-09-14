package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
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
		t.Fatalf("got %d tools, want 3 (unconfigured has no health tool)", len(p.Tools))
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

// --- httptest mock tests ---

func newMockPromQLClient(promHandler, amHandler http.Handler) (*redhat.PromQLClient, *httptest.Server, *httptest.Server) {
	promSrv := httptest.NewServer(promHandler)
	amSrv := httptest.NewServer(amHandler)
	client := redhat.NewPromQLClient(redhat.PromQLClientConfig{
		BaseURL:         promSrv.URL,
		AlertManagerURL: amSrv.URL,
	})
	return client, promSrv, amSrv
}

func findTool(tools []plugin.ToolDef, name string) *plugin.ToolDef {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

func TestMock_PromQL_InstantQuery(t *testing.T) {
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.Error(w, "not found", 404)
			return
		}
		q := r.URL.Query().Get("query")
		if q == "" {
			http.Error(w, "missing query", 400)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up","job":"prometheus"},"value":[1234567890,"1"]},{"metric":{"__name__":"up","job":"node"},"value":[1234567890,"0"]}]}}`)
	})
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	tools := buildObsTools(client)
	tool := findTool(tools, "obs_promql")
	if tool == nil {
		t.Fatal("obs_promql tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"up"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "2 vectors") {
		t.Errorf("expected '2 vectors', got:\n%s", result)
	}
	if !strings.Contains(result, "prometheus") {
		t.Errorf("expected 'prometheus' label, got:\n%s", result)
	}
}

func TestMock_PromQL_RangeQuery(t *testing.T) {
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			http.Error(w, "not found", 404)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"cpu_usage"},"values":[[1000,"0.5"],[1060,"0.7"],[1120,"0.6"]]}]}}`)
	})
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	tools := buildObsTools(client)
	tool := findTool(tools, "obs_promql")
	if tool == nil {
		t.Fatal("obs_promql tool not found")
	}

	args := `{"query":"cpu_usage","start":"2026-01-01T00:00:00Z","end":"2026-01-01T01:00:00Z","step":"60s"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "1 series") {
		t.Errorf("expected '1 series', got:\n%s", result)
	}
	if !strings.Contains(result, "cpu_usage") {
		t.Errorf("expected 'cpu_usage' label, got:\n%s", result)
	}
}

func TestMock_PromQL_EmptyResults(t *testing.T) {
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	})
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	tools := buildObsTools(client)
	tool := findTool(tools, "obs_promql")
	if tool == nil {
		t.Fatal("obs_promql tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"nonexistent_metric"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "no results") {
		t.Errorf("expected 'no results', got:\n%s", result)
	}
}

func TestMock_Alerts_WithSeverityFilter(t *testing.T) {
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			http.Error(w, "not found", 404)
			return
		}
		fmt.Fprint(w, `[
			{"labels":{"alertname":"HighCPU","severity":"critical","namespace":"monitoring"},"annotations":{"description":"CPU too high"},"state":"firing","activeAt":"2026-01-01T00:00:00Z"},
			{"labels":{"alertname":"DiskLow","severity":"warning","namespace":"storage"},"annotations":{"description":"Disk space low"},"state":"firing","activeAt":"2026-01-01T01:00:00Z"},
			{"labels":{"alertname":"OOMKilled","severity":"critical","namespace":"apps"},"annotations":{"description":"OOM killed"},"state":"firing","activeAt":"2026-01-01T02:00:00Z"}
		]`)
	})
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	tools := buildObsTools(client)
	tool := findTool(tools, "obs_alerts")
	if tool == nil {
		t.Fatal("obs_alerts tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"severity":"critical"}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Active Alerts: 2") {
		t.Errorf("expected 'Active Alerts: 2', got:\n%s", result)
	}
	if !strings.Contains(result, "HighCPU") {
		t.Errorf("expected 'HighCPU', got:\n%s", result)
	}
	if !strings.Contains(result, "OOMKilled") {
		t.Errorf("expected 'OOMKilled', got:\n%s", result)
	}
	if strings.Contains(result, "DiskLow") {
		t.Errorf("should not contain 'DiskLow' (warning severity), got:\n%s", result)
	}
}

func TestMock_Alerts_NoAlerts(t *testing.T) {
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	})
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	tools := buildObsTools(client)
	tool := findTool(tools, "obs_alerts")
	if tool == nil {
		t.Fatal("obs_alerts tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No active alerts") {
		t.Errorf("expected 'No active alerts', got:\n%s", result)
	}
}

func TestMock_Health_AllUp(t *testing.T) {
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/query" {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1234567890,"1"]}]}}`)
			return
		}
		http.Error(w, "not found", 404)
	})
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/alerts" {
			fmt.Fprint(w, `[]`)
			return
		}
		http.Error(w, "not found", 404)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	healthTool := buildHealthTool(client)
	result, err := healthTool.Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "[OK] Prometheus/Thanos") {
		t.Errorf("expected '[OK] Prometheus/Thanos', got:\n%s", result)
	}
	if !strings.Contains(result, "[OK] AlertManager") {
		t.Errorf("expected '[OK] AlertManager', got:\n%s", result)
	}
	if !strings.Contains(result, "Service Health:") {
		t.Errorf("expected 'Service Health:', got:\n%s", result)
	}
}

func TestMock_Health_PromDown(t *testing.T) {
	promHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`error`))
	})
	amHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/alerts" {
			fmt.Fprint(w, `[]`)
			return
		}
		http.Error(w, "not found", 404)
	})

	client, promSrv, amSrv := newMockPromQLClient(promHandler, amHandler)
	defer promSrv.Close()
	defer amSrv.Close()

	healthTool := buildHealthTool(client)
	result, err := healthTool.Execute(context.Background(), json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "[DOWN] Prometheus/Thanos") {
		t.Errorf("expected '[DOWN] Prometheus/Thanos', got:\n%s", result)
	}
	if !strings.Contains(result, "[OK] AlertManager") {
		t.Errorf("expected '[OK] AlertManager', got:\n%s", result)
	}
}
