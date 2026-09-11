package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type clusterContext struct {
	Cluster   string   `json:"cluster"`
	Version   string   `json:"version"`
	Nodes     string   `json:"nodes"`
	Namespace string   `json:"namespace"`
	Operators []string `json:"operators"`
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
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["consoleOfflineToken"].(string); ok {
		opts.ConsoleOfflineToken = v
	}
	if v, ok := raw["clientId"].(string); ok {
		opts.ClientID = v
	}
	return opts
}

type state struct {
	mu          sync.RWMutex
	clusterCtx  *clusterContext
	costCtx     *costContext
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
				Metadata struct{ Name string `json:"name"` } `json:"metadata"`
				Spec     struct{ DisplayName string `json:"displayName"` } `json:"spec"`
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
		},
		Hooks: plugin.HookHandlers{
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) error {
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
				return nil
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

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
