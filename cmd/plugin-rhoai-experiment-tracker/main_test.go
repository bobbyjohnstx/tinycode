package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
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

func newMockAPI(handler http.Handler) (*redhat.APIClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}), srv
}

func TestFetchLastRun_Success(t *testing.T) {
	api, srv := newMockAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/2.0/mlflow/experiments/get-by-name":
			if r.URL.Query().Get("experiment_name") != "test-exp" {
				t.Errorf("unexpected experiment_name: %s", r.URL.Query().Get("experiment_name"))
			}
			json.NewEncoder(w).Encode(map[string]any{
				"experiment": map[string]any{"experiment_id": "exp-1"},
			})
		case r.URL.Path == "/api/2.0/mlflow/runs/search":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			endTime := int64(1700001120000)
			json.NewEncoder(w).Encode(map[string]any{
				"runs": []map[string]any{
					{
						"info": map[string]any{
							"run_id":     "run-abc",
							"status":     "FINISHED",
							"start_time": 1700001000000,
							"end_time":   endTime,
						},
						"data": map[string]any{
							"metrics": []map[string]any{
								{"key": "accuracy", "value": 0.95},
								{"key": "loss", "value": 0.05},
							},
							"params": []map[string]any{
								{"key": "lr", "value": "0.001"},
								{"key": "epochs", "value": "10"},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	info, err := fetchLastRun(context.Background(), api, "test-exp", "/api/2.0/mlflow")
	if err != nil {
		t.Fatalf("fetchLastRun returned error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil lastRunInfo")
	}
	if info.RunID != "run-abc" {
		t.Errorf("RunID = %q, want %q", info.RunID, "run-abc")
	}
	if info.Status != "FINISHED" {
		t.Errorf("Status = %q, want %q", info.Status, "FINISHED")
	}
	if info.Metrics["accuracy"] != 0.95 {
		t.Errorf("accuracy = %v, want 0.95", info.Metrics["accuracy"])
	}
	if info.Metrics["loss"] != 0.05 {
		t.Errorf("loss = %v, want 0.05", info.Metrics["loss"])
	}
	if info.Params["lr"] != "0.001" {
		t.Errorf("lr = %q, want %q", info.Params["lr"], "0.001")
	}
	if info.Params["epochs"] != "10" {
		t.Errorf("epochs = %q, want %q", info.Params["epochs"], "10")
	}
	if info.Duration == nil {
		t.Fatal("expected non-nil duration")
	}
	if *info.Duration != 120.0 {
		t.Errorf("Duration = %v, want 120.0", *info.Duration)
	}
}

func TestFetchLastRun_NoRuns(t *testing.T) {
	api, srv := newMockAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/2.0/mlflow/experiments/get-by-name":
			json.NewEncoder(w).Encode(map[string]any{
				"experiment": map[string]any{"experiment_id": "exp-2"},
			})
		case r.URL.Path == "/api/2.0/mlflow/runs/search":
			json.NewEncoder(w).Encode(map[string]any{
				"runs": []map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	info, err := fetchLastRun(context.Background(), api, "empty-exp", "/api/2.0/mlflow")
	if err != nil {
		t.Fatalf("fetchLastRun returned error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil lastRunInfo, got %+v", info)
	}
}

func TestFetchLastRun_ExperimentNotFound(t *testing.T) {
	api, srv := newMockAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error_code":"RESOURCE_DOES_NOT_EXIST","message":"not found"}`))
	}))
	defer srv.Close()

	_, err := fetchLastRun(context.Background(), api, "missing-exp", "/api/2.0/mlflow")
	if err == nil {
		t.Fatal("expected error for missing experiment")
	}
}

func TestFetchLastRun_NoDuration(t *testing.T) {
	api, srv := newMockAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/2.0/mlflow/experiments/get-by-name":
			json.NewEncoder(w).Encode(map[string]any{
				"experiment": map[string]any{"experiment_id": "exp-3"},
			})
		case r.URL.Path == "/api/2.0/mlflow/runs/search":
			json.NewEncoder(w).Encode(map[string]any{
				"runs": []map[string]any{
					{
						"info": map[string]any{
							"run_id":     "run-no-end",
							"status":     "RUNNING",
							"start_time": 1700001000000,
						},
						"data": map[string]any{
							"metrics": []map[string]any{},
							"params":  []map[string]any{},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	info, err := fetchLastRun(context.Background(), api, "running-exp", "/api/2.0/mlflow")
	if err != nil {
		t.Fatalf("fetchLastRun returned error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil lastRunInfo")
	}
	if info.Duration != nil {
		t.Errorf("expected nil Duration for run without end_time, got %v", *info.Duration)
	}
}

func TestFormatLastSession_Integration(t *testing.T) {
	dur := 300.0
	info := &lastRunInfo{
		RunID:    "run-integration",
		Status:   "FINISHED",
		Duration: &dur,
		Metrics:  map[string]float64{"f1": 0.88, "recall": 0.92},
		Params:   map[string]string{"model": "bert", "batch_size": "32"},
	}
	got := formatLastSession(info)

	if !strings.HasPrefix(got, "<last-session>") {
		t.Errorf("expected output to start with <last-session>, got: %s", got)
	}
	if !strings.HasSuffix(got, "</last-session>") {
		t.Errorf("expected output to end with </last-session>, got: %s", got)
	}
	for _, want := range []string{"run=run-integration", "status=FINISHED", "duration=5m", "f1=0.88", "model=bert"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %s", want, got)
		}
	}
}
