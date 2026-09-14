package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

const defaultSandboxAPIBaseURL = "https://api.sandbox.devshift.net/api/v1"

type options struct {
	Namespace           string
	RouteHost           string
	ConsoleOfflineToken string
	SandboxURL          string
}

func parseOptions(raw map[string]any) options {
	var opts options
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
	if opts.SandboxURL == "" {
		opts.SandboxURL = defaultSandboxAPIBaseURL
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

// --- sandbox types ---

type sandboxSignupResponse struct {
	Status struct {
		Ready              bool   `json:"ready"`
		Reason             string `json:"reason"`
		VerificationDigits string `json:"verificationDigits"`
	} `json:"status"`
	APIEndpoint     string `json:"apiEndpoint"`
	ClusterName     string `json:"clusterName"`
	Company         string `json:"company"`
	CompliantUsername string `json:"compliantUsername"`
	ConsoleURL      string `json:"consoleURL"`
	FamilyName      string `json:"familyName"`
	GivenName       string `json:"givenName"`
	StartDate       string `json:"startDate"`
	Username        string `json:"username"`
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

// --- tool builders ---

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

func buildSandboxTools(sandboxClient *redhat.APIClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_sandbox_status",
			Description: "Check the status of a Developer Sandbox environment.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				resp, err := sandboxClient.Get(ctx, "/signup", nil)
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
				resp, err := sandboxClient.Post(ctx, "/signup", nil)
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

func newPlugin(opts options) plugin.Plugin {
	oc := redhat.NewOcClient()

	tools := buildModelTools(oc, opts.Namespace)

	if opts.ConsoleOfflineToken != "" {
		authClient := redhat.NewConsoleAuthClient(redhat.ConsoleAuthConfig{
			OfflineToken: opts.ConsoleOfflineToken,
		})
		sandboxClient := redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: opts.SandboxURL,
			TokenFn: authClient.GetAccessToken,
		})
		tools = append(tools, buildSandboxTools(sandboxClient)...)
	} else {
		tools = append(tools, unconfiguredSandboxTools()...)
	}

	return plugin.Plugin{
		ID:    "rhoai-model-serving",
		Tools: tools,
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
