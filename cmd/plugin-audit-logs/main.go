package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func main() {
	plugin.Run(newPlugin())
}

func newPlugin() plugin.Plugin {
	return plugin.Plugin{
		ID:    "audit-logs",
		Tools: buildTools(),
	}
}

func buildTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		buildAuditTop(),
		buildAuditSearch(),
		buildAuditTimeline(),
		buildAuditAnomalies(),
		buildAuditHealth(),
	}
}

func buildAuditTop() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "audit_top",
		Description: "Aggregate audit events and show top-N by user, verb, resource, or namespace. Stream-parses JSON audit logs without loading full files into memory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string", "description": "Path to must-gather directory"},
				"by":        map[string]any{"type": "string", "description": "Aggregate by: user, verb, resource, or namespace"},
				"n":         map[string]any{"type": "integer", "description": "Number of top results (default 20)"},
				"verb":      map[string]any{"type": "string", "description": "Filter by verb (get, list, create, update, delete, watch, patch)"},
				"namespace": map[string]any{"type": "string", "description": "Filter by namespace"},
			},
			"required": []string{"path", "by"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path      string `json:"path"`
				By        string `json:"by"`
				N         int    `json:"n"`
				Verb      string `json:"verb"`
				Namespace string `json:"namespace"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			switch input.By {
			case "user", "verb", "resource", "namespace":
			default:
				return "", fmt.Errorf("by must be one of: user, verb, resource, namespace")
			}
			if input.N <= 0 {
				input.N = 20
			}
			return toolAuditTop(input.Path, input.By, input.N, &filter{
				Verb:      input.Verb,
				Namespace: input.Namespace,
			})
		},
	}
}

func buildAuditSearch() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "audit_search",
		Description: "Search audit events by user, verb, resource, namespace, or status code. Returns matching events with details.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "Path to must-gather directory"},
				"user":        map[string]any{"type": "string", "description": "Filter by username (substring match)"},
				"verb":        map[string]any{"type": "string", "description": "Filter by verb"},
				"resource":    map[string]any{"type": "string", "description": "Filter by resource type"},
				"namespace":   map[string]any{"type": "string", "description": "Filter by namespace"},
				"status_code": map[string]any{"type": "integer", "description": "Filter by HTTP status code"},
				"max":         map[string]any{"type": "integer", "description": "Maximum results to return (default 50)"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path       string `json:"path"`
				User       string `json:"user"`
				Verb       string `json:"verb"`
				Resource   string `json:"resource"`
				Namespace  string `json:"namespace"`
				StatusCode int    `json:"status_code"`
				Max        int    `json:"max"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			if input.Max <= 0 {
				input.Max = 50
			}
			return toolAuditSearch(input.Path, input.Max, &filter{
				Verb:       input.Verb,
				User:       input.User,
				Resource:   input.Resource,
				Namespace:  input.Namespace,
				StatusCode: input.StatusCode,
			})
		},
	}
}

func buildAuditTimeline() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "audit_timeline",
		Description: "Bucket audit events by time interval and show event volume over time. Identifies spikes in activity.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":     map[string]any{"type": "string", "description": "Path to must-gather directory"},
				"interval": map[string]any{"type": "string", "description": "Time bucket interval: minute or hour (default hour)"},
				"verb":     map[string]any{"type": "string", "description": "Filter by verb"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path     string `json:"path"`
				Interval string `json:"interval"`
				Verb     string `json:"verb"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			if input.Interval == "" {
				input.Interval = "hour"
			}
			if input.Interval != "minute" && input.Interval != "hour" {
				return "", fmt.Errorf("interval must be minute or hour")
			}
			return toolAuditTimeline(input.Path, input.Interval, &filter{Verb: input.Verb})
		},
	}
}

func buildAuditAnomalies() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "audit_anomalies",
		Description: "Detect unusual patterns in audit logs: failed auth spikes (401/403), mass deletions, privilege escalation attempts, and unusual service account activity.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path to must-gather directory"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			return toolAuditAnomalies(input.Path)
		},
	}
}

func buildAuditHealth() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "audit_health",
		Description: "Overall audit log health summary with [OK], [WARN], and [CRITICAL] ratings covering auth failures, deletion rates, privilege escalation, and event volume.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path to must-gather directory"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			return toolAuditHealth(input.Path)
		},
	}
}
