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
	path string
}

func (s *state) getForPath(path string) *mustgather.Root {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.root != nil && s.path == path {
		return s.root
	}
	return nil
}

func (s *state) set(path string, r *mustgather.Root) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root = r
	s.path = path
}

func newPlugin() plugin.Plugin {
	st := &state{}
	return plugin.Plugin{
		ID:    "ingress-inspect",
		Tools: buildTools(st),
	}
}

func main() {
	plugin.Run(newPlugin())
}

func requireRoot(st *state, path string) (*mustgather.Root, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if r := st.getForPath(path); r != nil {
		return r, nil
	}
	root, err := mustgather.Use(path)
	if err != nil {
		return nil, err
	}
	st.set(path, root)
	return root, nil
}

func buildTools(st *state) []plugin.ToolDef {
	return []plugin.ToolDef{
		buildIngressControllers(st),
		buildIngressBackends(st),
		buildIngressRouteCheck(st),
		buildIngressConfig(st),
		buildIngressHealth(st),
	}
}

func buildIngressControllers(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "ingress_controllers",
		Description: "List IngressController CRs from must-gather. Shows name, domain, replicas, endpoint publishing strategy, default certificate.",
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
			root, err := requireRoot(st, input.Path)
			if err != nil {
				return "", err
			}
			return toolIngressControllers(root)
		},
	}
}

func buildIngressBackends(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "ingress_backends",
		Description: "Parse HAProxy config files from must-gather and list all backends with server counts, mode, and balance algorithm.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Path to must-gather directory"},
				"filter": map[string]any{"type": "string", "description": "Filter by backend name pattern"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path   string `json:"path"`
				Filter string `json:"filter"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			root, err := requireRoot(st, input.Path)
			if err != nil {
				return "", err
			}
			return toolIngressBackends(root, input.Filter)
		},
	}
}

func buildIngressRouteCheck(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "ingress_route_check",
		Description: "Cross-reference Route resources against HAProxy config. Detects stale backends, missing routes, and misconfigurations.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string", "description": "Path to must-gather directory"},
				"namespace": map[string]any{"type": "string", "description": "Filter Routes by namespace"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path      string `json:"path"`
				Namespace string `json:"namespace"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			root, err := requireRoot(st, input.Path)
			if err != nil {
				return "", err
			}
			return toolIngressRouteCheck(root, input.Namespace)
		},
	}
}

func buildIngressConfig(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "ingress_config",
		Description: "Show detailed HAProxy global/defaults configuration: timeouts, maxconn, SSL settings.",
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
			root, err := requireRoot(st, input.Path)
			if err != nil {
				return "", err
			}
			return toolIngressConfig(root)
		},
	}
}

func buildIngressHealth(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "ingress_health",
		Description: "Overall ingress health summary. Checks IngressController availability, HAProxy config, router pod logs, and certificate status.",
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
			root, err := requireRoot(st, input.Path)
			if err != nil {
				return "", err
			}
			return toolIngressHealth(root)
		},
	}
}
