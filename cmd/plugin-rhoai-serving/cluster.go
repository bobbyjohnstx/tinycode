package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

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
