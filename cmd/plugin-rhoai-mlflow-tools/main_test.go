package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-mlflow-tools" {
		t.Errorf("expected plugin ID 'rhoai-mlflow-tools', got %q", p.ID)
	}
}

func TestToolCountUnconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 8 {
		t.Fatalf("expected 8 unconfigured tools, got %d", len(p.Tools))
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
