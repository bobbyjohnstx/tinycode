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
	if p.ID != "rhoai-mlflow-tools" {
		t.Errorf("expected plugin ID 'rhoai-mlflow-tools', got %q", p.ID)
	}
}

func TestToolCountUnconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 9 {
		t.Fatalf("expected 9 unconfigured tools, got %d", len(p.Tools))
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{})
	expectedNames := []string{
		"mlflow_experiments",
		"mlflow_runs",
		"mlflow_compare",
		"mlflow_artifacts",
		"mlflow_model_registry",
		"mlflow_model_version",
		"mlflow_promote",
		"mlflow_log_metric",
		"mlflow_setup",
	}

	if len(p.Tools) != len(expectedNames) {
		t.Fatalf("expected %d tools, got %d", len(expectedNames), len(p.Tools))
	}

	for i, name := range expectedNames {
		tool := p.Tools[i]
		if tool.Name != name {
			t.Errorf("tool[%d]: expected name %q, got %q", i, name, tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("tool %q: expected non-empty description", name)
		}
		if tool.Execute == nil {
			t.Errorf("tool %q: expected non-nil Execute", name)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{})
	for _, tool := range p.Tools {
		typ, ok := tool.Parameters["type"]
		if !ok || typ != "object" {
			t.Errorf("tool %q: expected type 'object', got %v", tool.Name, typ)
		}
		if _, ok := tool.Parameters["properties"]; !ok {
			t.Errorf("tool %q: missing 'properties' key", tool.Name)
		}
	}
}

func TestParseOptions(t *testing.T) {
	t.Run("with mlflowUrl", func(t *testing.T) {
		opts := parseOptions(map[string]any{"mlflowUrl": "http://localhost:5000"})
		if opts.MlflowURL != "http://localhost:5000" {
			t.Errorf("expected MlflowURL 'http://localhost:5000', got %q", opts.MlflowURL)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		opts := parseOptions(map[string]any{})
		if opts.MlflowURL != "" {
			t.Errorf("expected empty MlflowURL, got %q", opts.MlflowURL)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		opts := parseOptions(map[string]any{"mlflowUrl": 123})
		if opts.MlflowURL != "" {
			t.Errorf("expected empty MlflowURL for non-string, got %q", opts.MlflowURL)
		}
	})
}

func TestFormatExperiments(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := formatExperiments(nil)
		if result != "No experiments found." {
			t.Errorf("expected 'No experiments found.', got %q", result)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		exps := []experiment{
			{ExperimentID: "1", Name: "exp-a", LifecycleStage: "active"},
			{ExperimentID: "2", Name: "exp-b", LifecycleStage: "deleted"},
		}
		result := formatExperiments(exps)
		if !strings.Contains(result, "Experiments: 2") {
			t.Errorf("expected count header, got %q", result)
		}
		if !strings.Contains(result, "[1] exp-a (stage: active)") {
			t.Errorf("expected experiment 1, got %q", result)
		}
		if !strings.Contains(result, "[2] exp-b (stage: deleted)") {
			t.Errorf("expected experiment 2, got %q", result)
		}
	})
}

func TestFormatRuns(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := formatRuns(nil)
		if result != "No runs found." {
			t.Errorf("expected 'No runs found.', got %q", result)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		runs := []mlflowRun{
			{Info: runInfo{RunID: "run-1", Status: "FINISHED", ExperimentID: "1"}},
			{Info: runInfo{RunID: "run-2", Status: "RUNNING", ExperimentID: "2"}},
		}
		result := formatRuns(runs)
		if !strings.Contains(result, "Runs: 2") {
			t.Errorf("expected count, got %q", result)
		}
		if !strings.Contains(result, "[run-1] status=FINISHED experiment=1") {
			t.Errorf("expected run-1 info, got %q", result)
		}
		if !strings.Contains(result, "[run-2] status=RUNNING experiment=2") {
			t.Errorf("expected run-2 info, got %q", result)
		}
	})
}

func TestFormatComparison(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := formatComparison(nil)
		if result != "No runs to compare." {
			t.Errorf("expected 'No runs to compare.', got %q", result)
		}
	})

	t.Run("two runs", func(t *testing.T) {
		comparisons := []map[string]any{
			{
				"runId":   "abcdefghij",
				"params":  map[string]string{"lr": "0.01"},
				"metrics": map[string]float64{"accuracy": 0.95},
			},
			{
				"runId":   "1234567890",
				"params":  map[string]string{"lr": "0.001"},
				"metrics": map[string]float64{"accuracy": 0.97},
			},
		}
		result := formatComparison(comparisons)
		if !strings.Contains(result, "Comparison of 2 runs:") {
			t.Errorf("expected comparison header, got %q", result)
		}
		if !strings.Contains(result, "abcdefgh") {
			t.Errorf("expected truncated run ID, got %q", result)
		}
		if !strings.Contains(result, "lr") {
			t.Errorf("expected param key, got %q", result)
		}
		if !strings.Contains(result, "accuracy") {
			t.Errorf("expected metric key, got %q", result)
		}
	})
}

func TestFormatArtifacts(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := formatArtifacts(nil)
		if result != "No artifacts found." {
			t.Errorf("expected 'No artifacts found.', got %q", result)
		}
	})

	t.Run("files and dirs", func(t *testing.T) {
		size := int64(1024)
		artifacts := []artifact{
			{Path: "model.pkl", IsDir: false, FileSize: &size},
			{Path: "checkpoints", IsDir: true},
			{Path: "config.yaml", IsDir: false},
		}
		result := formatArtifacts(artifacts)
		if !strings.Contains(result, "Artifacts: 3") {
			t.Errorf("expected count, got %q", result)
		}
		if !strings.Contains(result, "[file] model.pkl (1024 bytes)") {
			t.Errorf("expected file with size, got %q", result)
		}
		if !strings.Contains(result, "[dir] checkpoints") {
			t.Errorf("expected dir, got %q", result)
		}
	})
}

func TestFormatModels(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := formatModels(nil)
		if result != "No registered models found." {
			t.Errorf("expected 'No registered models found.', got %q", result)
		}
	})

	t.Run("with versions", func(t *testing.T) {
		models := []registeredModel{
			{
				Name: "my-model",
				LatestVersions: []modelVersion{
					{Version: "1", CurrentStage: "Production", Status: "READY", RunID: "run-abc"},
					{Version: "2", CurrentStage: "Staging", Status: "READY", RunID: "run-def"},
				},
			},
		}
		result := formatModels(models)
		if !strings.Contains(result, "Registered Models: 1") {
			t.Errorf("expected count, got %q", result)
		}
		if !strings.Contains(result, "my-model (2 versions)") {
			t.Errorf("expected model name with version count, got %q", result)
		}
		if !strings.Contains(result, "v1: stage=Production status=READY run=run-abc") {
			t.Errorf("expected version 1 info, got %q", result)
		}
		if !strings.Contains(result, "v2: stage=Staging status=READY run=run-def") {
			t.Errorf("expected version 2 info, got %q", result)
		}
	})
}

func TestFormatModelVersion(t *testing.T) {
	v := &modelVersion{
		Name:         "bert-finetuned",
		Version:      "3",
		CurrentStage: "Production",
		Status:       "READY",
		RunID:        "run-xyz",
		Source:       "s3://bucket/model",
	}
	result := formatModelVersion(v)
	if !strings.Contains(result, "Model: bert-finetuned v3") {
		t.Errorf("expected model name+version, got %q", result)
	}
	if !strings.Contains(result, "Stage: Production | Status: READY") {
		t.Errorf("expected stage+status, got %q", result)
	}
	if !strings.Contains(result, "Run: run-xyz") {
		t.Errorf("expected run ID, got %q", result)
	}
	if !strings.Contains(result, "Source: s3://bucket/model") {
		t.Errorf("expected source, got %q", result)
	}
}

// --- httptest mock-based tests ---

func newMockMlflowClient(handler http.Handler) (*mlflowReadClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	api := redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL})
	return newMlflowReadClient(api, "/api/2.0/mlflow"), srv
}

func TestMock_ListExperiments(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/experiments/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"experiments": []map[string]any{
				{"experiment_id": "1", "name": "fraud-detection", "lifecycle_stage": "active"},
				{"experiment_id": "2", "name": "sentiment-v2", "lifecycle_stage": "active"},
			},
		})
	}))
	defer srv.Close()

	exps, err := client.listExperiments(context.Background())
	if err != nil {
		t.Fatalf("listExperiments error: %v", err)
	}
	if len(exps) != 2 {
		t.Fatalf("expected 2 experiments, got %d", len(exps))
	}
	if exps[0].Name != "fraud-detection" {
		t.Errorf("expected name 'fraud-detection', got %q", exps[0].Name)
	}
	if exps[1].ExperimentID != "2" {
		t.Errorf("expected experiment_id '2', got %q", exps[1].ExperimentID)
	}
}

func TestMock_ListRuns(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/runs/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		ids, _ := body["experiment_ids"].([]any)
		if len(ids) == 0 || ids[0] != "exp-1" {
			t.Errorf("expected experiment_ids=[exp-1], got %v", ids)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"runs": []map[string]any{
				{
					"info": map[string]any{
						"run_id":        "run-aaa",
						"experiment_id": "exp-1",
						"status":        "FINISHED",
						"start_time":    1700000000000,
					},
					"data": map[string]any{
						"metrics": []map[string]any{
							{"key": "accuracy", "value": 0.95},
							{"key": "loss", "value": 0.12},
						},
						"params": []map[string]any{
							{"key": "lr", "value": "0.001"},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	runs, err := client.listRuns(context.Background(), "exp-1", "")
	if err != nil {
		t.Fatalf("listRuns error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Info.RunID != "run-aaa" {
		t.Errorf("expected run_id 'run-aaa', got %q", runs[0].Info.RunID)
	}
	if runs[0].Info.Status != "FINISHED" {
		t.Errorf("expected status FINISHED, got %q", runs[0].Info.Status)
	}
	if len(runs[0].Data.Metrics) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(runs[0].Data.Metrics))
	}
	if len(runs[0].Data.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(runs[0].Data.Params))
	}
}

func TestMock_ListRuns_WithFilter(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["filter"] != "status = 'FINISHED'" {
			t.Errorf("expected filter, got %v", body["filter"])
		}
		json.NewEncoder(w).Encode(map[string]any{"runs": []any{}})
	}))
	defer srv.Close()

	runs, err := client.listRuns(context.Background(), "exp-1", "status = 'FINISHED'")
	if err != nil {
		t.Fatalf("listRuns error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

func TestMock_CompareRuns(t *testing.T) {
	callCount := 0
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/runs/get" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		runID := r.URL.Query().Get("run_id")
		callCount++
		json.NewEncoder(w).Encode(map[string]any{
			"run": map[string]any{
				"info": map[string]any{
					"run_id": runID,
					"status": "FINISHED",
				},
				"data": map[string]any{
					"metrics": []map[string]any{
						{"key": "accuracy", "value": 0.9 + float64(callCount)*0.01},
					},
					"params": []map[string]any{
						{"key": "epochs", "value": "10"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	comparisons, err := client.compareRuns(context.Background(), []string{"run-1", "run-2"})
	if err != nil {
		t.Fatalf("compareRuns error: %v", err)
	}
	if len(comparisons) != 2 {
		t.Fatalf("expected 2 comparisons, got %d", len(comparisons))
	}
	if comparisons[0]["runId"] != "run-1" {
		t.Errorf("expected runId 'run-1', got %v", comparisons[0]["runId"])
	}
	metrics, ok := comparisons[0]["metrics"].(map[string]float64)
	if !ok {
		t.Fatal("expected metrics map")
	}
	if _, exists := metrics["accuracy"]; !exists {
		t.Error("expected accuracy metric")
	}
	params, ok := comparisons[0]["params"].(map[string]string)
	if !ok {
		t.Fatal("expected params map")
	}
	if params["epochs"] != "10" {
		t.Errorf("expected epochs=10, got %q", params["epochs"])
	}
}

func TestMock_ListArtifacts(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/artifacts/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.URL.Query().Get("run_id") != "run-abc" {
			t.Errorf("expected run_id=run-abc, got %q", r.URL.Query().Get("run_id"))
		}
		if r.URL.Query().Get("path") != "models" {
			t.Errorf("expected path=models, got %q", r.URL.Query().Get("path"))
		}
		size := int64(2048)
		json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"path": "models/model.pkl", "is_dir": false, "file_size": size},
				{"path": "models/config", "is_dir": true},
			},
		})
	}))
	defer srv.Close()

	artifacts, err := client.listArtifacts(context.Background(), "run-abc", "models")
	if err != nil {
		t.Fatalf("listArtifacts error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	if artifacts[0].Path != "models/model.pkl" {
		t.Errorf("expected path 'models/model.pkl', got %q", artifacts[0].Path)
	}
	if artifacts[0].IsDir {
		t.Error("expected file, not dir")
	}
	if artifacts[0].FileSize == nil || *artifacts[0].FileSize != 2048 {
		t.Errorf("expected file_size 2048, got %v", artifacts[0].FileSize)
	}
	if !artifacts[1].IsDir {
		t.Error("expected dir for second artifact")
	}
}

func TestMock_ListArtifacts_NoPath(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("path") != "" {
			t.Errorf("expected no path param, got %q", r.URL.Query().Get("path"))
		}
		json.NewEncoder(w).Encode(map[string]any{"files": []any{}})
	}))
	defer srv.Close()

	artifacts, err := client.listArtifacts(context.Background(), "run-abc", "")
	if err != nil {
		t.Fatalf("listArtifacts error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
}

func TestMock_ListRegisteredModels(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/registered-models/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"registered_models": []map[string]any{
				{
					"name": "bert-classifier",
					"latest_versions": []map[string]any{
						{"name": "bert-classifier", "version": "1", "current_stage": "Production", "status": "READY", "run_id": "run-111", "source": "s3://models/bert"},
						{"name": "bert-classifier", "version": "2", "current_stage": "Staging", "status": "READY", "run_id": "run-222", "source": "s3://models/bert-v2"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	models, err := client.listRegisteredModels(context.Background())
	if err != nil {
		t.Fatalf("listRegisteredModels error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].Name != "bert-classifier" {
		t.Errorf("expected name 'bert-classifier', got %q", models[0].Name)
	}
	if len(models[0].LatestVersions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(models[0].LatestVersions))
	}
	if models[0].LatestVersions[0].CurrentStage != "Production" {
		t.Errorf("expected stage Production, got %q", models[0].LatestVersions[0].CurrentStage)
	}
	if models[0].LatestVersions[1].RunID != "run-222" {
		t.Errorf("expected run_id 'run-222', got %q", models[0].LatestVersions[1].RunID)
	}
}

func TestMock_GetModelVersion(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/model-versions/get" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.URL.Query().Get("name") != "my-model" {
			t.Errorf("expected name=my-model, got %q", r.URL.Query().Get("name"))
		}
		if r.URL.Query().Get("version") != "3" {
			t.Errorf("expected version=3, got %q", r.URL.Query().Get("version"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model_version": map[string]any{
				"name":               "my-model",
				"version":            "3",
				"current_stage":      "Production",
				"status":             "READY",
				"source":             "s3://bucket/my-model/3",
				"run_id":             "run-xyz",
				"creation_timestamp": 1700000000000,
			},
		})
	}))
	defer srv.Close()

	v, err := client.getModelVersion(context.Background(), "my-model", "3")
	if err != nil {
		t.Fatalf("getModelVersion error: %v", err)
	}
	if v.Name != "my-model" {
		t.Errorf("expected name 'my-model', got %q", v.Name)
	}
	if v.Version != "3" {
		t.Errorf("expected version '3', got %q", v.Version)
	}
	if v.CurrentStage != "Production" {
		t.Errorf("expected stage Production, got %q", v.CurrentStage)
	}
	if v.Status != "READY" {
		t.Errorf("expected status READY, got %q", v.Status)
	}
	if v.RunID != "run-xyz" {
		t.Errorf("expected run_id 'run-xyz', got %q", v.RunID)
	}
	if v.Source != "s3://bucket/my-model/3" {
		t.Errorf("expected source 's3://bucket/my-model/3', got %q", v.Source)
	}
}

func TestMock_TransitionModelStage(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/model-versions/transition-stage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "my-model" {
			t.Errorf("expected name 'my-model', got %v", body["name"])
		}
		if body["version"] != "2" {
			t.Errorf("expected version '2', got %v", body["version"])
		}
		if body["stage"] != "Production" {
			t.Errorf("expected stage 'Production', got %v", body["stage"])
		}
		if body["archive_existing_versions"] != true {
			t.Errorf("expected archive_existing_versions=true, got %v", body["archive_existing_versions"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model_version": map[string]any{
				"name":          "my-model",
				"version":       "2",
				"current_stage": "Production",
			},
		})
	}))
	defer srv.Close()

	err := client.transitionModelStage(context.Background(), "my-model", "2", "Production")
	if err != nil {
		t.Fatalf("transitionModelStage error: %v", err)
	}
}

func TestMock_TransitionModelStage_Error(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error_code":"RESOURCE_DOES_NOT_EXIST","message":"Model not found"}`, 404)
	}))
	defer srv.Close()

	err := client.transitionModelStage(context.Background(), "nonexistent", "1", "Production")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestMock_ListExperiments_Empty(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"experiments": []any{}})
	}))
	defer srv.Close()

	exps, err := client.listExperiments(context.Background())
	if err != nil {
		t.Fatalf("listExperiments error: %v", err)
	}
	if len(exps) != 0 {
		t.Errorf("expected 0 experiments, got %d", len(exps))
	}
}

func TestMock_CompareRuns_Error(t *testing.T) {
	client, srv := newMockMlflowClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", 500)
	}))
	defer srv.Close()

	_, err := client.compareRuns(context.Background(), []string{"run-1"})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}
