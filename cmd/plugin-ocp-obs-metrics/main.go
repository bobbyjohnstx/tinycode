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

type options struct {
	PrometheusURL   string
	AlertManagerURL string
	Token           string
	Namespace       string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["prometheusUrl"].(string); ok {
		opts.PrometheusURL = v
	}
	if v, ok := raw["alertManagerUrl"].(string); ok {
		opts.AlertManagerURL = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	if v, ok := raw["namespace"].(string); ok {
		opts.Namespace = v
	}
	return opts
}

type alertState struct {
	mu      sync.RWMutex
	summary string
}

func formatVectorResult(data json.RawMessage) string {
	var vectors []struct {
		Metric map[string]string `json:"metric"`
		Value  [2]any            `json:"value"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		return "Failed to parse vector results."
	}
	if len(vectors) == 0 {
		return "Query returned no results."
	}
	lines := []string{fmt.Sprintf("Results: %d vectors", len(vectors)), ""}
	for _, v := range vectors {
		var labels []string
		for k, val := range v.Metric {
			labels = append(labels, fmt.Sprintf("%s=%q", k, val))
		}
		value := fmt.Sprintf("%v", v.Value[1])
		lines = append(lines, fmt.Sprintf("{%s} => %s", strings.Join(labels, ", "), value))
	}
	return strings.Join(lines, "\n")
}

func formatMatrixResult(data json.RawMessage) string {
	var matrices []struct {
		Metric map[string]string `json:"metric"`
		Values [][2]any          `json:"values"`
	}
	if err := json.Unmarshal(data, &matrices); err != nil {
		return "Failed to parse matrix results."
	}
	if len(matrices) == 0 {
		return "Query returned no results."
	}
	lines := []string{fmt.Sprintf("Results: %d series", len(matrices)), ""}
	for _, m := range matrices {
		var labels []string
		for k, val := range m.Metric {
			labels = append(labels, fmt.Sprintf("%s=%q", k, val))
		}
		lines = append(lines, fmt.Sprintf("{%s}:", strings.Join(labels, ", ")))
		for _, v := range m.Values {
			lines = append(lines, fmt.Sprintf("  %v => %v", v[0], v[1]))
		}
		lines = append(lines, "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func formatAlertSummary(alerts []redhat.Alert) string {
	bySeverity := map[string][]string{}
	for _, a := range alerts {
		severity := a.Labels["severity"]
		if severity == "" {
			severity = "unknown"
		}
		name := a.Labels["alertname"]
		if name == "" {
			name = "unknown"
		}
		bySeverity[severity] = append(bySeverity[severity], name)
	}

	var parts []string
	for _, severity := range []string{"critical", "warning", "info"} {
		names := bySeverity[severity]
		if len(names) > 0 {
			parts = append(parts, fmt.Sprintf("%d %s (%s)", len(names), severity, strings.Join(names, ", ")))
		}
	}

	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func buildObsTools(client *redhat.PromQLClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "obs_promql",
			Description: "Run an arbitrary PromQL query against Prometheus/Thanos. Supports both instant and range queries.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "PromQL query expression"},
					"time":  map[string]any{"type": "string", "description": "Evaluation timestamp for instant query (RFC3339 or Unix)"},
					"start": map[string]any{"type": "string", "description": "Range query start time (RFC3339 or Unix)"},
					"end":   map[string]any{"type": "string", "description": "Range query end time (RFC3339 or Unix)"},
					"step":  map[string]any{"type": "string", "description": "Range query step (e.g. '15s', '1m')"},
				},
				"required": []string{"query"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Query string `json:"query"`
					Time  string `json:"time"`
					Start string `json:"start"`
					End   string `json:"end"`
					Step  string `json:"step"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				if input.Start != "" && input.End != "" && input.Step != "" {
					result, err := client.RangeQuery(ctx, input.Query, input.Start, input.End, input.Step)
					if err != nil {
						return fmt.Sprintf("PromQL query failed: %v", err), nil
					}
					resultJSON, _ := json.Marshal(result.Result)
					return formatMatrixResult(resultJSON), nil
				}

				result, err := client.InstantQuery(ctx, input.Query, input.Time)
				if err != nil {
					return fmt.Sprintf("PromQL query failed: %v", err), nil
				}
				resultJSON, _ := json.Marshal(result.Result)
				return formatVectorResult(resultJSON), nil
			},
		},
		{
			Name:        "obs_alerts",
			Description: "List active firing alerts from AlertManager. Filter by severity or namespace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"severity":  map[string]any{"type": "string", "description": "Filter alerts by severity (critical, warning, info)"},
					"namespace": map[string]any{"type": "string", "description": "Filter alerts by namespace"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Severity  string `json:"severity"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				active := true
				silenced := false
				alerts, err := client.Alerts(ctx, &active, &silenced)
				if err != nil {
					return fmt.Sprintf("Failed to query alerts: %v", err), nil
				}

				var filtered []redhat.Alert
				for _, a := range alerts {
					if input.Severity != "" && a.Labels["severity"] != input.Severity {
						continue
					}
					if input.Namespace != "" && a.Labels["namespace"] != input.Namespace {
						continue
					}
					filtered = append(filtered, a)
				}

				if len(filtered) == 0 {
					return "No active alerts matching filters.", nil
				}

				lines := []string{fmt.Sprintf("Active Alerts: %d", len(filtered)), ""}
				for _, a := range filtered {
					severity := strings.ToUpper(a.Labels["severity"])
					if severity == "" {
						severity = "UNKNOWN"
					}
					name := a.Labels["alertname"]
					if name == "" {
						name = "unknown"
					}
					ns := a.Labels["namespace"]
					if ns == "" {
						ns = "cluster"
					}
					desc := a.Annotations["description"]
					if desc == "" {
						desc = "No description"
					}
					lines = append(lines, fmt.Sprintf("[%s] %s | %s | since %s | %s",
						severity, name, ns, a.ActiveAt, desc))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "obs_alert_silence",
			Description: "Silence a firing alert in AlertManager for a specified duration.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alertName": map[string]any{"type": "string", "description": "Name of the alert to silence"},
					"duration":  map[string]any{"type": "string", "description": "Silence duration (e.g. '1h', '30m', '2h')"},
					"comment":   map[string]any{"type": "string", "description": "Reason for silencing the alert"},
				},
				"required": []string{"alertName", "duration", "comment"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					AlertName string `json:"alertName"`
					Duration  string `json:"duration"`
					Comment   string `json:"comment"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				if _, err := redhat.ParseDuration(input.Duration); err != nil {
					return fmt.Sprintf("Invalid duration: %v", err), nil
				}

				matchers := []redhat.AlertMatcher{
					{Name: "alertname", Value: input.AlertName, IsRegex: false, IsEqual: true},
				}
				silenceID, err := client.SilenceAlert(ctx, matchers, input.Duration, "tinycode", input.Comment)
				if err != nil {
					return fmt.Sprintf("Failed to silence alert: %v", err), nil
				}
				return fmt.Sprintf("Alert '%s' silenced for %s. Silence ID: %s", input.AlertName, input.Duration, silenceID), nil
			},
		},
	}
}

func unconfiguredObsTools() []plugin.ToolDef {
	msg := "Observability plugin not configured. Set prometheusUrl in plugin options to your Prometheus/Thanos endpoint."
	return []plugin.ToolDef{
		{
			Name:        "obs_promql",
			Description: "Run an arbitrary PromQL query against Prometheus/Thanos.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "PromQL query expression"},
				},
				"required": []string{"query"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "obs_alerts",
			Description: "List active firing alerts from AlertManager.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"severity": map[string]any{"type": "string", "description": "Filter alerts by severity"},
				},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "obs_alert_silence",
			Description: "Silence a firing alert in AlertManager for a specified duration.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alertName": map[string]any{"type": "string", "description": "Alert name"},
					"duration":  map[string]any{"type": "string", "description": "Silence duration"},
					"comment":   map[string]any{"type": "string", "description": "Reason"},
				},
				"required": []string{"alertName", "duration", "comment"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	if opts.PrometheusURL == "" {
		return plugin.Plugin{
			ID:    "ocp-obs-metrics",
			Tools: unconfiguredObsTools(),
		}
	}

	tokenFn := func(_ context.Context) (string, error) { return opts.Token, nil }
	client := redhat.NewPromQLClient(redhat.PromQLClientConfig{
		BaseURL:         opts.PrometheusURL,
		TokenFn:         tokenFn,
		AlertManagerURL: opts.AlertManagerURL,
	})

	as := &alertState{}

	return plugin.Plugin{
		ID:    "ocp-obs-metrics",
		Tools: buildObsTools(client),
		Hooks: plugin.HookHandlers{
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) error {
				slog.Info("ocp-obs-metrics: fetching alert summary", "sessionId", event.SessionID)
				active := true
				silenced := false
				alerts, err := client.Alerts(ctx, &active, &silenced)
				if err != nil {
					slog.Warn("ocp-obs-metrics: failed to fetch alerts", "error", err)
					return nil
				}
				if len(alerts) > 0 {
					as.mu.Lock()
					as.summary = formatAlertSummary(alerts)
					as.mu.Unlock()
				}
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
