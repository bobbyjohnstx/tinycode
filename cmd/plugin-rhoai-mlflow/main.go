package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type options struct {
	MlflowURL      string
	APIPrefix      string
	ExperimentName string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["mlflowUrl"].(string); ok {
		opts.MlflowURL = v
	}
	if v, ok := raw["apiPrefix"].(string); ok {
		opts.APIPrefix = v
	}
	if v, ok := raw["experimentName"].(string); ok {
		opts.ExperimentName = v
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

// --- experiment tracker types ---

type lastRunInfo struct {
	RunID    string             `json:"runId"`
	Status   string             `json:"status"`
	Metrics  map[string]float64 `json:"metrics"`
	Params   map[string]string  `json:"params"`
	Duration *float64           `json:"duration,omitempty"`
}

type trackerState struct {
	mu             sync.RWMutex
	runID          string
	toolCallCount  int
	startTime      time.Time
	lastRun        *lastRunInfo
	experimentName string
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

func formatLastSession(info *lastRunInfo) string {
	if info == nil {
		return "No previous session data available."
	}

	parts := []string{
		fmt.Sprintf("run=%s", info.RunID),
		fmt.Sprintf("status=%s", info.Status),
	}

	if info.Duration != nil {
		minutes := int(math.Round(*info.Duration / 60))
		parts = append(parts, fmt.Sprintf("duration=%dm", minutes))
	}

	if len(info.Metrics) > 0 {
		var metricParts []string
		for k, v := range info.Metrics {
			metricParts = append(metricParts, fmt.Sprintf("%s=%g", k, v))
		}
		parts = append(parts, fmt.Sprintf("metrics: %s", strings.Join(metricParts, ", ")))
	}

	if len(info.Params) > 0 {
		var paramParts []string
		for k, v := range info.Params {
			paramParts = append(paramParts, fmt.Sprintf("%s=%s", k, v))
		}
		parts = append(parts, fmt.Sprintf("params: %s", strings.Join(paramParts, ", ")))
	}

	return fmt.Sprintf("<last-session>%s</last-session>", strings.Join(parts, " "))
}

// --- experiment tracker ---

func fetchLastRun(ctx context.Context, api *redhat.APIClient, experimentName, apiPrefix string) (*lastRunInfo, error) {
	expResp, err := api.Get(ctx, apiPrefix+"/experiments/get-by-name", map[string]string{
		"experiment_name": experimentName,
	})
	if err != nil {
		return nil, err
	}
	var expResult struct {
		Experiment struct {
			ExperimentID string `json:"experiment_id"`
		} `json:"experiment"`
	}
	if err := json.Unmarshal(expResp.Data, &expResult); err != nil {
		return nil, err
	}

	runsResp, err := api.Post(ctx, apiPrefix+"/runs/search", map[string]any{
		"experiment_ids": []string{expResult.Experiment.ExperimentID},
		"filter":         "status = 'FINISHED'",
		"order_by":       []string{"start_time DESC"},
		"max_results":    1,
	})
	if err != nil {
		return nil, err
	}

	var runsResult struct {
		Runs []struct {
			Info struct {
				RunID     string `json:"run_id"`
				Status    string `json:"status"`
				StartTime int64  `json:"start_time"`
				EndTime   *int64 `json:"end_time,omitempty"`
			} `json:"info"`
			Data struct {
				Metrics []struct {
					Key   string  `json:"key"`
					Value float64 `json:"value"`
				} `json:"metrics"`
				Params []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"params"`
			} `json:"data"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(runsResp.Data, &runsResult); err != nil {
		return nil, err
	}
	if len(runsResult.Runs) == 0 {
		return nil, nil
	}

	run := runsResult.Runs[0]
	metrics := make(map[string]float64)
	for _, m := range run.Data.Metrics {
		metrics[m.Key] = m.Value
	}
	params := make(map[string]string)
	for _, p := range run.Data.Params {
		params[p.Key] = p.Value
	}

	info := &lastRunInfo{
		RunID:   run.Info.RunID,
		Status:  run.Info.Status,
		Metrics: metrics,
		Params:  params,
	}
	if run.Info.EndTime != nil {
		dur := float64(*run.Info.EndTime-run.Info.StartTime) / 1000.0
		info.Duration = &dur
	}
	return info, nil
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

func buildExperimentLastSessionTool(st *trackerState) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "experiment_last_session",
		Description: "Get information about the last tracked experiment session including metrics and parameters.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			st.mu.RLock()
			defer st.mu.RUnlock()
			return formatLastSession(st.lastRun), nil
		},
	}
}

const mlflowSetupGuide = `MLflow is not included in RHOAI by default. Deploy it on OpenShift with:

1. Create a project:
   oc new-project mlflow

2. Apply the deployment (PVC + Deployment + Service + Route):
   oc apply -n mlflow -f - <<'EOF'
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: mlflow-data
   spec:
     accessModes: [ReadWriteOnce]
     resources:
       requests:
         storage: 5Gi
   ---
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: mlflow
     labels:
       app: mlflow
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: mlflow
     template:
       metadata:
         labels:
           app: mlflow
       spec:
         containers:
         - name: mlflow
           image: ghcr.io/mlflow/mlflow:v2.16.2
           command: ["mlflow", "server"]
           args:
           - "--host=0.0.0.0"
           - "--port=5000"
           - "--backend-store-uri=sqlite:///data/mlflow.db"
           - "--default-artifact-root=/data/artifacts"
           ports:
           - containerPort: 5000
           volumeMounts:
           - name: data
             mountPath: /data
           resources:
             requests:
               cpu: 200m
               memory: 512Mi
             limits:
               cpu: "1"
               memory: 1Gi
           readinessProbe:
             httpGet:
               path: /health
               port: 5000
             initialDelaySeconds: 10
             periodSeconds: 10
           livenessProbe:
             httpGet:
               path: /health
               port: 5000
             initialDelaySeconds: 15
             periodSeconds: 30
         volumes:
         - name: data
           persistentVolumeClaim:
             claimName: mlflow-data
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: mlflow
   spec:
     selector:
       app: mlflow
     ports:
     - port: 5000
       targetPort: 5000
   ---
   apiVersion: route.openshift.io/v1
   kind: Route
   metadata:
     name: mlflow
   spec:
     to:
       kind: Service
       name: mlflow
     port:
       targetPort: 5000
     tls:
       termination: edge
       insecureEdgeTerminationPolicy: Redirect
   EOF

3. Get the route URL:
   oc get route mlflow -n mlflow -o jsonpath='https://{.spec.host}'

4. Set mlflowUrl in plugin options to that URL.

For production use, replace SQLite with PostgreSQL and add S3-compatible storage for artifacts.`

func unconfiguredMlflowTools() []plugin.ToolDef {
	msg := "MLflow tools not configured. Set mlflowUrl in plugin options. Use mlflow_setup for installation instructions."
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
	tools = append(tools, plugin.ToolDef{
		Name:        "mlflow_setup",
		Description: "Get instructions for deploying MLflow on OpenShift. MLflow is not included in RHOAI by default and must be deployed separately.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			return mlflowSetupGuide, nil
		},
	})
	return tools
}

func unconfiguredExperimentLastSessionTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "experiment_last_session",
		Description: "Get information about the last tracked experiment session.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			return "Experiment tracker not configured. Set mlflowUrl in plugin options.", nil
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	if opts.MlflowURL == "" {
		tools := unconfiguredMlflowTools()
		tools = append(tools, unconfiguredExperimentLastSessionTool())
		return plugin.Plugin{
			ID:    "rhoai-mlflow",
			Tools: tools,
		}
	}

	tokenFn := func(_ context.Context) (string, error) { return "", nil }
	api := redhat.NewAPIClient(redhat.APIClientConfig{
		BaseURL: opts.MlflowURL,
		TokenFn: tokenFn,
	})
	readClient := newMlflowReadClient(api, opts.APIPrefix)
	writeClient := redhat.NewMlflowClient(api)

	st := &trackerState{}
	mlflow := writeClient

	tools := buildMlflowTools(readClient, writeClient)
	tools = append(tools, buildExperimentLastSessionTool(st))

	return plugin.Plugin{
		ID:    "rhoai-mlflow",
		Tools: tools,
		Hooks: plugin.HookHandlers{
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) (*plugin.SessionStartOutput, error) {
				expName := opts.ExperimentName
				if expName == "" {
					expName = filepath.Base(event.Directory)
					if expName == "" {
						expName = "default"
					}
				}

				st.mu.Lock()
				st.experimentName = expName
				st.mu.Unlock()

				// Fetch last run (best-effort).
				if info, err := fetchLastRun(ctx, api, expName, opts.APIPrefix); err == nil {
					st.mu.Lock()
					st.lastRun = info
					st.mu.Unlock()
				}

				// Create experiment if needed, then create run.
				experimentID, err := mlflow.GetExperimentByName(ctx, expName)
				if err != nil || experimentID == "" {
					experimentID, err = mlflow.CreateExperiment(ctx, expName)
					if err != nil {
						slog.Warn("rhoai-mlflow: failed to create experiment", "error", err)
						return nil, nil
					}
				}

				runID, err := mlflow.CreateRun(ctx, experimentID, []redhat.RunTag{
					{Key: "sessionID", Value: event.SessionID},
				})
				if err != nil {
					slog.Warn("rhoai-mlflow: failed to create run", "error", err)
					return nil, nil
				}

				st.mu.Lock()
				st.runID = runID
				st.toolCallCount = 0
				st.startTime = time.Now()
				st.mu.Unlock()

				return nil, nil
			},
			ToolExecAfter: func(ctx context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
				st.mu.Lock()
				runID := st.runID
				st.toolCallCount++
				step := st.toolCallCount
				st.mu.Unlock()

				if runID == "" {
					return nil, nil
				}

				_ = mlflow.LogMetric(ctx, runID, input.ToolName, 1, &step)
				return nil, nil
			},
			SessionEnd: func(ctx context.Context, _ plugin.SessionEndEvent) error {
				st.mu.Lock()
				runID := st.runID
				toolCallCount := st.toolCallCount
				duration := time.Since(st.startTime).Seconds()
				st.runID = ""
				st.mu.Unlock()

				if runID == "" {
					return nil
				}

				_ = mlflow.LogMetric(ctx, runID, "tool_call_count", float64(toolCallCount), nil)
				_ = mlflow.LogMetric(ctx, runID, "session_duration_seconds", duration, nil)
				_ = mlflow.EndRun(ctx, runID, "FINISHED")
				return nil
			},
			Dispose: func(ctx context.Context) error {
				st.mu.Lock()
				runID := st.runID
				st.runID = ""
				st.mu.Unlock()

				if runID == "" {
					return nil
				}

				_ = mlflow.EndRun(ctx, runID, "KILLED")
				return nil
			},
		},
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
