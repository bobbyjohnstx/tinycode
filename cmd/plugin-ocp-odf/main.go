package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type state struct {
	mu      sync.RWMutex
	summary *odfSummary
}

func odfStorageClassNames(ctx context.Context, oc *redhat.OcClient) map[string]bool {
	raw, err := oc.Raw(ctx, "get", "storageclasses", "-o", "json")
	if err != nil {
		return nil
	}
	classes, err := parseStorageClasses(raw)
	if err != nil {
		return nil
	}
	names := make(map[string]bool, len(classes))
	for _, c := range classes {
		names[c.Name] = true
	}
	return names
}

func buildTools(oc *redhat.OcClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "odf_status",
			Description: "Get ODF StorageCluster status including phase and version.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: openshift-storage)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := plugin.UnmarshalToolArgs(args, &input); err != nil {
					return "", err
				}
				ns := input.Namespace
				if ns == "" {
					ns = "openshift-storage"
				}
				raw, err := oc.Raw(ctx, "get", "storageclusters.ocs.openshift.io", "-n", ns, "-o", "json")
				if err != nil {
					return fmt.Sprintf("Error getting StorageClusters: %v", err), nil
				}
				clusters, err := parseStorageClusters(raw)
				if err != nil {
					return fmt.Sprintf("Error parsing StorageClusters: %v", err), nil
				}
				if len(clusters) == 0 {
					return "No StorageClusters found. ODF may not be installed.", nil
				}
				out, _ := json.MarshalIndent(clusters, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "odf_ceph_status",
			Description: "Get Ceph cluster health, capacity, OSD counts, and mon quorum status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: openshift-storage)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := plugin.UnmarshalToolArgs(args, &input); err != nil {
					return "", err
				}
				ns := input.Namespace
				if ns == "" {
					ns = "openshift-storage"
				}
				raw, err := oc.Raw(ctx, "get", "cephclusters.ceph.rook.io", "-n", ns, "-o", "json")
				if err != nil {
					return fmt.Sprintf("Error getting CephClusters: %v", err), nil
				}
				clusters, err := parseCephClusters(raw)
				if err != nil {
					return fmt.Sprintf("Error parsing CephClusters: %v", err), nil
				}
				if len(clusters) == 0 {
					return "No CephClusters found.", nil
				}

				// Try to get live ceph status from the tools pod
				for i := range clusters {
					liveStatus, err := oc.Raw(ctx,
						"-n", ns, "exec", "deploy/rook-ceph-tools", "--",
						"ceph", "status", "-f", "json-pretty",
					)
					if err != nil {
						continue
					}
					var live struct {
						Health struct {
							Status string `json:"status"`
							Checks map[string]struct {
								Summary struct {
									Message string `json:"message"`
								} `json:"summary"`
							} `json:"checks,omitempty"`
						} `json:"health"`
						OSDMap struct {
							NumOSDs   int `json:"num_osds"`
							NumUpOSDs int `json:"num_up_osds"`
							NumInOSDs int `json:"num_in_osds"`
						} `json:"osdmap"`
						MonMap struct {
							NumMons int `json:"num_mons"`
						} `json:"monmap"`
						PGMap struct {
							BytesTotal json.Number `json:"bytes_total"`
							BytesUsed  json.Number `json:"bytes_used"`
							BytesAvail json.Number `json:"bytes_avail"`
						} `json:"pgmap"`
					}
					if json.Unmarshal([]byte(liveStatus), &live) == nil {
						clusters[i].Health = live.Health.Status
						var msgs []string
						for _, check := range live.Health.Checks {
							if check.Summary.Message != "" {
								msgs = append(msgs, check.Summary.Message)
							}
						}
						if len(msgs) > 0 {
							clusters[i].Message = strings.Join(msgs, "; ")
						}
						clusters[i].OSDStatus = &osdStatusInfo{
							Total: live.OSDMap.NumOSDs,
							Up:    live.OSDMap.NumUpOSDs,
							In:    live.OSDMap.NumInOSDs,
						}
						clusters[i].MonCount = live.MonMap.NumMons
						clusters[i].Capacity = &cephCapacity{
							Total:     formatBytes(live.PGMap.BytesTotal),
							Used:      formatBytes(live.PGMap.BytesUsed),
							Available: formatBytes(live.PGMap.BytesAvail),
						}
					}
				}

				out, _ := json.MarshalIndent(clusters, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "odf_pools",
			Description: "List CephBlockPools and CephFilesystems with replication/erasure coding config and status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: openshift-storage)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := plugin.UnmarshalToolArgs(args, &input); err != nil {
					return "", err
				}
				ns := input.Namespace
				if ns == "" {
					ns = "openshift-storage"
				}

				var allPools []poolInfo

				if raw, err := oc.Raw(ctx, "get", "cephblockpools.ceph.rook.io", "-n", ns, "-o", "json"); err == nil {
					if pools, err := parseBlockPools(raw); err == nil {
						allPools = append(allPools, pools...)
					}
				}

				if raw, err := oc.Raw(ctx, "get", "cephfilesystems.ceph.rook.io", "-n", ns, "-o", "json"); err == nil {
					if pools, err := parseFilesystems(raw); err == nil {
						allPools = append(allPools, pools...)
					}
				}

				if len(allPools) == 0 {
					return "No CephBlockPools or CephFilesystems found.", nil
				}
				out, _ := json.MarshalIndent(allPools, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "odf_pvcs",
			Description: "List PersistentVolumeClaims backed by ODF storage classes, with capacity and status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace to list PVCs in, or 'all' for all namespaces (default: all)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := plugin.UnmarshalToolArgs(args, &input); err != nil {
					return "", err
				}

				odfClasses := odfStorageClassNames(ctx, oc)
				if len(odfClasses) == 0 {
					return "No ODF storage classes found. ODF may not be installed.", nil
				}

				var raw string
				var err error
				ns := input.Namespace
				if ns == "" || ns == "all" {
					raw, err = oc.Raw(ctx, "get", "pvc", "-A", "-o", "json")
				} else {
					raw, err = oc.Raw(ctx, "get", "pvc", "-n", ns, "-o", "json")
				}
				if err != nil {
					return fmt.Sprintf("Error listing PVCs: %v", err), nil
				}
				pvcs, err := parsePVCs(raw, odfClasses)
				if err != nil {
					return fmt.Sprintf("Error parsing PVCs: %v", err), nil
				}
				if len(pvcs) == 0 {
					return "No PVCs found using ODF storage classes.", nil
				}
				out, _ := json.MarshalIndent(pvcs, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "odf_buckets",
			Description: "List ObjectBucketClaims and their backing store status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace, or 'all' for all namespaces (default: all)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := plugin.UnmarshalToolArgs(args, &input); err != nil {
					return "", err
				}

				var raw string
				var err error
				ns := input.Namespace
				if ns == "" || ns == "all" {
					raw, err = oc.Raw(ctx, "get", "objectbucketclaims", "-A", "-o", "json")
				} else {
					raw, err = oc.Raw(ctx, "get", "objectbucketclaims", "-n", ns, "-o", "json")
				}
				if err != nil {
					return fmt.Sprintf("Error listing ObjectBucketClaims: %v", err), nil
				}
				buckets, err := parseBuckets(raw)
				if err != nil {
					return fmt.Sprintf("Error parsing ObjectBucketClaims: %v", err), nil
				}
				if len(buckets) == 0 {
					return "No ObjectBucketClaims found.", nil
				}
				out, _ := json.MarshalIndent(buckets, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "odf_storage_classes",
			Description: "List ODF-managed StorageClasses (Ceph RBD, CephFS, NooBaa) with provisioner and reclaim policy.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				raw, err := oc.Raw(ctx, "get", "storageclasses", "-o", "json")
				if err != nil {
					return fmt.Sprintf("Error listing StorageClasses: %v", err), nil
				}
				classes, err := parseStorageClasses(raw)
				if err != nil {
					return fmt.Sprintf("Error parsing StorageClasses: %v", err), nil
				}
				if len(classes) == 0 {
					return "No ODF storage classes found.", nil
				}
				out, _ := json.MarshalIndent(classes, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "odf_node_resources",
			Description: "Show node CPU, memory, and ephemeral-storage capacity vs allocatable for sizing and eviction diagnostics.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				data, err := oc.Get(ctx, "nodes", nil)
				if err != nil {
					return fmt.Sprintf("Error listing nodes: %v", err), nil
				}
				nodes, err := parseNodeResources(data)
				if err != nil {
					return fmt.Sprintf("Error parsing nodes: %v", err), nil
				}
				if len(nodes) == 0 {
					return "No nodes found.", nil
				}
				out, _ := json.MarshalIndent(nodes, "", "  ")
				return string(out), nil
			},
		},
	}
}

func gatherODFSummary(ctx context.Context, oc *redhat.OcClient) *odfSummary {
	if !oc.IsAvailable(ctx) || !oc.IsLoggedIn(ctx) {
		return nil
	}

	summary := &odfSummary{}

	if raw, err := oc.Raw(ctx, "get", "storageclusters.ocs.openshift.io", "-n", "openshift-storage", "-o", "json"); err == nil {
		if clusters, err := parseStorageClusters(raw); err == nil && len(clusters) > 0 {
			summary.Phase = clusters[0].Phase
		}
	}

	if raw, err := oc.Raw(ctx, "get", "cephclusters.ceph.rook.io", "-n", "openshift-storage", "-o", "json"); err == nil {
		if clusters, err := parseCephClusters(raw); err == nil && len(clusters) > 0 {
			summary.CephHealth = clusters[0].Health
			if clusters[0].Capacity != nil {
				summary.Capacity = fmt.Sprintf("%s used / %s total", clusters[0].Capacity.Used, clusters[0].Capacity.Total)
			}
		}
	}

	if summary.Phase == "" && summary.CephHealth == "" {
		return nil
	}
	return summary
}

func buildHealthTool(oc *redhat.OcClient) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "odf_health",
		Description: "Check OpenShift Data Foundation service health: oc connectivity, ODF operator, Ceph cluster status.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			var lines []string
			lines = append(lines, "Service Health:", "")

			if !oc.IsAvailable(ctx) {
				lines = append(lines, "[DOWN] oc CLI — not found in PATH")
				return strings.Join(lines, "\n"), nil
			}
			if !oc.IsLoggedIn(ctx) {
				lines = append(lines, "[DOWN] OpenShift cluster — not logged in")
				return strings.Join(lines, "\n"), nil
			}
			lines = append(lines, "[OK] OpenShift cluster")

			_, err := oc.Raw(ctx, "get", "crd", "storageclusters.ocs.openshift.io", "-o", "name")
			if err != nil {
				lines = append(lines, "[DOWN] ODF — CRD not found (OpenShift Data Foundation may not be installed)")
			} else {
				lines = append(lines, "[OK] ODF CRD")
			}

			scRaw, err := oc.Raw(ctx, "get", "storageclusters.ocs.openshift.io", "-n", "openshift-storage", "-o", "json")
			if err != nil {
				lines = append(lines, "[SKIP] StorageCluster — unable to query")
			} else {
				clusters, parseErr := parseStorageClusters(scRaw)
				if parseErr != nil || len(clusters) == 0 {
					lines = append(lines, "[DOWN] StorageCluster — not found")
				} else {
					phase := clusters[0].Phase
					if phase == "Ready" {
						lines = append(lines, fmt.Sprintf("[OK] StorageCluster (%s)", phase))
					} else {
						lines = append(lines, fmt.Sprintf("[DOWN] StorageCluster (%s)", phase))
					}
				}
			}

			cephRaw, err := oc.Raw(ctx, "get", "cephclusters.ceph.rook.io", "-n", "openshift-storage", "-o", "json")
			if err != nil {
				lines = append(lines, "[SKIP] CephCluster — unable to query")
			} else {
				cephs, parseErr := parseCephClusters(cephRaw)
				if parseErr != nil || len(cephs) == 0 {
					lines = append(lines, "[DOWN] CephCluster — not found")
				} else {
					health := cephs[0].Health
					if health == "HEALTH_OK" {
						lines = append(lines, fmt.Sprintf("[OK] CephCluster (%s)", health))
					} else {
						lines = append(lines, fmt.Sprintf("[DOWN] CephCluster (%s)", health))
					}
				}
			}

			return strings.Join(lines, "\n"), nil
		},
	}
}

func newPlugin() plugin.Plugin {
	oc := redhat.NewOcClient()
	s := &state{}

	tools := buildTools(oc)
	tools = append(tools, buildHealthTool(oc))

	return plugin.Plugin{
		ID:    "ocp-odf",
		Tools: tools,
		Hooks: plugin.HookHandlers{
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) (*plugin.SessionStartOutput, error) {
				slog.Info("ocp-odf: gathering storage summary", "sessionId", event.SessionID)
				summary := gatherODFSummary(ctx, oc)
				s.mu.Lock()
				s.summary = summary
				s.mu.Unlock()
				return nil, nil
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}
