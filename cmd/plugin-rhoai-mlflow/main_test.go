package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-mlflow" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-mlflow")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 10 {
		t.Fatalf("got %d tools, want 10", len(p.Tools))
	}
	wantNames := []string{
		"mlflow_experiments",
		"mlflow_runs",
		"mlflow_compare",
		"mlflow_artifacts",
		"mlflow_model_registry",
		"mlflow_model_version",
		"mlflow_promote",
		"mlflow_log_metric",
		"mlflow_setup",
		"experiment_last_session",
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

func TestToolDefinitions_Configured(t *testing.T) {
	p := newPlugin(options{MlflowURL: "http://localhost:5000"})
	if len(p.Tools) != 9 {
		t.Fatalf("got %d tools, want 9", len(p.Tools))
	}
	wantNames := []string{
		"mlflow_experiments",
		"mlflow_runs",
		"mlflow_compare",
		"mlflow_artifacts",
		"mlflow_model_registry",
		"mlflow_model_version",
		"mlflow_promote",
		"mlflow_log_metric",
		"experiment_last_session",
	}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Description == "" {
			t.Errorf("tool[%d] description is empty", i)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{MlflowURL: "http://localhost:5000"})
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
		name           string
		raw            map[string]any
		mlflowURL      string
		apiPrefix      string
		experimentName string
	}{
		{
			name:           "extracts all fields",
			raw:            map[string]any{"mlflowUrl": "http://mlflow:5000", "apiPrefix": "/api/3.0", "experimentName": "my-exp"},
			mlflowURL:      "http://mlflow:5000",
			apiPrefix:      "/api/3.0",
			experimentName: "my-exp",
		},
		{
			name:      "empty map defaults apiPrefix",
			raw:       map[string]any{},
			mlflowURL: "",
			apiPrefix: "/api/2.0/mlflow",
		},
		{
			name:      "wrong types ignored",
			raw:       map[string]any{"mlflowUrl": 42, "apiPrefix": true},
			mlflowURL: "",
			apiPrefix: "/api/2.0/mlflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.MlflowURL != tt.mlflowURL {
				t.Errorf("MlflowURL = %q, want %q", opts.MlflowURL, tt.mlflowURL)
			}
			if opts.APIPrefix != tt.apiPrefix {
				t.Errorf("APIPrefix = %q, want %q", opts.APIPrefix, tt.apiPrefix)
			}
			if opts.ExperimentName != tt.experimentName {
				t.Errorf("ExperimentName = %q, want %q", opts.ExperimentName, tt.experimentName)
			}
		})
	}
}

func TestFormatExperiments(t *testing.T) {
	tests := []struct {
		name     string
		exps     []experiment
		contains []string
	}{
		{
			name:     "empty list",
			exps:     nil,
			contains: []string{"No experiments found"},
		},
		{
			name: "single experiment",
			exps: []experiment{
				{ExperimentID: "1", Name: "train-model", LifecycleStage: "active"},
			},
			contains: []string{"Experiments: 1", "1", "train-model", "active"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExperiments(tt.exps)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatRuns(t *testing.T) {
	tests := []struct {
		name     string
		runs     []mlflowRun
		contains []string
	}{
		{
			name:     "empty list",
			runs:     nil,
			contains: []string{"No runs found"},
		},
		{
			name: "single run",
			runs: []mlflowRun{
				{Info: runInfo{RunID: "r-1", ExperimentID: "1", Status: "FINISHED"}},
			},
			contains: []string{"Runs: 1", "r-1", "FINISHED", "1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRuns(tt.runs)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatComparison(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := formatComparison(nil)
		if !strings.Contains(got, "No runs to compare") {
			t.Errorf("expected empty message, got %q", got)
		}
	})

	t.Run("two runs", func(t *testing.T) {
		comparisons := []map[string]any{
			{"runId": "run-aaa", "metrics": map[string]float64{"loss": 0.5}, "params": map[string]string{"lr": "0.01"}},
			{"runId": "run-bbb", "metrics": map[string]float64{"loss": 0.3}, "params": map[string]string{"lr": "0.001"}},
		}
		got := formatComparison(comparisons)
		if !strings.Contains(got, "Comparison of 2 runs") {
			t.Errorf("missing header in:\n%s", got)
		}
		if !strings.Contains(got, "run-aaaa") || !strings.Contains(got, "run-bbbb") {
			// Short IDs are truncated to 8 chars.
			if !strings.Contains(got, "run-aaa") {
				t.Errorf("missing run-aaa in:\n%s", got)
			}
		}
	})
}

func TestFormatArtifacts(t *testing.T) {
	tests := []struct {
		name     string
		arts     []artifact
		contains []string
	}{
		{
			name:     "empty",
			arts:     nil,
			contains: []string{"No artifacts found"},
		},
		{
			name: "file and dir",
			arts: []artifact{
				{Path: "model.pkl", IsDir: false, FileSize: intPtr(1024)},
				{Path: "checkpoints", IsDir: true},
			},
			contains: []string{"Artifacts: 2", "file", "model.pkl", "1024 bytes", "dir", "checkpoints"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatArtifacts(tt.arts)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func intPtr(v int64) *int64 { return &v }

func TestFormatModels(t *testing.T) {
	tests := []struct {
		name     string
		models   []registeredModel
		contains []string
	}{
		{
			name:     "empty",
			models:   nil,
			contains: []string{"No registered models found"},
		},
		{
			name: "with versions",
			models: []registeredModel{
				{
					Name: "sentiment-model",
					LatestVersions: []modelVersion{
						{Name: "sentiment-model", Version: "1", CurrentStage: "Production", Status: "READY", RunID: "r-1"},
					},
				},
			},
			contains: []string{"Registered Models: 1", "sentiment-model", "v1", "Production", "READY", "r-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatModels(tt.models)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatModelVersion(t *testing.T) {
	v := &modelVersion{
		Name:         "my-model",
		Version:      "3",
		CurrentStage: "Staging",
		Status:       "READY",
		RunID:        "run-xyz",
		Source:       "s3://bucket/model",
	}
	got := formatModelVersion(v)
	for _, want := range []string{"my-model", "v3", "Staging", "READY", "run-xyz", "s3://bucket/model"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatLastSession(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got := formatLastSession(nil)
		if !strings.Contains(got, "No previous session data") {
			t.Errorf("expected nil message, got %q", got)
		}
	})

	t.Run("with data", func(t *testing.T) {
		dur := 120.0
		info := &lastRunInfo{
			RunID:    "r-abc",
			Status:   "FINISHED",
			Metrics:  map[string]float64{"accuracy": 0.95},
			Params:   map[string]string{"epochs": "10"},
			Duration: &dur,
		}
		got := formatLastSession(info)
		for _, want := range []string{"r-abc", "FINISHED", "accuracy", "epochs", "10"} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q in:\n%s", want, got)
			}
		}
	})
}

// --- httptest mock-based tests ---

func newMockMlflowReadClient(handler http.Handler) (*mlflowReadClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &mlflowReadClient{
		api:       redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
		apiPrefix: "/api/2.0/mlflow",
	}, srv
}

func TestListExperiments_Mock(t *testing.T) {
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/2.0/mlflow/experiments/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"experiments":[
			{"experiment_id":"1","name":"train","lifecycle_stage":"active"},
			{"experiment_id":"2","name":"eval","lifecycle_stage":"deleted"}
		]}`))
	}))
	defer srv.Close()

	exps, err := client.listExperiments(context.Background())
	if err != nil {
		t.Fatalf("listExperiments() error: %v", err)
	}
	if len(exps) != 2 {
		t.Fatalf("got %d experiments, want 2", len(exps))
	}
	if exps[0].ExperimentID != "1" {
		t.Errorf("experiment[0].ExperimentID = %q, want %q", exps[0].ExperimentID, "1")
	}
	if exps[0].Name != "train" {
		t.Errorf("experiment[0].Name = %q, want %q", exps[0].Name, "train")
	}
	if exps[1].LifecycleStage != "deleted" {
		t.Errorf("experiment[1].LifecycleStage = %q, want %q", exps[1].LifecycleStage, "deleted")
	}
}

func TestListRuns_Mock(t *testing.T) {
	var gotPath string
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"runs":[
			{"info":{"run_id":"r-1","experiment_id":"1","status":"FINISHED","start_time":1000},"data":{"metrics":[{"key":"loss","value":0.5}],"params":[{"key":"lr","value":"0.01"}]}}
		]}`))
	}))
	defer srv.Close()

	runs, err := client.listRuns(context.Background(), "1", "")
	if err != nil {
		t.Fatalf("listRuns() error: %v", err)
	}
	if gotPath != "/api/2.0/mlflow/runs/search" {
		t.Errorf("path = %q, want %q", gotPath, "/api/2.0/mlflow/runs/search")
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Info.RunID != "r-1" {
		t.Errorf("run.RunID = %q, want %q", runs[0].Info.RunID, "r-1")
	}
	if runs[0].Info.Status != "FINISHED" {
		t.Errorf("run.Status = %q, want %q", runs[0].Info.Status, "FINISHED")
	}
	if len(runs[0].Data.Metrics) != 1 || runs[0].Data.Metrics[0].Key != "loss" {
		t.Errorf("unexpected metrics: %+v", runs[0].Data.Metrics)
	}
}

func TestListArtifacts_Mock(t *testing.T) {
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("run_id") != "r-1" {
			t.Errorf("expected run_id=r-1, got %q", r.URL.Query().Get("run_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"files":[
			{"path":"model.pkl","is_dir":false,"file_size":2048},
			{"path":"checkpoints","is_dir":true}
		]}`))
	}))
	defer srv.Close()

	arts, err := client.listArtifacts(context.Background(), "r-1", "")
	if err != nil {
		t.Fatalf("listArtifacts() error: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(arts))
	}
	if arts[0].Path != "model.pkl" {
		t.Errorf("artifact[0].Path = %q, want %q", arts[0].Path, "model.pkl")
	}
	if arts[0].IsDir {
		t.Error("artifact[0] should not be a directory")
	}
	if arts[1].Path != "checkpoints" || !arts[1].IsDir {
		t.Errorf("artifact[1] = %+v, want dir 'checkpoints'", arts[1])
	}
}

func TestListRegisteredModels_Mock(t *testing.T) {
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/2.0/mlflow/registered-models/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"registered_models":[
			{"name":"my-model","latest_versions":[{"name":"my-model","version":"1","current_stage":"Production","status":"READY","run_id":"r-1","creation_timestamp":1000}]}
		]}`))
	}))
	defer srv.Close()

	models, err := client.listRegisteredModels(context.Background())
	if err != nil {
		t.Fatalf("listRegisteredModels() error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if models[0].Name != "my-model" {
		t.Errorf("model.Name = %q, want %q", models[0].Name, "my-model")
	}
	if len(models[0].LatestVersions) != 1 {
		t.Fatalf("got %d versions, want 1", len(models[0].LatestVersions))
	}
	if models[0].LatestVersions[0].CurrentStage != "Production" {
		t.Errorf("version.CurrentStage = %q, want %q", models[0].LatestVersions[0].CurrentStage, "Production")
	}
}

func TestGetModelVersion_Mock(t *testing.T) {
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "my-model" {
			t.Errorf("expected name=my-model, got %q", r.URL.Query().Get("name"))
		}
		if r.URL.Query().Get("version") != "2" {
			t.Errorf("expected version=2, got %q", r.URL.Query().Get("version"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model_version":{"name":"my-model","version":"2","current_stage":"Staging","status":"READY","source":"s3://bucket","run_id":"r-2","creation_timestamp":2000}}`))
	}))
	defer srv.Close()

	v, err := client.getModelVersion(context.Background(), "my-model", "2")
	if err != nil {
		t.Fatalf("getModelVersion() error: %v", err)
	}
	if v.Name != "my-model" {
		t.Errorf("Name = %q, want %q", v.Name, "my-model")
	}
	if v.Version != "2" {
		t.Errorf("Version = %q, want %q", v.Version, "2")
	}
	if v.CurrentStage != "Staging" {
		t.Errorf("CurrentStage = %q, want %q", v.CurrentStage, "Staging")
	}
}

func TestMlflowReadClient_ServerError(t *testing.T) {
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	_, err := client.listExperiments(context.Background())
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestMlflowReadClient_InvalidJSON(t *testing.T) {
	client, srv := newMockMlflowReadClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	_, err := client.listExperiments(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
