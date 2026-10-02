package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	bolt "go.etcd.io/bbolt"

	"github.com/bobbyjohnstx/tinycode/pkg/mustgather"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type state struct {
	mu   sync.RWMutex
	root *mustgather.Root
	db   *bolt.DB
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

func (s *state) setDB(db *bolt.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}

func (s *state) getDB() *bolt.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db
}

func (s *state) closeDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func newPlugin() plugin.Plugin {
	st := &state{}
	return plugin.Plugin{
		ID:    "etcd-diag",
		Tools: buildTools(st),
		Hooks: plugin.HookHandlers{
			Dispose: func(ctx context.Context) error {
				return st.closeDB()
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}

func requireRoot(st *state) (*mustgather.Root, error) {
	r := st.get()
	if r == nil {
		return nil, fmt.Errorf("no must-gather directory loaded — call etcd_diag_stats with a path first")
	}
	return r, nil
}

func requireDB(st *state) (*bolt.DB, error) {
	db := st.getDB()
	if db == nil {
		return nil, fmt.Errorf("no snapshot opened — call etcd_snapshot_open with a path first")
	}
	return db, nil
}

func buildTools(st *state) []plugin.ToolDef {
	return []plugin.ToolDef{
		buildEtcdDiagStats(st),
		buildEtcdDiagErrors(st),
		buildEtcdDiagTimeline(st),
		buildEtcdDiagCompare(st),
		buildEtcdDiagLive(),
		buildEtcdDiagHealth(st),
		buildSnapshotOpen(st),
		buildSnapshotResources(st),
		buildSnapshotGet(st),
		buildSnapshotSearch(st),
		buildSnapshotStorage(st),
	}
}

func buildEtcdDiagStats(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_diag_stats",
		Description: "Parse etcd pod logs from must-gather and extract statistics: slow write counts, slow fsync counts, compaction durations. Report max/min/median/average for duration values.",
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
			root, err := mustgather.Use(input.Path)
			if err != nil {
				return "", err
			}
			st.set(root)
			return toolStats(root)
		},
	}
}

func buildEtcdDiagErrors(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_diag_errors",
		Description: "Extract and categorize error messages from etcd logs. Group by error type (auth failures, storage errors, raft errors, network errors).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path to must-gather directory"},
				"max":  map[string]any{"type": "integer", "description": "Maximum error entries to return (default 50)"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path string `json:"path"`
				Max  int    `json:"max"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Max == 0 {
				input.Max = 50
			}
			root, err := mustgather.Use(input.Path)
			if err != nil {
				return "", err
			}
			st.set(root)
			return toolErrors(root, input.Max)
		},
	}
}

func buildEtcdDiagTimeline(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_diag_timeline",
		Description: "Build a timeline of significant etcd events: leader elections, member changes, compactions, defragmentations. Sort chronologically.",
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
			root, err := mustgather.Use(input.Path)
			if err != nil {
				return "", err
			}
			st.set(root)
			return toolTimeline(root)
		},
	}
}

func buildEtcdDiagCompare(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_diag_compare",
		Description: "Cross-pod correlation: compare metrics across all etcd pods to identify node-specific vs cluster-wide issues. Show per-pod slow apply counts, fsync counts side by side.",
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
			root, err := mustgather.Use(input.Path)
			if err != nil {
				return "", err
			}
			st.set(root)
			return toolCompare(root)
		},
	}
}

func buildEtcdDiagLive() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_diag_live",
		Description: "(Stub) Placeholder for live Prometheus etcd metrics.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			return "Live mode is not yet implemented. Use offline mode with a must-gather directory instead:\n\n  etcd_diag_stats(path: \"/path/to/must-gather\")\n  etcd_diag_health(path: \"/path/to/must-gather\")", nil
		},
	}
}

func buildEtcdDiagHealth(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_diag_health",
		Description: "Overall etcd health check combining stats from other tools. Report [OK]/[WARN]/[CRITICAL] for each dimension.",
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
			root, err := mustgather.Use(input.Path)
			if err != nil {
				return "", err
			}
			st.set(root)
			return toolHealth(root)
		},
	}
}
