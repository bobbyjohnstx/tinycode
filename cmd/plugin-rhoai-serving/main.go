package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

const defaultSandboxAPIBaseURL = "https://api.sandbox.devshift.net/api/v1"

const defaultSandboxAPIPrefix = ""

type options struct {
	// From model-serving.
	Namespace           string
	RouteHost           string
	ConsoleOfflineToken string
	SandboxURL          string
	APIPrefix           string
	// From eval-trustyai.
	EvalAPIURL  string
	TrustyAIURL string
	Token       string
	Username    string
	Password    string
}

func parseOptions(raw map[string]any) options {
	var opts options
	// Model-serving options.
	if v, ok := raw["namespace"].(string); ok {
		opts.Namespace = v
	}
	if v, ok := raw["routeHost"].(string); ok {
		opts.RouteHost = v
	}
	if v, ok := raw["consoleOfflineToken"].(string); ok {
		opts.ConsoleOfflineToken = v
	}
	if v, ok := raw["sandboxUrl"].(string); ok {
		opts.SandboxURL = v
	}
	if v, ok := raw["apiPrefix"].(string); ok {
		opts.APIPrefix = v
	}
	if opts.SandboxURL == "" {
		opts.SandboxURL = defaultSandboxAPIBaseURL
	}
	// Eval-trustyai options.
	if v, ok := raw["evalApiUrl"].(string); ok {
		opts.EvalAPIURL = v
	}
	if v, ok := raw["trustyaiUrl"].(string); ok {
		opts.TrustyAIURL = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	if v, ok := raw["username"].(string); ok {
		opts.Username = v
	}
	if v, ok := raw["password"].(string); ok {
		opts.Password = v
	}
	return opts
}

// --- model discovery types ---

type condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type inferenceServiceItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Predictor struct {
			Model struct {
				ModelFormat struct {
					Name string `json:"name"`
				} `json:"modelFormat"`
				Runtime string `json:"runtime"`
			} `json:"model"`
		} `json:"predictor"`
	} `json:"spec"`
	Status struct {
		URL        string      `json:"url"`
		Conditions []condition `json:"conditions"`
	} `json:"status"`
}

type podItem struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
	Spec struct {
		Containers []struct {
			Name      string `json:"name"`
			Resources struct {
				Limits map[string]string `json:"limits"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"spec"`
}

type servingRuntimeItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		SupportedModelFormats []struct {
			Name string `json:"name"`
		} `json:"supportedModelFormats"`
		Containers []struct {
			Image string `json:"image"`
		} `json:"containers"`
	} `json:"spec"`
}

// --- sandbox types ---

type sandboxSignupResponse struct {
	Status struct {
		Ready              bool   `json:"ready"`
		Reason             string `json:"reason"`
		VerificationDigits string `json:"verificationDigits"`
	} `json:"status"`
	APIEndpoint      string `json:"apiEndpoint"`
	ClusterName      string `json:"clusterName"`
	Company          string `json:"company"`
	CompliantUsername string `json:"compliantUsername"`
	ConsoleURL       string `json:"consoleURL"`
	FamilyName       string `json:"familyName"`
	GivenName        string `json:"givenName"`
	StartDate        string `json:"startDate"`
	Username         string `json:"username"`
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

// --- model formatters ---

func isReady(conditions []condition) bool {
	for _, c := range conditions {
		if c.Type == "Ready" && c.Status == "True" {
			return true
		}
	}
	return false
}

func formatModelList(items []inferenceServiceItem) string {
	if len(items) == 0 {
		return "No inference services found."
	}
	lines := []string{fmt.Sprintf("Inference Services: %d", len(items)), ""}
	for _, item := range items {
		status := "NotReady"
		if isReady(item.Status.Conditions) {
			status = "Ready"
		}
		format := item.Spec.Predictor.Model.ModelFormat.Name
		if format == "" {
			format = "unknown"
		}
		runtime := item.Spec.Predictor.Model.Runtime
		if runtime == "" {
			runtime = "default"
		}
		url := item.Status.URL
		if url == "" {
			url = "n/a"
		}
		lines = append(lines, fmt.Sprintf("  %s/%s [%s] format=%s runtime=%s url=%s",
			item.Metadata.Namespace, item.Metadata.Name, status, format, runtime, url))
	}
	return strings.Join(lines, "\n")
}

func formatModelStatus(item inferenceServiceItem, pods []podItem) string {
	status := "NotReady"
	if isReady(item.Status.Conditions) {
		status = "Ready"
	}
	lines := []string{
		fmt.Sprintf("Model: %s/%s", item.Metadata.Namespace, item.Metadata.Name),
		fmt.Sprintf("Status: %s", status),
	}
	if item.Status.URL != "" {
		lines = append(lines, fmt.Sprintf("URL: %s", item.Status.URL))
	}

	if len(item.Status.Conditions) > 0 {
		lines = append(lines, "", "Conditions:")
		for _, c := range item.Status.Conditions {
			lines = append(lines, fmt.Sprintf("  %s: %s", c.Type, c.Status))
		}
	}

	if len(pods) > 0 {
		lines = append(lines, "", "Pods:")
		for _, p := range pods {
			gpuInfo := ""
			for _, c := range p.Spec.Containers {
				if gpu, ok := c.Resources.Limits["nvidia.com/gpu"]; ok {
					gpuInfo = fmt.Sprintf(" gpu=%s", gpu)
				}
			}
			lines = append(lines, fmt.Sprintf("  %s [%s]%s", p.Metadata.Name, p.Status.Phase, gpuInfo))
		}
	}

	return strings.Join(lines, "\n")
}

func formatRuntimeList(items []servingRuntimeItem) string {
	if len(items) == 0 {
		return "No serving runtimes found."
	}
	lines := []string{fmt.Sprintf("Serving Runtimes: %d", len(items)), ""}
	for _, item := range items {
		var formats []string
		for _, f := range item.Spec.SupportedModelFormats {
			formats = append(formats, f.Name)
		}
		image := "unknown"
		if len(item.Spec.Containers) > 0 && item.Spec.Containers[0].Image != "" {
			image = item.Spec.Containers[0].Image
		}
		lines = append(lines, fmt.Sprintf("  %s/%s formats=[%s] image=%s",
			item.Metadata.Namespace, item.Metadata.Name, strings.Join(formats, ", "), image))
	}
	return strings.Join(lines, "\n")
}

func formatSandboxStatus(resp sandboxSignupResponse) string {
	if !resp.Status.Ready {
		state := "pending"
		if resp.Status.Reason == "NotSignedUp" || resp.Status.Reason == "" {
			state = "not-registered"
		}
		return fmt.Sprintf("Sandbox Status: %s\nReason: %s", state, resp.Status.Reason)
	}
	lines := []string{
		"Sandbox Status: ready",
		fmt.Sprintf("Cluster: %s", resp.ClusterName),
		fmt.Sprintf("Namespace: %s", resp.CompliantUsername+"-dev"),
		fmt.Sprintf("Console: %s", resp.ConsoleURL),
		fmt.Sprintf("API: %s", resp.APIEndpoint),
		fmt.Sprintf("Provisioned: %s", resp.StartDate),
	}
	return strings.Join(lines, "\n")
}

// --- eval formatters ---

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

func formatAlert(a trustyAlert) string {
	return fmt.Sprintf("[%s] %s: %s — %s at %.4f (threshold: %.4f)",
		strings.ToUpper(a.Severity), a.Type, a.Model, a.Metric, a.CurrentValue, a.Threshold)
}

// --- model tool builders ---

func buildModelTools(oc *redhat.OcClient, defaultNS string) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_list_models",
			Description: "List deployed inference services (models) on OpenShift AI.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace to list models from"},
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
				data, err := oc.Get(ctx, "inferenceservices", getOpts)
				if err != nil {
					return fmt.Sprintf("Failed to list models: %v", err), nil
				}
				var list struct {
					Items []inferenceServiceItem `json:"items"`
				}
				if err := json.Unmarshal(data, &list); err != nil {
					return fmt.Sprintf("Failed to parse models: %v", err), nil
				}
				return formatModelList(list.Items), nil
			},
		},
		{
			Name:        "rhoai_model_status",
			Description: "Get detailed status of a specific deployed model including pods and GPU allocation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "Model (InferenceService) name"},
					"namespace": map[string]any{"type": "string", "description": "Namespace of the model"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = defaultNS
				}
				resource := "inferenceservices/" + input.Name
				var getOpts *redhat.OcGetOptions
				if ns != "" {
					getOpts = &redhat.OcGetOptions{Namespace: ns}
				}
				data, err := oc.Get(ctx, resource, getOpts)
				if err != nil {
					return fmt.Sprintf("Failed to get model: %v", err), nil
				}
				var item inferenceServiceItem
				if err := json.Unmarshal(data, &item); err != nil {
					return fmt.Sprintf("Failed to parse model: %v", err), nil
				}

				// Get pods for this model.
				podOpts := &redhat.OcGetOptions{
					Selector: "serving.kserve.io/inferenceservice=" + input.Name,
				}
				if ns != "" {
					podOpts.Namespace = ns
				}
				var pods []podItem
				podData, err := oc.Get(ctx, "pods", podOpts)
				if err == nil {
					var podList struct {
						Items []podItem `json:"items"`
					}
					if json.Unmarshal(podData, &podList) == nil {
						pods = podList.Items
					}
				}

				return formatModelStatus(item, pods), nil
			},
		},
		{
			Name:        "rhoai_list_runtimes",
			Description: "List available serving runtimes on OpenShift AI.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace to list runtimes from"},
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
				data, err := oc.Get(ctx, "servingruntimes", getOpts)
				if err != nil {
					return fmt.Sprintf("Failed to list runtimes: %v", err), nil
				}
				var list struct {
					Items []servingRuntimeItem `json:"items"`
				}
				if err := json.Unmarshal(data, &list); err != nil {
					return fmt.Sprintf("Failed to parse runtimes: %v", err), nil
				}
				return formatRuntimeList(list.Items), nil
			},
		},
	}
}

// --- sandbox tool builders ---

func buildSandboxTools(sandboxClient *redhat.APIClient, apiPrefix string) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_sandbox_status",
			Description: "Check the status of a Developer Sandbox environment.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				resp, err := sandboxClient.Get(ctx, apiPrefix+"/signup", nil)
				if err != nil {
					return fmt.Sprintf("Failed to get sandbox status: %v", err), nil
				}
				var status sandboxSignupResponse
				if err := json.Unmarshal(resp.Data, &status); err != nil {
					return fmt.Sprintf("Failed to parse sandbox status: %v", err), nil
				}
				return formatSandboxStatus(status), nil
			},
		},
		{
			Name:        "rhoai_sandbox_provision",
			Description: "Provision a new Developer Sandbox environment.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				resp, err := sandboxClient.Post(ctx, apiPrefix+"/signup", nil)
				if err != nil {
					return fmt.Sprintf("Failed to provision sandbox: %v", err), nil
				}
				var status sandboxSignupResponse
				if err := json.Unmarshal(resp.Data, &status); err != nil {
					return fmt.Sprintf("Failed to parse response: %v", err), nil
				}
				return "Sandbox provisioning initiated.\n" + formatSandboxStatus(status), nil
			},
		},
	}
}

func unconfiguredSandboxTools() []plugin.ToolDef {
	msg := "Sandbox tools not configured. Set consoleOfflineToken in plugin options."
	return []plugin.ToolDef{
		{
			Name:        "rhoai_sandbox_status",
			Description: "Check Developer Sandbox status.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhoai_sandbox_provision",
			Description: "Provision a Developer Sandbox.",
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

// --- eval tool builders ---

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

// --- trustyai tool builders ---

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

// --- workbench tool builders ---

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

// --- combined health tool ---

func buildHealthTool(oc *redhat.OcClient, sandboxClient, evalClient, trustyClient *redhat.APIClient, apiPrefix string) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "rhoai_serving_health",
		Description: "Check connectivity to all dependent services (cluster API, sandbox, eval API, TrustyAI, workbenches).",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			var lines []string

			// Cluster API (InferenceService access).
			if _, err := oc.Get(ctx, "inferenceservices", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] Cluster API (InferenceService access): %v", err))
			} else {
				lines = append(lines, "[OK] Cluster API (InferenceService access)")
			}

			// Sandbox API.
			if sandboxClient != nil {
				if _, err := sandboxClient.Get(ctx, apiPrefix+"/signup", nil); err != nil {
					lines = append(lines, fmt.Sprintf("[DOWN] Sandbox API: %v", err))
				} else {
					lines = append(lines, "[OK] Sandbox API")
				}
			} else {
				lines = append(lines, "[SKIP] Sandbox API (not configured)")
			}

			// Eval API.
			if evalClient != nil {
				if _, err := evalClient.Get(ctx, "/api/v1/evaluations", nil); err != nil {
					lines = append(lines, fmt.Sprintf("[DOWN] Eval API: %v", err))
				} else {
					lines = append(lines, "[OK] Eval API")
				}
			} else {
				lines = append(lines, "[SKIP] Eval API (not configured)")
			}

			// TrustyAI API.
			if trustyClient != nil {
				if _, err := trustyClient.Get(ctx, "/api/v1/alerts", nil); err != nil {
					lines = append(lines, fmt.Sprintf("[DOWN] TrustyAI API: %v", err))
				} else {
					lines = append(lines, "[OK] TrustyAI API")
				}
			} else {
				lines = append(lines, "[SKIP] TrustyAI API (not configured)")
			}

			// Workbenches (notebooks CRD).
			if _, err := oc.Get(ctx, "notebooks.kubeflow.org", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] Workbenches (notebooks CRD): %v", err))
			} else {
				lines = append(lines, "[OK] Workbenches (notebooks CRD)")
			}

			return "Service Health:\n" + strings.Join(lines, "\n"), nil
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	oc := redhat.NewOcClient()

	// Model tools (always available via oc).
	tools := buildModelTools(oc, opts.Namespace)

	// Sandbox tools.
	var sandboxClient *redhat.APIClient
	if opts.ConsoleOfflineToken != "" {
		authClient := redhat.NewConsoleAuthClient(redhat.ConsoleAuthConfig{
			OfflineToken: opts.ConsoleOfflineToken,
		})
		sandboxClient = redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: opts.SandboxURL,
			TokenFn: authClient.GetAccessToken,
		})
		tools = append(tools, buildSandboxTools(sandboxClient, opts.APIPrefix)...)
	} else {
		tools = append(tools, unconfiguredSandboxTools()...)
	}

	// Eval tools.
	var evalClient *redhat.APIClient
	if opts.EvalAPIURL != "" {
		cfg := redhat.APIClientConfig{BaseURL: opts.EvalAPIURL}
		if opts.Username != "" && opts.Password != "" {
			cfg.BasicAuth = &redhat.BasicAuthConfig{Username: opts.Username, Password: opts.Password}
		} else if opts.Token != "" {
			cfg.TokenFn = func(_ context.Context) (string, error) { return opts.Token, nil }
		}
		evalClient = redhat.NewAPIClient(cfg)
		tools = append(tools, buildEvalTools(evalClient)...)
	} else {
		tools = append(tools, unconfiguredEvalTools()...)
	}

	// TrustyAI tools.
	var trustyClient *redhat.APIClient
	if opts.TrustyAIURL != "" {
		cfg := redhat.APIClientConfig{BaseURL: opts.TrustyAIURL}
		if opts.Username != "" && opts.Password != "" {
			cfg.BasicAuth = &redhat.BasicAuthConfig{Username: opts.Username, Password: opts.Password}
		} else if opts.Token != "" {
			cfg.TokenFn = func(_ context.Context) (string, error) { return opts.Token, nil }
		}
		trustyClient = redhat.NewAPIClient(cfg)
		tools = append(tools, buildTrustyTools(trustyClient)...)
	} else {
		tools = append(tools, unconfiguredTrustyTools()...)
	}

	// Workbench tools.
	tools = append(tools, buildWorkbenchTools(oc, opts.Namespace)...)

	// Combined health tool.
	tools = append(tools, buildHealthTool(oc, sandboxClient, evalClient, trustyClient, opts.APIPrefix))

	return plugin.Plugin{
		ID:    "rhoai-serving",
		Tools: tools,
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
