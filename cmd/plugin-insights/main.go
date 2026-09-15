package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type state struct {
	mu      sync.RWMutex
	rootDir string
}

func (s *state) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rootDir
}

func (s *state) set(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rootDir = dir
}

func newPlugin() plugin.Plugin {
	st := &state{}
	return plugin.Plugin{
		ID:    "insights",
		Tools: buildTools(st),
	}
}

func main() {
	plugin.Run(newPlugin())
}

func requireRoot(st *state) (string, error) {
	dir := st.get()
	if dir == "" {
		return "", fmt.Errorf("no Insights archive loaded — call insights_use first")
	}
	return dir, nil
}

func buildTools(st *state) []plugin.ToolDef {
	return []plugin.ToolDef{
		buildInsightsUse(st),
		buildInsightsSummary(st),
		buildInsightsNodes(st),
		buildInsightsOperators(st),
		buildInsightsMemory(st),
		buildInsightsEtcd(st),
		buildInsightsStorage(st),
		buildInsightsAlerts(st),
		buildInsightsUIDOverlap(st),
		buildInsightsHealth(st),
	}
}

func buildInsightsUse(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_use",
		Description: "Extract an Insights archive (.tar.gz) to a temp directory and validate its structure.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"archive": map[string]any{"type": "string", "description": "Path to Insights .tar.gz archive"},
			},
			"required": []string{"archive"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Archive string `json:"archive"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			summary, rootDir, err := toolUse(input.Archive)
			if err != nil {
				return "", err
			}
			st.set(rootDir)
			return summary, nil
		},
	}
}

func buildInsightsSummary(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_summary",
		Description: "Show cluster summary: cluster version, platform, node count, operator health.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolSummary(root)
		},
	}
}

func buildInsightsNodes(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_nodes",
		Description: "List nodes with roles, status, and capacity from the Insights archive.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolNodes(root)
		},
	}
}

func buildInsightsOperators(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_operators",
		Description: "List ClusterOperators with available/degraded/progressing status.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolOperators(root)
		},
	}
}

func buildInsightsMemory(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_memory",
		Description: "Parse container memory metrics, detect high memory usage pods, OOM-kill candidates.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"threshold": map[string]any{
					"type":        "integer",
					"description": "Percentage threshold for high memory (default 80)",
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Threshold int `json:"threshold"`
			}
			json.Unmarshal(args, &input)
			if input.Threshold == 0 {
				input.Threshold = 80
			}
			return toolMemory(root, input.Threshold)
		},
	}
}

func buildInsightsEtcd(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_etcd",
		Description: "Parse etcd-specific data: operator status, member health, known issues.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolEtcd(root)
		},
	}
}

func buildInsightsStorage(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_storage",
		Description: "Storage analysis: PV status, capacity, claims, storage class distribution.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolStorage(root)
		},
	}
}

func buildInsightsAlerts(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_alerts",
		Description: "Parse active alerts from the archive, group by severity.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"severity": map[string]any{
					"type":        "string",
					"description": "Filter by severity (critical, warning, info)",
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Severity string `json:"severity"`
			}
			json.Unmarshal(args, &input)
			return toolAlerts(root, input.Severity)
		},
	}
}

func buildInsightsUIDOverlap(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_uid_overlap",
		Description: "Detect UID range conflicts across namespaces from namespace annotations.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolUIDOverlap(root)
		},
	}
}

func buildInsightsHealth(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "insights_health",
		Description: "Overall Insights archive health summary aggregating findings from other tools.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolHealth(root)
		},
	}
}
