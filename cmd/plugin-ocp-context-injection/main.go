package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type clusterContext struct {
	Cluster   string        `json:"cluster"`
	Version   string        `json:"version"`
	Nodes     string        `json:"nodes"`
	Namespace string        `json:"namespace"`
	Operators []string      `json:"operators"`
	Alerts    *alertSummary `json:"alerts,omitempty"`
}

type alertSummary struct {
	Critical []alertEntry `json:"critical"`
	Warning  []alertEntry `json:"warning"`
	Info     int          `json:"info"`
}

type alertEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type costContext struct {
	Namespace       string `json:"namespace,omitempty"`
	MonthlyEstimate string `json:"monthlyEstimate"`
	TopResource     string `json:"topResource,omitempty"`
	TopResourceCost string `json:"topResourceCost,omitempty"`
	Currency        string `json:"currency"`
}

type options struct {
	ConsoleOfflineToken string
	ClientID            string
	APIURL              string
	ClusterID           string
	InsecureSkipTLS     bool
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["consoleOfflineToken"].(string); ok {
		opts.ConsoleOfflineToken = v
	}
	if v, ok := raw["clientId"].(string); ok {
		opts.ClientID = v
	}
	if v, ok := raw["apiUrl"].(string); ok {
		opts.APIURL = v
	}
	if v, ok := raw["clusterId"].(string); ok {
		opts.ClusterID = v
	}
	if v, ok := raw["insecureSkipTlsVerify"].(bool); ok {
		opts.InsecureSkipTLS = v
	}
	return opts
}

type state struct {
	mu         sync.RWMutex
	clusterCtx *clusterContext
	costCtx    *costContext
}

func queryFiringAlerts(ctx context.Context, oc *redhat.OcClient) *alertSummary {
	out, err := oc.Raw(ctx,
		"-n", "openshift-monitoring", "exec", "-c", "alertmanager", "alertmanager-main-0",
		"--", "curl", "-s", "http://localhost:9093/api/v2/alerts?active=true&silenced=false",
	)
	if err != nil {
		return nil
	}

	var alerts []struct {
		Labels struct {
			Alertname string `json:"alertname"`
			Namespace string `json:"namespace"`
			Severity  string `json:"severity"`
		} `json:"labels"`
	}
	if err := json.Unmarshal([]byte(out), &alerts); err != nil {
		return nil
	}

	summary := &alertSummary{}
	for _, a := range alerts {
		name := a.Labels.Alertname
		if name == "" {
			name = "Unknown"
		}
		ns := a.Labels.Namespace
		switch a.Labels.Severity {
		case "critical":
			summary.Critical = append(summary.Critical, alertEntry{Name: name, Namespace: ns})
		case "warning":
			summary.Warning = append(summary.Warning, alertEntry{Name: name, Namespace: ns})
		case "info":
			summary.Info++
		}
	}

	if len(summary.Critical) == 0 && len(summary.Warning) == 0 && summary.Info == 0 {
		return nil
	}
	return summary
}

func queryClusterContext(ctx context.Context, oc *redhat.OcClient) *clusterContext {
	if !oc.IsAvailable(ctx) || !oc.IsLoggedIn(ctx) {
		return nil
	}

	version := "unknown"
	if v, err := oc.Version(ctx); err == nil {
		if v.OpenshiftVersion != "" {
			version = v.OpenshiftVersion
		} else if v.ServerVersion != nil {
			version = v.ServerVersion["major"] + "." + v.ServerVersion["minor"]
		}
	}

	nodes := "unknown"
	if nodesJSON, err := oc.Get(ctx, "nodes", nil); err == nil {
		var nodeList struct {
			Items []struct {
				Metadata struct {
					Labels map[string]string `json:"labels"`
				} `json:"metadata"`
			} `json:"items"`
		}
		if json.Unmarshal(nodesJSON, &nodeList) == nil {
			total := len(nodeList.Items)
			controlPlane, worker := 0, 0
			for _, n := range nodeList.Items {
				if _, ok := n.Metadata.Labels["node-role.kubernetes.io/control-plane"]; ok {
					controlPlane++
				} else if _, ok := n.Metadata.Labels["node-role.kubernetes.io/master"]; ok {
					controlPlane++
				}
				if _, ok := n.Metadata.Labels["node-role.kubernetes.io/worker"]; ok {
					worker++
				}
			}
			nodes = fmt.Sprintf("%d (%d control-plane, %d worker)", total, controlPlane, worker)
		}
	}

	cluster, namespace := "unknown", "unknown"
	if ctxStr, err := oc.Raw(ctx, "config", "current-context"); err == nil {
		parts := strings.Split(strings.TrimSpace(ctxStr), "/")
		if len(parts) >= 2 {
			namespace = parts[0]
			cluster = parts[1]
		} else if len(parts) == 1 {
			cluster = parts[0]
		}
	}

	var operators []string
	if csvOut, err := oc.Raw(ctx, "get", "csv", "-A", "-o", "json"); err == nil {
		var csvData struct {
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Spec struct {
					DisplayName string `json:"displayName"`
				} `json:"spec"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(csvOut), &csvData) == nil {
			seen := make(map[string]bool)
			for _, item := range csvData.Items {
				name := item.Spec.DisplayName
				if name == "" {
					name = item.Metadata.Name
					if idx := strings.Index(name, ".v"); idx > 0 {
						name = name[:idx]
					}
				}
				if name != "" && !seen[name] {
					seen[name] = true
					operators = append(operators, name)
				}
			}
		}
	}

	alerts := queryFiringAlerts(ctx, oc)

	return &clusterContext{
		Cluster:   cluster,
		Version:   version,
		Nodes:     nodes,
		Namespace: namespace,
		Operators: operators,
		Alerts:    alerts,
	}
}

func formatAlertLine(label string, entries []alertEntry) string {
	var details []string
	for _, a := range entries {
		ns := a.Namespace
		if ns == "" {
			ns = "cluster"
		}
		details = append(details, fmt.Sprintf("%s: %s", a.Name, ns))
	}
	return fmt.Sprintf("%s: %d (%s)", label, len(entries), strings.Join(details, ", "))
}

func formatContextBlock(cc *clusterContext) string {
	lines := []string{
		fmt.Sprintf("cluster: %s", cc.Cluster),
		fmt.Sprintf("version: %s", cc.Version),
		fmt.Sprintf("nodes: %s", cc.Nodes),
		fmt.Sprintf("namespace: %s", cc.Namespace),
		fmt.Sprintf("operators: [%s]", strings.Join(cc.Operators, ", ")),
	}

	if cc.Alerts != nil {
		if len(cc.Alerts.Critical) > 0 {
			lines = append(lines, formatAlertLine("firing-alerts-critical", cc.Alerts.Critical))
		}
		if len(cc.Alerts.Warning) > 0 {
			lines = append(lines, formatAlertLine("firing-alerts-warning", cc.Alerts.Warning))
		}
		if cc.Alerts.Info > 0 {
			lines = append(lines, fmt.Sprintf("firing-alerts-info: %d", cc.Alerts.Info))
		}
	}

	return fmt.Sprintf("<cluster-context>\n%s\n</cluster-context>", strings.Join(lines, "\n"))
}

func formatCostBlock(cc *costContext) string {
	prefix := ""
	if cc.Currency == "USD" {
		prefix = "$"
	}
	var parts []string
	if cc.Namespace != "" {
		parts = append(parts, "namespace="+cc.Namespace)
	}
	parts = append(parts, fmt.Sprintf("monthly-cost=%s%s", prefix, cc.MonthlyEstimate))
	if cc.TopResource != "" {
		resourceCost := "?"
		if cc.TopResourceCost != "" {
			resourceCost = prefix + cc.TopResourceCost
		}
		parts = append(parts, fmt.Sprintf("top-resource=%s (%s/mo)", cc.TopResource, resourceCost))
	}
	return fmt.Sprintf("<cost-context>%s</cost-context>", strings.Join(parts, " "))
}

func newPlugin(opts options) plugin.Plugin {
	oc := redhat.NewOcClient()
	s := &state{}

	return plugin.Plugin{
		ID: "ocp-context-injection",
		Tools: []plugin.ToolDef{
			{
				Name:        "cluster_context",
				Description: "Get current OpenShift cluster context including version, nodes, operators, and alerts",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
				Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
					s.mu.RLock()
					defer s.mu.RUnlock()

					var result string
					if s.clusterCtx != nil {
						result = formatContextBlock(s.clusterCtx)
					} else {
						result = "<cluster-context>not connected</cluster-context>"
					}
					if s.costCtx != nil {
						result += "\n" + formatCostBlock(s.costCtx)
					}
					return result, nil
				},
			},
			{
				Name:        "oc_login",
				Description: "Authenticate to an OpenShift cluster using oc login with API token",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"server": map[string]any{
							"type":        "string",
							"description": "OpenShift cluster API URL (e.g. https://api.mycluster.example.com:6443)",
						},
						"token": map[string]any{
							"type":        "string",
							"description": "Authentication token",
						},
					},
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

					return ocLogin(ctx, server, token, opts.InsecureSkipTLS)
				},
			},
		},
		Hooks: plugin.HookHandlers{
			ShellEnv: func(_ context.Context, _ plugin.ShellEnvInput) (*plugin.ShellEnvOutput, error) {
				env := map[string]string{
					"OC_EDITOR": "cat",
				}
				if opts.ClusterID != "" {
					env["CLUSTER_ID"] = opts.ClusterID
				}
				return &plugin.ShellEnvOutput{Env: env}, nil
			},
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) (*plugin.SessionStartOutput, error) {
				slog.Info("ocp-context-injection: gathering cluster context", "sessionId", event.SessionID)

				cc := queryClusterContext(ctx, oc)
				s.mu.Lock()
				s.clusterCtx = cc
				s.mu.Unlock()

				if opts.ConsoleOfflineToken != "" && cc != nil {
					apiClient := redhat.NewConsoleAPIClient(
						redhat.ConsoleAuthConfig{OfflineToken: opts.ConsoleOfflineToken, ClientID: opts.ClientID},
						"/api/cost-management/v1",
						nil,
					)
					costResp, err := apiClient.Get(ctx, "/reports/openshift/costs/", map[string]string{
						"filter[cluster]":          cc.Cluster,
						"filter[time_scope_value]": "-1",
						"filter[time_scope_units]": "month",
						"filter[project]":          cc.Namespace,
					})
					if err == nil {
						var cost costContext
						if json.Unmarshal(costResp.Data, &cost) == nil {
							s.mu.Lock()
							s.costCtx = &cost
							s.mu.Unlock()
						}
					}
				}
				return nil, nil
			},
			Dispose: func(_ context.Context) error {
				s.mu.Lock()
				s.clusterCtx = nil
				s.costCtx = nil
				s.mu.Unlock()
				return nil
			},
		},
	}
}

func validateOcTarget(server, token string) error {
	if server == "" || strings.ContainsAny(server, " \t\r\n") || strings.HasPrefix(server, "-") {
		return fmt.Errorf("invalid server URL")
	}
	u, err := url.Parse(server)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("server must be an https URL")
	}
	if token == "" || strings.ContainsAny(token, " \t\r\n'\"\\") || strings.HasPrefix(token, "-") {
		return fmt.Errorf("invalid token")
	}
	return nil
}

func redactSecret(text, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "[REDACTED]")
}

func kubeconfigDest() (string, error) {
	if v := os.Getenv("KUBECONFIG"); v != "" && !strings.Contains(v, string(os.PathListSeparator)) {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".kube", "config"), nil
}

func writeKubeconfig(path, server, token string, insecure bool) error {
	if err := validateOcTarget(server, token); err != nil {
		return err
	}
	insecureLine := "    insecure-skip-tls-verify: false\n"
	if insecure {
		insecureLine = "    insecure-skip-tls-verify: true\n"
	}
	body := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: tinycode
  cluster:
    server: %s
%susers:
- name: tinycode
  user:
    token: %s
contexts:
- name: tinycode
  context:
    cluster: tinycode
    user: tinycode
current-context: tinycode
`, server, insecureLine, token)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// ocLogin authenticates with oc without placing the token on the process
// argument list. The token is written to a 0600 kubeconfig and verified with
// oc whoami --kubeconfig.
func ocLogin(ctx context.Context, server, token string, insecure bool) (string, error) {
	if err := validateOcTarget(server, token); err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "oc-login-*")
	if err != nil {
		return "", fmt.Errorf("creating login dir: %w", err)
	}
	defer os.RemoveAll(dir)

	kcPath := filepath.Join(dir, "config")
	if err := writeKubeconfig(kcPath, server, token, insecure); err != nil {
		return "", fmt.Errorf("writing kubeconfig: %w", err)
	}

	cmd := exec.CommandContext(ctx, "oc", "whoami", "--kubeconfig="+kcPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("oc login failed: %s", strings.TrimSpace(redactSecret(string(out), token)))
	}
	user := strings.TrimSpace(redactSecret(string(out), token))

	dest, err := kubeconfigDest()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return "", fmt.Errorf("creating kubeconfig dir: %w", err)
	}
	if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
		data, err := os.ReadFile(kcPath)
		if err != nil {
			return "", fmt.Errorf("reading kubeconfig: %w", err)
		}
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			return "", fmt.Errorf("writing kubeconfig: %w", err)
		}
		if err := os.Chmod(dest, 0o600); err != nil {
			return "", fmt.Errorf("chmod kubeconfig: %w", err)
		}
	} else {
		merge := exec.CommandContext(ctx, "oc", "config", "view", "--flatten")
		merge.Env = append(os.Environ(), "KUBECONFIG="+kcPath+string(os.PathListSeparator)+dest)
		merged, err := merge.Output()
		if err != nil {
			detail := string(merged)
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
				if strings.TrimSpace(detail) != "" {
					detail += "\n"
				}
				detail += string(exitErr.Stderr)
			}
			detail = strings.TrimSpace(redactSecret(detail, token))
			if detail == "" {
				detail = err.Error()
			}
			return "", fmt.Errorf("merging kubeconfig: %s", detail)
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, merged, 0o600); err != nil {
			return "", fmt.Errorf("writing kubeconfig: %w", err)
		}
		if err := os.Chmod(tmp, 0o600); err != nil {
			return "", fmt.Errorf("chmod kubeconfig: %w", err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			return "", fmt.Errorf("replacing kubeconfig: %w", err)
		}
	}

	return fmt.Sprintf("Logged in to %s as %s", server, user), nil
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
