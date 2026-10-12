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

// --- sandbox types ---

type sandboxSignupResponse struct {
	Status struct {
		Ready              bool   `json:"ready"`
		Reason             string `json:"reason"`
		VerificationDigits string `json:"verificationDigits"`
	} `json:"status"`
	APIEndpoint       string `json:"apiEndpoint"`
	ClusterName       string `json:"clusterName"`
	Company           string `json:"company"`
	CompliantUsername string `json:"compliantUsername"`
	ConsoleURL        string `json:"consoleURL"`
	FamilyName        string `json:"familyName"`
	GivenName         string `json:"givenName"`
	StartDate         string `json:"startDate"`
	Username          string `json:"username"`
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
