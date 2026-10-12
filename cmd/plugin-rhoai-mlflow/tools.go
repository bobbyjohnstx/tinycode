package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

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
