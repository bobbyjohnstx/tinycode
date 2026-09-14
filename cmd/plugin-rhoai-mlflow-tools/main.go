package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	MlflowURL string
	APIPrefix string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["mlflowUrl"].(string); ok {
		opts.MlflowURL = v
	}
	if v, ok := raw["apiPrefix"].(string); ok {
		opts.APIPrefix = v
	}
	if opts.APIPrefix == "" {
		opts.APIPrefix = "/api/2.0/mlflow"
	}
	return opts
}

// --- mlflow read types ---

type experiment struct {
	ExperimentID   string `json:"experiment_id"`
	Name           string `json:"name"`
	ArtifactLoc    string `json:"artifact_location"`
	LifecycleStage string `json:"lifecycle_stage"`
	LastUpdateTime *int64 `json:"last_update_time,omitempty"`
}

type runInfo struct {
	RunID          string `json:"run_id"`
	ExperimentID   string `json:"experiment_id"`
	Status         string `json:"status"`
	StartTime      int64  `json:"start_time"`
	EndTime        *int64 `json:"end_time,omitempty"`
	ArtifactURI    string `json:"artifact_uri"`
	LifecycleStage string `json:"lifecycle_stage"`
}

type runMetric struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

type runParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type mlflowRun struct {
	Info runInfo `json:"info"`
	Data struct {
		Metrics []runMetric `json:"metrics"`
		Params  []runParam  `json:"params"`
	} `json:"data"`
}

type artifact struct {
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	FileSize *int64 `json:"file_size,omitempty"`
}

type modelVersion struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	CurrentStage      string `json:"current_stage"`
	Status            string `json:"status"`
	Source            string `json:"source"`
	RunID             string `json:"run_id"`
	CreationTimestamp int64  `json:"creation_timestamp"`
}

type registeredModel struct {
	Name           string         `json:"name"`
	LatestVersions []modelVersion `json:"latest_versions"`
}

// --- mlflow read client ---

type mlflowReadClient struct {
	api       *redhat.APIClient
	apiPrefix string
}

func newMlflowReadClient(api *redhat.APIClient, apiPrefix string) *mlflowReadClient {
	return &mlflowReadClient{api: api, apiPrefix: apiPrefix}
}

func (c *mlflowReadClient) listExperiments(ctx context.Context) ([]experiment, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/experiments/search", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Experiments []experiment `json:"experiments"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Experiments, nil
}

func (c *mlflowReadClient) listRuns(ctx context.Context, experimentID, filter string) ([]mlflowRun, error) {
	body := map[string]any{
		"experiment_ids": []string{experimentID},
	}
	if filter != "" {
		body["filter"] = filter
	}
	resp, err := c.api.Post(ctx, c.apiPrefix+"/runs/search", body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Runs []mlflowRun `json:"runs"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Runs, nil
}

func (c *mlflowReadClient) compareRuns(ctx context.Context, runIDs []string) ([]map[string]any, error) {
	var comparisons []map[string]any
	for _, id := range runIDs {
		resp, err := c.api.Get(ctx, c.apiPrefix+"/runs/get", map[string]string{"run_id": id})
		if err != nil {
			return nil, fmt.Errorf("fetching run %s: %w", id, err)
		}
		var result struct {
			Run mlflowRun `json:"run"`
		}
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("parsing run %s: %w", id, err)
		}
		metrics := make(map[string]float64)
		for _, m := range result.Run.Data.Metrics {
			metrics[m.Key] = m.Value
		}
		params := make(map[string]string)
		for _, p := range result.Run.Data.Params {
			params[p.Key] = p.Value
		}
		comparisons = append(comparisons, map[string]any{
			"runId":   id,
			"params":  params,
			"metrics": metrics,
		})
	}
	return comparisons, nil
}

func (c *mlflowReadClient) listArtifacts(ctx context.Context, runID, path string) ([]artifact, error) {
	query := map[string]string{"run_id": runID}
	if path != "" {
		query["path"] = path
	}
	resp, err := c.api.Get(ctx, c.apiPrefix+"/artifacts/list", query)
	if err != nil {
		return nil, err
	}
	var result struct {
		Files []artifact `json:"files"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Files, nil
}

func (c *mlflowReadClient) listRegisteredModels(ctx context.Context) ([]registeredModel, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/registered-models/search", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		RegisteredModels []registeredModel `json:"registered_models"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.RegisteredModels, nil
}

func (c *mlflowReadClient) getModelVersion(ctx context.Context, name, version string) (*modelVersion, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/model-versions/get", map[string]string{
		"name":    name,
		"version": version,
	})
	if err != nil {
		return nil, err
	}
	var result struct {
		ModelVersion modelVersion `json:"model_version"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result.ModelVersion, nil
}

func (c *mlflowReadClient) transitionModelStage(ctx context.Context, name, version, stage string) error {
	_, err := c.api.Post(ctx, c.apiPrefix+"/model-versions/transition-stage", map[string]any{
		"name":                      name,
		"version":                   version,
		"stage":                     stage,
		"archive_existing_versions": true,
	})
	return err
}

// --- formatters ---

func formatExperiments(exps []experiment) string {
	if len(exps) == 0 {
		return "No experiments found."
	}
	lines := []string{fmt.Sprintf("Experiments: %d", len(exps)), ""}
	for _, e := range exps {
		lines = append(lines, fmt.Sprintf("  [%s] %s (stage: %s)", e.ExperimentID, e.Name, e.LifecycleStage))
	}
	return strings.Join(lines, "\n")
}

func formatRuns(runs []mlflowRun) string {
	if len(runs) == 0 {
		return "No runs found."
	}
	lines := []string{fmt.Sprintf("Runs: %d", len(runs)), ""}
	for _, r := range runs {
		lines = append(lines, fmt.Sprintf("  [%s] status=%s experiment=%s",
			r.Info.RunID, r.Info.Status, r.Info.ExperimentID))
	}
	return strings.Join(lines, "\n")
}

func formatComparison(comparisons []map[string]any) string {
	if len(comparisons) == 0 {
		return "No runs to compare."
	}

	// Collect all metric and param keys.
	metricKeys := map[string]bool{}
	paramKeys := map[string]bool{}
	for _, c := range comparisons {
		if m, ok := c["metrics"].(map[string]float64); ok {
			for k := range m {
				metricKeys[k] = true
			}
		}
		if p, ok := c["params"].(map[string]string); ok {
			for k := range p {
				paramKeys[k] = true
			}
		}
	}

	lines := []string{fmt.Sprintf("Comparison of %d runs:", len(comparisons)), ""}

	// Header.
	header := "| Key |"
	sep := "| --- |"
	for _, c := range comparisons {
		runID := c["runId"].(string)
		short := runID
		if len(short) > 8 {
			short = short[:8]
		}
		header += fmt.Sprintf(" %s |", short)
		sep += " --- |"
	}
	lines = append(lines, header, sep)

	// Params.
	for key := range paramKeys {
		row := fmt.Sprintf("| %s |", key)
		for _, c := range comparisons {
			val := "-"
			if p, ok := c["params"].(map[string]string); ok {
				if v, ok := p[key]; ok {
					val = v
				}
			}
			row += fmt.Sprintf(" %s |", val)
		}
		lines = append(lines, row)
	}

	// Metrics.
	for key := range metricKeys {
		row := fmt.Sprintf("| %s |", key)
		for _, c := range comparisons {
			val := "-"
			if m, ok := c["metrics"].(map[string]float64); ok {
				if v, ok := m[key]; ok {
					val = fmt.Sprintf("%.4f", v)
				}
			}
			row += fmt.Sprintf(" %s |", val)
		}
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

func formatArtifacts(artifacts []artifact) string {
	if len(artifacts) == 0 {
		return "No artifacts found."
	}
	lines := []string{fmt.Sprintf("Artifacts: %d", len(artifacts)), ""}
	for _, a := range artifacts {
		kind := "file"
		if a.IsDir {
			kind = "dir"
		}
		size := ""
		if a.FileSize != nil {
			size = fmt.Sprintf(" (%d bytes)", *a.FileSize)
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s%s", kind, a.Path, size))
	}
	return strings.Join(lines, "\n")
}

func formatModels(models []registeredModel) string {
	if len(models) == 0 {
		return "No registered models found."
	}
	lines := []string{fmt.Sprintf("Registered Models: %d", len(models)), ""}
	for _, m := range models {
		lines = append(lines, fmt.Sprintf("  %s (%d versions)", m.Name, len(m.LatestVersions)))
		for _, v := range m.LatestVersions {
			lines = append(lines, fmt.Sprintf("    v%s: stage=%s status=%s run=%s", v.Version, v.CurrentStage, v.Status, v.RunID))
		}
	}
	return strings.Join(lines, "\n")
}

func formatModelVersion(v *modelVersion) string {
	return strings.Join([]string{
		fmt.Sprintf("Model: %s v%s", v.Name, v.Version),
		fmt.Sprintf("Stage: %s | Status: %s", v.CurrentStage, v.Status),
		fmt.Sprintf("Run: %s", v.RunID),
		fmt.Sprintf("Source: %s", v.Source),
	}, "\n")
}

// --- tool builders ---

func buildMlflowTools(readClient *mlflowReadClient, writeClient *redhat.MlflowClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "mlflow_experiments",
			Description: "List all MLflow experiments.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				exps, err := readClient.listExperiments(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list experiments: %v", err), nil
				}
				return formatExperiments(exps), nil
			},
		},
		{
			Name:        "mlflow_runs",
			Description: "List runs for an MLflow experiment with optional filter.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"experimentId": map[string]any{"type": "string", "description": "Experiment ID to list runs for"},
					"filter":       map[string]any{"type": "string", "description": "Optional filter expression (e.g. status = 'FINISHED')"},
				},
				"required": []string{"experimentId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					ExperimentID string `json:"experimentId"`
					Filter       string `json:"filter"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				runs, err := readClient.listRuns(ctx, input.ExperimentID, input.Filter)
				if err != nil {
					return fmt.Sprintf("Failed to list runs: %v", err), nil
				}
				return formatRuns(runs), nil
			},
		},
		{
			Name:        "mlflow_compare",
			Description: "Compare multiple MLflow runs side by side with metrics and parameters.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runIds": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"minItems":    2,
						"maxItems":    5,
						"description": "Run IDs to compare (2-5)",
					},
				},
				"required": []string{"runIds"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					RunIDs []string `json:"runIds"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				if len(input.RunIDs) < 2 || len(input.RunIDs) > 5 {
					return "Provide 2 to 5 run IDs for comparison.", nil
				}
				comparisons, err := readClient.compareRuns(ctx, input.RunIDs)
				if err != nil {
					return fmt.Sprintf("Failed to compare runs: %v", err), nil
				}
				return formatComparison(comparisons), nil
			},
		},
		{
			Name:        "mlflow_artifacts",
			Description: "List artifacts for an MLflow run.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runId": map[string]any{"type": "string", "description": "Run ID to list artifacts for"},
					"path":  map[string]any{"type": "string", "description": "Optional sub-path within artifacts"},
				},
				"required": []string{"runId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					RunID string `json:"runId"`
					Path  string `json:"path"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				artifacts, err := readClient.listArtifacts(ctx, input.RunID, input.Path)
				if err != nil {
					return fmt.Sprintf("Failed to list artifacts: %v", err), nil
				}
				return formatArtifacts(artifacts), nil
			},
		},
		{
			Name:        "mlflow_model_registry",
			Description: "List all registered models in the MLflow model registry.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				models, err := readClient.listRegisteredModels(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list models: %v", err), nil
				}
				return formatModels(models), nil
			},
		},
		{
			Name:        "mlflow_model_version",
			Description: "Get details of a specific model version.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Model name"},
					"version": map[string]any{"type": "string", "description": "Model version"},
				},
				"required": []string{"name", "version"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				v, err := readClient.getModelVersion(ctx, input.Name, input.Version)
				if err != nil {
					return fmt.Sprintf("Failed to get model version: %v", err), nil
				}
				return formatModelVersion(v), nil
			},
		},
		{
			Name:        "mlflow_promote",
			Description: "Transition a model version to a new stage (Staging, Production, or Archived).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Model name"},
					"version": map[string]any{"type": "string", "description": "Model version"},
					"stage":   map[string]any{"type": "string", "enum": []string{"Staging", "Production", "Archived"}, "description": "Target stage"},
				},
				"required": []string{"name", "version", "stage"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name    string `json:"name"`
					Version string `json:"version"`
					Stage   string `json:"stage"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				if err := readClient.transitionModelStage(ctx, input.Name, input.Version, input.Stage); err != nil {
					return fmt.Sprintf("Failed to promote model: %v", err), nil
				}
				return fmt.Sprintf("Model %s v%s transitioned to %s.", input.Name, input.Version, input.Stage), nil
			},
		},
		{
			Name:        "mlflow_log_metric",
			Description: "Log a metric to an MLflow run.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runId": map[string]any{"type": "string", "description": "Run ID to log metric to"},
					"key":   map[string]any{"type": "string", "description": "Metric key"},
					"value": map[string]any{"type": "number", "description": "Metric value"},
					"step":  map[string]any{"type": "integer", "description": "Optional step number"},
				},
				"required": []string{"runId", "key", "value"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					RunID string  `json:"runId"`
					Key   string  `json:"key"`
					Value float64 `json:"value"`
					Step  *int    `json:"step"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				if err := writeClient.LogMetric(ctx, input.RunID, input.Key, input.Value, input.Step); err != nil {
					return fmt.Sprintf("Failed to log metric: %v", err), nil
				}
				return fmt.Sprintf("Metric %s=%.4f logged to run %s.", input.Key, input.Value, input.RunID), nil
			},
		},
	}
}

func unconfiguredMlflowTools() []plugin.ToolDef {
	msg := "MLflow tools not configured. Set mlflowUrl in plugin options."
	names := []struct {
		name string
		desc string
	}{
		{"mlflow_experiments", "List all MLflow experiments."},
		{"mlflow_runs", "List runs for an MLflow experiment."},
		{"mlflow_compare", "Compare multiple MLflow runs."},
		{"mlflow_artifacts", "List artifacts for an MLflow run."},
		{"mlflow_model_registry", "List registered models."},
		{"mlflow_model_version", "Get model version details."},
		{"mlflow_promote", "Transition a model version stage."},
		{"mlflow_log_metric", "Log a metric to a run."},
	}
	var tools []plugin.ToolDef
	for _, n := range names {
		name := n.name
		desc := n.desc
		tools = append(tools, plugin.ToolDef{
			Name:        name,
			Description: desc,
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		})
	}
	return tools
}

func newPlugin(opts options) plugin.Plugin {
	if opts.MlflowURL == "" {
		return plugin.Plugin{
			ID:    "rhoai-mlflow-tools",
			Tools: unconfiguredMlflowTools(),
		}
	}

	tokenFn := func(_ context.Context) (string, error) { return "", nil }
	api := redhat.NewAPIClient(redhat.APIClientConfig{
		BaseURL: opts.MlflowURL,
		TokenFn: tokenFn,
	})
	readClient := newMlflowReadClient(api, opts.APIPrefix)
	writeClient := redhat.NewMlflowClient(api)

	return plugin.Plugin{
		ID:    "rhoai-mlflow-tools",
		Tools: buildMlflowTools(readClient, writeClient),
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
