package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// options holds the parsed initialize params for cluster-ops.
type options struct {
	ClusterID           string
	APIURL              string
	ConsoleOfflineToken string
	InsecureSkipTLS     bool
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["clusterId"].(string); ok {
		opts.ClusterID = v
	}
	if v, ok := raw["apiUrl"].(string); ok {
		opts.APIURL = v
	}
	if v, ok := raw["consoleOfflineToken"].(string); ok {
		opts.ConsoleOfflineToken = v
	}
	if v, ok := raw["insecureSkipTLSVerify"].(bool); ok {
		opts.InsecureSkipTLS = v
	}
	return opts
}

// newPlugin builds the cluster-ops plugin with the given options.
func newPlugin(opts options) plugin.Plugin {
	return plugin.Plugin{
		ID:    "cluster-ops",
		Tools: buildTools(opts),
		Hooks: buildHooks(opts),
	}
}

func buildTools(opts options) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "oc-login",
			Description: "Authenticate to an OpenShift cluster using oc login",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"server": map[string]any{
						"type":        "string",
						"description": "API server URL (defaults to configured apiUrl)",
					},
					"token": map[string]any{
						"type":        "string",
						"description": "Authentication token (defaults to configured consoleOfflineToken)",
					},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
				var input struct {
					Server string `json:"server"`
					Token  string `json:"token"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				server := input.Server
				if server == "" {
					server = opts.APIURL
				}
				if server == "" {
					return "", fmt.Errorf("no server URL provided and apiUrl not configured")
				}

				token := input.Token
				if token == "" {
					token = opts.ConsoleOfflineToken
				}
				if token == "" {
					return "", fmt.Errorf("no token provided and consoleOfflineToken not configured")
				}

				loginArgs := []string{"login", "--server=" + server, "--token=" + token}
				if opts.InsecureSkipTLS {
					loginArgs = append(loginArgs, "--insecure-skip-tls-verify")
				}
				cmd := exec.CommandContext(ctx, "oc", loginArgs...)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return string(out), fmt.Errorf("oc login failed: %w", err)
				}
				return strings.TrimSpace(string(out)), nil
			},
		},
		{
			Name:        "oc-status",
			Description: "Show the status of the current OpenShift cluster",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				cmd := exec.CommandContext(ctx, "oc", "status")
				out, err := cmd.CombinedOutput()
				if err != nil {
					return string(out), fmt.Errorf("oc status failed: %w", err)
				}
				return strings.TrimSpace(string(out)), nil
			},
		},
		{
			Name:        "cluster-info",
			Description: "Show details about the current cluster including version and nodes",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				cmd := exec.CommandContext(ctx, "kubectl", "cluster-info")
				out, err := cmd.CombinedOutput()
				if err != nil {
					return string(out), fmt.Errorf("kubectl cluster-info failed: %w", err)
				}
				return strings.TrimSpace(string(out)), nil
			},
		},
	}
}

func buildHooks(opts options) plugin.HookHandlers {
	return plugin.HookHandlers{
		ShellEnv: func(_ context.Context, input plugin.ShellEnvInput) (*plugin.ShellEnvOutput, error) {
			env := map[string]string{}
			if opts.ClusterID != "" {
				env["CLUSTER_ID"] = opts.ClusterID
			}
			kubeconfigPath := fmt.Sprintf("%s/.kube/config", input.Directory)
			env["KUBECONFIG"] = kubeconfigPath
			return &plugin.ShellEnvOutput{Env: env}, nil
		},
		SessionStart: func(_ context.Context, event plugin.SessionStartEvent) error {
			slog.Info("cluster-ops: session started",
				"sessionId", event.SessionID,
				"clusterId", opts.ClusterID,
			)
			return nil
		},
	}
}

func main() {
	// In a real plugin, options come from the initialize handshake.
	// The SDK handles this internally — the plugin registers tools/hooks
	// with default options, and the server passes options during initialize.
	// For now, we build with empty options; the server populates them.
	opts := options{}
	plugin.Run(newPlugin(opts))
}
