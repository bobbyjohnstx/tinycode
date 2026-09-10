package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	Server              string
	InsecureSkipTLS     bool
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["server"].(string); ok {
		opts.Server = v
	}
	if v, ok := raw["insecureSkipTlsVerify"].(bool); ok {
		opts.InsecureSkipTLS = v
	}
	return opts
}

func newPlugin(opts options) plugin.Plugin {
	return plugin.Plugin{
		ID: "ocp-oauth",
		Tools: []plugin.ToolDef{
			{
				Name:        "oc-login",
				Description: "Authenticate to an OpenShift cluster with oc login using an API token",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"server": map[string]any{
							"type":        "string",
							"description": "OpenShift cluster API URL (e.g. https://api.mycluster.example.com:6443)",
						},
						"token": map[string]any{
							"type":        "string",
							"description": "API token (starts with sha256~)",
						},
					},
					"required": []string{"server", "token"},
				},
				Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
					var input struct {
						Server string `json:"server"`
						Token  string `json:"token"`
					}
					if err := json.Unmarshal(args, &input); err != nil {
						return "", fmt.Errorf("parsing args: %w", err)
					}

					server := input.Server
					if server == "" {
						server = opts.Server
					}
					if server == "" {
						return "", fmt.Errorf("no server URL provided")
					}

					if input.Token == "" {
						return "", fmt.Errorf("no token provided")
					}

					loginArgs := []string{"login", "--token=" + input.Token, "--server=" + server}
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
		},
		Hooks: plugin.HookHandlers{
			ShellEnv: func(_ context.Context, _ plugin.ShellEnvInput) (*plugin.ShellEnvOutput, error) {
				return &plugin.ShellEnvOutput{
					Env: map[string]string{
						"OC_EDITOR": "cat",
					},
				}, nil
			},
		},
	}
}

func main() {
	opts := options{}
	plugin.Run(newPlugin(opts))
}
