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
	LokiURL  string
	TempoURL string
	Token    string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["lokiUrl"].(string); ok {
		opts.LokiURL = v
	}
	if v, ok := raw["tempoUrl"].(string); ok {
		opts.TempoURL = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	return opts
}

type lokiClient struct {
	api *redhat.APIClient
}

type logEntry struct {
	Timestamp string
	Line      string
	Labels    map[string]string
}

func newLokiClient(baseURL, token string) *lokiClient {
	tokenFn := func(_ context.Context) (string, error) { return token, nil }
	return &lokiClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: baseURL,
			TokenFn: tokenFn,
		}),
	}
}

func (c *lokiClient) query(ctx context.Context, logql string, limit int, start, end string) ([]logEntry, error) {
	params := map[string]string{"query": logql}
	if limit > 0 {
		params["limit"] = fmt.Sprintf("%d", limit)
	}
	if start != "" {
		params["start"] = start
	}
	if end != "" {
		params["end"] = end
	}

	resp, err := c.api.Get(ctx, "/loki/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Result []struct {
				Stream map[string]string  `json:"stream"`
				Values [][2]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parsing loki response: %w", err)
	}

	var entries []logEntry
	for _, stream := range result.Data.Result {
		for _, v := range stream.Values {
			entries = append(entries, logEntry{
				Timestamp: v[0],
				Line:      v[1],
				Labels:    stream.Stream,
			})
		}
	}
	return entries, nil
}

type traceSummary struct {
	TraceID         string `json:"traceID"`
	RootServiceName string `json:"rootServiceName"`
	RootTraceName   string `json:"rootTraceName"`
	DurationMs      int    `json:"durationMs"`
	SpanCount       int    `json:"spanCount"`
}

type span struct {
	TraceID       string            `json:"traceID"`
	SpanID        string            `json:"spanID"`
	OperationName string            `json:"operationName"`
	ServiceName   string            `json:"serviceName"`
	Duration      int               `json:"duration"`
	StartTime     int64             `json:"startTime"`
	Tags          map[string]string `json:"tags"`
	Children      []span            `json:"children"`
}

type traceDetail struct {
	TraceID string `json:"traceID"`
	Spans   []span `json:"spans"`
}

type tempoClient struct {
	api *redhat.APIClient
}

func newTempoClient(baseURL, token string) *tempoClient {
	tokenFn := func(_ context.Context) (string, error) { return token, nil }
	return &tempoClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: baseURL,
			TokenFn: tokenFn,
		}),
	}
}

func (c *tempoClient) searchTraces(ctx context.Context, service string, operation, minDuration string, limit int) ([]traceSummary, error) {
	params := map[string]string{"service.name": service}
	if operation != "" {
		params["name"] = operation
	}
	if minDuration != "" {
		params["minDuration"] = minDuration
	}
	if limit > 0 {
		params["limit"] = fmt.Sprintf("%d", limit)
	}

	resp, err := c.api.Get(ctx, "/api/search", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Traces []traceSummary `json:"traces"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parsing tempo response: %w", err)
	}
	return result.Traces, nil
}

func (c *tempoClient) getTrace(ctx context.Context, traceID string) (*traceDetail, error) {
	resp, err := c.api.Get(ctx, "/api/traces/"+traceID, nil)
	if err != nil {
		return nil, err
	}

	var detail traceDetail
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		return nil, fmt.Errorf("parsing trace detail: %w", err)
	}
	return &detail, nil
}

func formatLogEntries(entries []logEntry) string {
	if len(entries) == 0 {
		return "No log entries found."
	}
	lines := []string{fmt.Sprintf("Log entries: %d", len(entries)), ""}
	for _, e := range entries {
		var labels []string
		for k, v := range e.Labels {
			labels = append(labels, fmt.Sprintf("%s=%q", k, v))
		}
		lines = append(lines, fmt.Sprintf("[%s] {%s} %s", e.Timestamp, strings.Join(labels, ", "), e.Line))
	}
	return strings.Join(lines, "\n")
}

func buildLogQL(namespace, pod, severity string) string {
	var selectors []string
	if namespace != "" {
		selectors = append(selectors, fmt.Sprintf("namespace=%q", namespace))
	}
	if pod != "" {
		selectors = append(selectors, fmt.Sprintf("pod=%q", pod))
	}

	stream := `{job=~".+"}`
	if len(selectors) > 0 {
		stream = fmt.Sprintf("{%s}", strings.Join(selectors, ", "))
	}

	if severity != "" {
		return fmt.Sprintf(`%s |= "%s"`, stream, severity)
	}
	return stream
}

func formatSpanTree(spans []span, indent int) string {
	var lines []string
	prefix := strings.Repeat("  ", indent)
	for _, s := range spans {
		lines = append(lines, fmt.Sprintf("%s[%dms] %s:%s", prefix, s.Duration, s.ServiceName, s.OperationName))
		if len(s.Children) > 0 {
			lines = append(lines, formatSpanTree(s.Children, indent+1))
		}
	}
	return strings.Join(lines, "\n")
}

func buildLogTools(loki *lokiClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "obs_logs",
			Description: "Run a LogQL query against Loki to search logs. Optionally build a query from namespace, pod, and severity filters.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":     map[string]any{"type": "string", "description": "LogQL query expression (overrides filters)"},
					"namespace": map[string]any{"type": "string", "description": "Filter by namespace"},
					"pod":       map[string]any{"type": "string", "description": "Filter by pod name"},
					"severity":  map[string]any{"type": "string", "description": "Filter by severity (e.g. error, warning)"},
					"since":     map[string]any{"type": "string", "description": "Start time (RFC3339 or relative like '1h')"},
					"limit":     map[string]any{"type": "integer", "description": "Max entries to return"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Query     string `json:"query"`
					Namespace string `json:"namespace"`
					Pod       string `json:"pod"`
					Severity  string `json:"severity"`
					Since     string `json:"since"`
					Limit     int    `json:"limit"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				logql := input.Query
				if logql == "" {
					logql = buildLogQL(input.Namespace, input.Pod, input.Severity)
				}

				entries, err := loki.query(ctx, logql, input.Limit, input.Since, "")
				if err != nil {
					return fmt.Sprintf("Log query failed: %v", err), nil
				}
				return formatLogEntries(entries), nil
			},
		},
	}
}

func buildTraceTools(tempo *tempoClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "obs_traces",
			Description: "Search distributed traces by service name, operation, and minimum duration.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"service":     map[string]any{"type": "string", "description": "Service name to search traces for"},
					"operation":   map[string]any{"type": "string", "description": "Filter by operation name"},
					"minDuration": map[string]any{"type": "string", "description": "Minimum trace duration (e.g. '500ms', '1s')"},
					"limit":       map[string]any{"type": "integer", "description": "Max traces to return"},
				},
				"required": []string{"service"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Service     string `json:"service"`
					Operation   string `json:"operation"`
					MinDuration string `json:"minDuration"`
					Limit       int    `json:"limit"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				traces, err := tempo.searchTraces(ctx, input.Service, input.Operation, input.MinDuration, input.Limit)
				if err != nil {
					return fmt.Sprintf("Trace search failed: %v", err), nil
				}
				if len(traces) == 0 {
					return "No traces found matching criteria.", nil
				}

				lines := []string{fmt.Sprintf("Traces: %d", len(traces)), ""}
				for _, t := range traces {
					lines = append(lines, fmt.Sprintf("%s | %s:%s | %dms | %d spans",
						t.TraceID, t.RootServiceName, t.RootTraceName, t.DurationMs, t.SpanCount))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "obs_trace_detail",
			Description: "Get the full span tree for a specific trace ID, showing service calls and timing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"traceId": map[string]any{"type": "string", "description": "Trace ID to retrieve"},
				},
				"required": []string{"traceId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					TraceID string `json:"traceId"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				detail, err := tempo.getTrace(ctx, input.TraceID)
				if err != nil {
					return fmt.Sprintf("Failed to get trace: %v", err), nil
				}
				if len(detail.Spans) == 0 {
					return "Trace has no spans.", nil
				}

				lines := []string{
					fmt.Sprintf("Trace: %s (%d spans)", detail.TraceID, len(detail.Spans)),
					"",
					formatSpanTree(detail.Spans, 0),
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func buildOcTools(oc *redhat.OcClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "obs_flow_collectors",
			Description: "List FlowCollector resources from the Network Observability operator. Shows agent type, log types, and readiness.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				result, err := oc.Get(ctx, "flowcollectors.flows.netobserv.io", nil)
				if err != nil {
					return fmt.Sprintf("Failed to list flow collectors: %v", err), nil
				}

				var fcList struct {
					Items []struct {
						Metadata struct{ Name, Namespace string } `json:"metadata"`
						Spec     struct {
							Agent     struct{ Type string } `json:"agent"`
							Processor struct{ LogTypes string } `json:"processor"`
						} `json:"spec"`
						Status struct {
							Conditions []struct {
								Type   string `json:"type"`
								Status string `json:"status"`
							} `json:"conditions"`
						} `json:"status"`
					} `json:"items"`
				}
				if err := json.Unmarshal(result, &fcList); err != nil {
					return fmt.Sprintf("Failed to parse flow collectors: %v", err), nil
				}

				if len(fcList.Items) == 0 {
					return "No FlowCollector resources found. Network Observability may not be installed.", nil
				}

				lines := []string{"Flow Collectors:", ""}
				for _, fc := range fcList.Items {
					agentType := fc.Spec.Agent.Type
					if agentType == "" {
						agentType = "unknown"
					}
					logTypes := fc.Spec.Processor.LogTypes
					if logTypes == "" {
						logTypes = "unknown"
					}
					status := "Unknown"
					for _, c := range fc.Status.Conditions {
						if c.Type == "Ready" {
							status = c.Status
							break
						}
					}
					lines = append(lines, fmt.Sprintf("%s | agent: %s | logTypes: %s | ready: %s",
						fc.Metadata.Name, agentType, logTypes, status))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "obs_dashboards",
			Description: "List available observability dashboards from OpenShift ConfigMaps.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				result, err := oc.Get(ctx, "configmaps", &redhat.OcGetOptions{
					Namespace: "openshift-config-managed",
					Selector:  "grafana_dashboard=1",
				})
				if err != nil {
					return fmt.Sprintf("Failed to list dashboards: %v", err), nil
				}

				var cmList struct {
					Items []struct {
						Metadata struct {
							Name      string `json:"name"`
							Namespace string `json:"namespace"`
						} `json:"metadata"`
					} `json:"items"`
				}
				if err := json.Unmarshal(result, &cmList); err != nil {
					return fmt.Sprintf("Failed to parse dashboards: %v", err), nil
				}

				if len(cmList.Items) == 0 {
					return "No dashboards found.", nil
				}

				lines := []string{fmt.Sprintf("Dashboards: %d", len(cmList.Items)), ""}
				for _, cm := range cmList.Items {
					lines = append(lines, fmt.Sprintf("%s | %s", cm.Metadata.Name, cm.Metadata.Namespace))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func unconfiguredLogTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "obs_logs",
		Description: "Run a LogQL query against Loki to search logs.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "LogQL query expression"},
			},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			return "Logging not configured. Set lokiUrl in plugin options to your Loki endpoint.", nil
		},
	}
}

func unconfiguredTraceTools() []plugin.ToolDef {
	msg := "Tracing not configured. Set tempoUrl in plugin options to your Tempo endpoint."
	return []plugin.ToolDef{
		{
			Name:        "obs_traces",
			Description: "Search distributed traces by service name.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"service": map[string]any{"type": "string", "description": "Service name"},
				},
				"required": []string{"service"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "obs_trace_detail",
			Description: "Get the full span tree for a trace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"traceId": map[string]any{"type": "string", "description": "Trace ID"},
				},
				"required": []string{"traceId"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func buildHealthTool(loki *lokiClient, tempo *tempoClient, oc *redhat.OcClient) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "logging_health",
		Description: "Check health of logging and tracing services (Loki, Tempo, ClusterLogging operator).",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			var lines []string

			// Check Loki
			if loki != nil {
				if _, err := loki.api.Get(ctx, "/ready", nil); err != nil {
					lines = append(lines, fmt.Sprintf("[DOWN] Loki: %v", err))
				} else {
					lines = append(lines, "[OK] Loki")
				}
			} else {
				lines = append(lines, "[SKIP] Loki (not configured)")
			}

			// Check Tempo
			if tempo != nil {
				if _, err := tempo.api.Get(ctx, "/ready", nil); err != nil {
					lines = append(lines, fmt.Sprintf("[DOWN] Tempo: %v", err))
				} else {
					lines = append(lines, "[OK] Tempo")
				}
			} else {
				lines = append(lines, "[SKIP] Tempo (not configured)")
			}

			// Check ClusterLogging operator status
			if _, err := oc.Get(ctx, "clusterloggings.logging.openshift.io", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] ClusterLogging operator: %v", err))
			} else {
				lines = append(lines, "[OK] ClusterLogging operator")
			}

			// Check ClusterLogForwarder status
			if _, err := oc.Get(ctx, "clusterlogforwarders.logging.openshift.io", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] ClusterLogForwarder: %v", err))
			} else {
				lines = append(lines, "[OK] ClusterLogForwarder")
			}

			return "Service Health:\n" + strings.Join(lines, "\n"), nil
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	oc := redhat.NewOcClient()

	var tools []plugin.ToolDef

	var loki *lokiClient
	if opts.LokiURL != "" {
		loki = newLokiClient(opts.LokiURL, opts.Token)
		tools = append(tools, buildLogTools(loki)...)
	} else {
		tools = append(tools, unconfiguredLogTool())
	}

	var tempo *tempoClient
	if opts.TempoURL != "" {
		tempo = newTempoClient(opts.TempoURL, opts.Token)
		tools = append(tools, buildTraceTools(tempo)...)
	} else {
		tools = append(tools, unconfiguredTraceTools()...)
	}

	tools = append(tools, buildOcTools(oc)...)
	tools = append(tools, buildHealthTool(loki, tempo, oc))

	return plugin.Plugin{
		ID:    "ocp-obs-logging",
		Tools: tools,
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
