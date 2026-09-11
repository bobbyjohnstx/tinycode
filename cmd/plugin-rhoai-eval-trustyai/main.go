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
	EvalAPIURL  string
	TrustyAIURL string
	Namespace   string
	Token       string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["evalApiUrl"].(string); ok {
		opts.EvalAPIURL = v
	}
	if v, ok := raw["trustyaiUrl"].(string); ok {
		opts.TrustyAIURL = v
	}
	if v, ok := raw["namespace"].(string); ok {
		opts.Namespace = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	return opts
}

// --- eval types ---

type evalResult struct {
	EvalID      string `json:"eval_id"`
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
	Results     []struct {
		Metric string  `json:"metric"`
		Score  float64 `json:"score"`
		Detail string  `json:"detail,omitempty"`
	} `json:"results,omitempty"`
}

func formatEvalResult(r evalResult) string {
	lines := []string{
		fmt.Sprintf("Eval: %s", r.EvalID),
		fmt.Sprintf("Model: %s | Provider: %s", r.Model, r.Provider),
		fmt.Sprintf("Status: %s | Created: %s", r.Status, r.CreatedAt),
	}
	if r.CompletedAt != "" {
		lines = append(lines, fmt.Sprintf("Completed: %s", r.CompletedAt))
	}
	if r.Error != "" {
		lines = append(lines, fmt.Sprintf("Error: %s", r.Error))
	}
	if len(r.Results) > 0 {
		lines = append(lines, "", "Results:")
		lines = append(lines, fmt.Sprintf("%-20s %-10s %s", "METRIC", "SCORE", "DETAIL"))
		lines = append(lines, strings.Repeat("-", 50))
		for _, res := range r.Results {
			detail := res.Detail
			if detail == "" {
				detail = "-"
			}
			lines = append(lines, fmt.Sprintf("%-20s %-10.4f %s", res.Metric, res.Score, detail))
		}
	}
	return strings.Join(lines, "\n")
}

// --- trustyai types ---

type trustyMetrics struct {
	Model                string             `json:"model"`
	DriftScore           float64            `json:"driftScore"`
	BiasMetrics          map[string]float64 `json:"biasMetrics"`
	FeatureDistributions map[string]struct {
		Mean   float64 `json:"mean"`
		Stddev float64 `json:"stddev"`
	} `json:"featureDistributions"`
}

type trustyAlert struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Model        string  `json:"model"`
	Metric       string  `json:"metric"`
	Threshold    float64 `json:"threshold"`
	CurrentValue float64 `json:"currentValue"`
	Severity     string  `json:"severity"`
	TriggeredAt  string  `json:"triggeredAt"`
}

func formatAlert(a trustyAlert) string {
	return fmt.Sprintf("[%s] %s: %s — %s at %.4f (threshold: %.4f)",
		strings.ToUpper(a.Severity), a.Type, a.Model, a.Metric, a.CurrentValue, a.Threshold)
}

// --- tool builders ---

func buildEvalTools(client *redhat.APIClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_eval_run",
			Description: "Start an evaluation run for a model on RHOAI. Returns an eval ID to track progress.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model":    map[string]any{"type": "string", "description": "Model name to evaluate"},
					"provider": map[string]any{"type": "string", "description": "Model provider (e.g. vllm, tgis)"},
					"config":   map[string]any{"type": "object", "description": "Optional evaluation configuration"},
				},
				"required": []string{"model", "provider"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Model    string         `json:"model"`
					Provider string         `json:"provider"`
					Config   map[string]any `json:"config"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				body := map[string]any{
					"model":    input.Model,
					"provider": input.Provider,
				}
				if input.Config != nil {
					body["config"] = input.Config
				}
				resp, err := client.Post(ctx, "/api/v1/evaluations", body)
				if err != nil {
					return fmt.Sprintf("Failed to start evaluation: %v", err), nil
				}
				var result struct {
					EvalID string `json:"eval_id"`
				}
				if err := json.Unmarshal(resp.Data, &result); err != nil {
					return fmt.Sprintf("Failed to parse response: %v", err), nil
				}
				return fmt.Sprintf("Evaluation started. ID: %s\nUse rhoai_eval_status to check progress.", result.EvalID), nil
			},
		},
		{
			Name:        "rhoai_eval_status",
			Description: "Check the status of an evaluation run by its ID.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"evalId": map[string]any{"type": "string", "description": "Evaluation ID to check"},
				},
				"required": []string{"evalId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					EvalID string `json:"evalId"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				resp, err := client.Get(ctx, "/api/v1/evaluations/"+input.EvalID, nil)
				if err != nil {
					return fmt.Sprintf("Failed to get eval status: %v", err), nil
				}
				var result evalResult
				if err := json.Unmarshal(resp.Data, &result); err != nil {
					return fmt.Sprintf("Failed to parse response: %v", err), nil
				}
				return formatEvalResult(result), nil
			},
		},
		{
			Name:        "rhoai_eval_compare",
			Description: "Compare multiple evaluation runs side by side.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"evalIds": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of evaluation IDs to compare",
					},
				},
				"required": []string{"evalIds"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					EvalIDs []string `json:"evalIds"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				if len(input.EvalIDs) < 2 {
					return "At least 2 evaluation IDs are required for comparison.", nil
				}
				var results []evalResult
				for _, id := range input.EvalIDs {
					resp, err := client.Get(ctx, "/api/v1/evaluations/"+id, nil)
					if err != nil {
						return fmt.Sprintf("Failed to fetch eval %s: %v", id, err), nil
					}
					var r evalResult
					if err := json.Unmarshal(resp.Data, &r); err != nil {
						return fmt.Sprintf("Failed to parse eval %s: %v", id, err), nil
					}
					results = append(results, r)
				}
				lines := []string{fmt.Sprintf("Comparison of %d evaluations:", len(results)), ""}
				for _, r := range results {
					lines = append(lines, formatEvalResult(r), "")
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func buildTrustyTools(client *redhat.APIClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_trusty_metrics",
			Description: "Get TrustyAI metrics for a deployed model including drift and bias scores.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model": map[string]any{"type": "string", "description": "Model name to get metrics for"},
				},
				"required": []string{"model"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Model string `json:"model"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				resp, err := client.Get(ctx, "/api/v1/models/"+input.Model+"/metrics", nil)
				if err != nil {
					return fmt.Sprintf("Failed to get metrics: %v", err), nil
				}
				var metrics trustyMetrics
				if err := json.Unmarshal(resp.Data, &metrics); err != nil {
					return fmt.Sprintf("Failed to parse metrics: %v", err), nil
				}
				lines := []string{
					fmt.Sprintf("Model: %s", metrics.Model),
					fmt.Sprintf("Drift Score: %.4f", metrics.DriftScore),
				}
				if len(metrics.BiasMetrics) > 0 {
					lines = append(lines, "", "Bias Metrics:")
					for k, v := range metrics.BiasMetrics {
						lines = append(lines, fmt.Sprintf("  %s: %.4f", k, v))
					}
				}
				if len(metrics.FeatureDistributions) > 0 {
					lines = append(lines, "", "Feature Distributions:")
					for k, v := range metrics.FeatureDistributions {
						lines = append(lines, fmt.Sprintf("  %s: mean=%.4f stddev=%.4f", k, v.Mean, v.Stddev))
					}
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "rhoai_trusty_alerts",
			Description: "List active TrustyAI alerts for drift and bias across models.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				resp, err := client.Get(ctx, "/api/v1/alerts", nil)
				if err != nil {
					return fmt.Sprintf("Failed to get alerts: %v", err), nil
				}
				var alerts []trustyAlert
				if err := json.Unmarshal(resp.Data, &alerts); err != nil {
					return fmt.Sprintf("Failed to parse alerts: %v", err), nil
				}
				if len(alerts) == 0 {
					return "No active TrustyAI alerts.", nil
				}
				lines := []string{fmt.Sprintf("Active Alerts: %d", len(alerts)), ""}
				for _, a := range alerts {
					lines = append(lines, formatAlert(a))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func buildWorkbenchTools(oc *redhat.OcClient, defaultNS string) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_workbench_list",
			Description: "List RHOAI workbenches (Jupyter notebooks) running on the cluster.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace to list workbenches from"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = defaultNS
				}
				var getOpts *redhat.OcGetOptions
				if ns != "" {
					getOpts = &redhat.OcGetOptions{Namespace: ns}
				}
				data, err := oc.Get(ctx, "notebooks.kubeflow.org", getOpts)
				if err != nil {
					return fmt.Sprintf("Failed to list workbenches: %v", err), nil
				}
				var list struct {
					Items []struct {
						Metadata struct {
							Name      string `json:"name"`
							Namespace string `json:"namespace"`
						} `json:"metadata"`
						Status struct {
							ReadyReplicas int `json:"readyReplicas"`
						} `json:"status"`
					} `json:"items"`
				}
				if err := json.Unmarshal(data, &list); err != nil {
					return fmt.Sprintf("Failed to parse workbenches: %v", err), nil
				}
				if len(list.Items) == 0 {
					return "No workbenches found.", nil
				}
				lines := []string{fmt.Sprintf("Workbenches: %d", len(list.Items)), ""}
				for _, item := range list.Items {
					status := "Stopped"
					if item.Status.ReadyReplicas > 0 {
						status = "Running"
					}
					lines = append(lines, fmt.Sprintf("  %s/%s [%s]", item.Metadata.Namespace, item.Metadata.Name, status))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func unconfiguredEvalTools() []plugin.ToolDef {
	msg := "Eval plugin not configured. Set evalApiUrl in plugin options."
	return []plugin.ToolDef{
		{
			Name:        "rhoai_eval_run",
			Description: "Start an evaluation run for a model on RHOAI.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model": map[string]any{"type": "string", "description": "Model name"},
				},
				"required": []string{"model"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhoai_eval_status",
			Description: "Check the status of an evaluation run.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"evalId": map[string]any{"type": "string", "description": "Evaluation ID"},
				},
				"required": []string{"evalId"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhoai_eval_compare",
			Description: "Compare multiple evaluation runs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"evalIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"evalIds"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func unconfiguredTrustyTools() []plugin.ToolDef {
	msg := "TrustyAI plugin not configured. Set trustyaiUrl in plugin options."
	return []plugin.ToolDef{
		{
			Name:        "rhoai_trusty_metrics",
			Description: "Get TrustyAI metrics for a deployed model.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model": map[string]any{"type": "string", "description": "Model name"},
				},
				"required": []string{"model"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhoai_trusty_alerts",
			Description: "List active TrustyAI alerts.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	oc := redhat.NewOcClient()

	var tools []plugin.ToolDef

	if opts.EvalAPIURL != "" {
		tokenFn := func(_ context.Context) (string, error) { return opts.Token, nil }
		evalClient := redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: opts.EvalAPIURL,
			TokenFn: tokenFn,
		})
		tools = append(tools, buildEvalTools(evalClient)...)
	} else {
		tools = append(tools, unconfiguredEvalTools()...)
	}

	if opts.TrustyAIURL != "" {
		tokenFn := func(_ context.Context) (string, error) { return opts.Token, nil }
		trustyClient := redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: opts.TrustyAIURL,
			TokenFn: tokenFn,
		})
		tools = append(tools, buildTrustyTools(trustyClient)...)
	} else {
		tools = append(tools, unconfiguredTrustyTools()...)
	}

	tools = append(tools, buildWorkbenchTools(oc, opts.Namespace)...)

	return plugin.Plugin{
		ID:    "rhoai-eval-trustyai",
		Tools: tools,
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
