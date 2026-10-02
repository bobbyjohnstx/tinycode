package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bobbyjohnstx/tinycode/pkg/mustgather"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type state struct {
	mu   sync.RWMutex
	root *mustgather.Root
}

func (s *state) get() *mustgather.Root {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root
}

func (s *state) set(r *mustgather.Root) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root = r
}

func newPlugin() plugin.Plugin {
	st := &state{}
	return plugin.Plugin{
		ID:    "ocp-must-gather",
		Tools: buildTools(st),
	}
}

func main() {
	plugin.Run(newPlugin())
}

func requireRoot(st *state) (*mustgather.Root, error) {
	r := st.get()
	if r == nil {
		return nil, fmt.Errorf("no must-gather directory loaded — call mg_use first")
	}
	return r, nil
}

func buildTools(st *state) []plugin.ToolDef {
	return []plugin.ToolDef{
		buildMgUse(st),
		buildMgClusterVersion(st),
		buildMgNodes(st),
		buildMgOperators(st),
		buildMgCerts(st),
		buildMgNodeLogs(st),
		buildMgOvn(st),
		buildMgPrometheus(st),
		buildMgMachineConfig(st),
		buildMgPods(st),
		buildMgEvents(st),
		buildMgHealth(st),
	}
}

func buildMgUse(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_use",
		Description: "Set the active must-gather directory path. Validates structure and discovers available data.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path to must-gather directory or archive"},
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
			root, err := mustgather.Use(input.Path)
			if err != nil {
				return "", err
			}
			st.set(root)
			return root.Summary(), nil
		},
	}
}

func buildMgClusterVersion(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_cluster_version",
		Description: "Show cluster version, update channel, and upgrade history from must-gather.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolClusterVersion(root)
		},
	}
}

func buildMgNodes(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_nodes",
		Description: "List nodes with status, roles, conditions, and capacity from must-gather.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"role": map[string]any{"type": "string", "description": "Filter by role (master, worker, infra)"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Role string `json:"role"`
			}
			json.Unmarshal(args, &input)
			return toolNodes(root, input.Role)
		},
	}
}

func buildMgOperators(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_operators",
		Description: "List ClusterOperators with available/degraded/progressing status from must-gather.",
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

func buildMgCerts(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_certs",
		Description: "Check certificate expiry across the cluster from must-gather secrets.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expired_only": map[string]any{"type": "boolean", "description": "Show only expired or soon-to-expire certificates"},
				"days":         map[string]any{"type": "integer", "description": "Warn if certificate expires within this many days (default 30)"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				ExpiredOnly bool `json:"expired_only"`
				Days        int  `json:"days"`
			}
			json.Unmarshal(args, &input)
			if input.Days == 0 {
				input.Days = 30
			}
			return toolCerts(root, input.ExpiredOnly, input.Days)
		},
	}
}

func buildMgNodeLogs(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_node_logs",
		Description: "Search host-level journal and service logs from must-gather.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node":    map[string]any{"type": "string", "description": "Node name to search (searches all if empty)"},
				"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for"},
				"max":     map[string]any{"type": "integer", "description": "Maximum results (default 100)"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Node    string `json:"node"`
				Pattern string `json:"pattern"`
				Max     int    `json:"max"`
			}
			json.Unmarshal(args, &input)
			if input.Max == 0 {
				input.Max = 100
			}
			return toolNodeLogs(root, input.Node, input.Pattern, input.Max)
		},
	}
}

func buildMgOvn(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_ovn",
		Description: "OVN-Kubernetes network diagnostics from must-gather (EgressIP, NetworkPolicy, pod logs).",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolOvn(root)
		},
	}
}

func buildMgPrometheus(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_prometheus",
		Description: "Search Prometheus metrics data from must-gather monitoring directory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"metric": map[string]any{"type": "string", "description": "Metric name to search for"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Metric string `json:"metric"`
			}
			json.Unmarshal(args, &input)
			return toolPrometheus(root, input.Metric)
		},
	}
}

func buildMgMachineConfig(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_machine_config",
		Description: "Show MachineConfig and MachineConfigPool status from must-gather.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			return toolMachineConfig(root)
		},
	}
}

func buildMgPods(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_pods",
		Description: "List pods with status, restarts, and resource usage from must-gather.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "Filter by namespace"},
				"status":    map[string]any{"type": "string", "description": "Filter by status (Running, Failed, Pending, etc.)"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Namespace string `json:"namespace"`
				Status    string `json:"status"`
			}
			json.Unmarshal(args, &input)
			return toolPods(root, input.Namespace, input.Status)
		},
	}
}

func buildMgEvents(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_events",
		Description: "Show cluster events filtered by namespace, type, or reason from must-gather.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "Filter by namespace"},
				"type":      map[string]any{"type": "string", "description": "Filter by type (Normal, Warning)"},
				"reason":    map[string]any{"type": "string", "description": "Filter by reason pattern"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			root, err := requireRoot(st)
			if err != nil {
				return "", err
			}
			var input struct {
				Namespace string `json:"namespace"`
				Type      string `json:"type"`
				Reason    string `json:"reason"`
			}
			json.Unmarshal(args, &input)
			return toolEvents(root, input.Namespace, input.Type, input.Reason)
		},
	}
}

func buildMgHealth(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "mg_health",
		Description: "Overall must-gather health summary aggregating checks from other tools.",
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
