package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture creates a JSON file in the fixture directory.
func writeFixture(t *testing.T, root, path string, data any) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupFixture creates a minimal Insights archive directory with test data.
func setupFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// clusterversion.json
	writeFixture(t, root, "config/clusterversion.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{"name": "version"},
				"spec": map[string]any{
					"channel":   "stable-4.14",
					"clusterID": "test-cluster-123",
				},
				"status": map[string]any{
					"desired": map[string]any{"version": "4.14.5"},
					"conditions": []any{
						map[string]any{"type": "Available", "status": "True"},
						map[string]any{"type": "Progressing", "status": "False"},
					},
				},
			},
		},
	})

	// infrastructure.json
	writeFixture(t, root, "config/infrastructure.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{"name": "cluster"},
				"status": map[string]any{
					"platform":           "AWS",
					"infrastructureName": "test-infra-abc",
				},
			},
		},
	})

	// node.json
	writeFixture(t, root, "config/node.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{
					"name": "master-0",
					"labels": map[string]any{
						"node-role.kubernetes.io/master":        "",
						"node-role.kubernetes.io/control-plane": "",
					},
				},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Ready", "status": "True"},
					},
					"capacity": map[string]any{"cpu": "8", "memory": "32Gi"},
					"nodeInfo": map[string]any{"kubeletVersion": "v1.27.8"},
				},
			},
			map[string]any{
				"metadata": map[string]any{
					"name": "worker-0",
					"labels": map[string]any{
						"node-role.kubernetes.io/worker": "",
					},
				},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Ready", "status": "True"},
					},
					"capacity": map[string]any{"cpu": "16", "memory": "64Gi"},
					"nodeInfo": map[string]any{"kubeletVersion": "v1.27.8"},
				},
			},
			map[string]any{
				"metadata": map[string]any{
					"name": "worker-1",
					"labels": map[string]any{
						"node-role.kubernetes.io/worker": "",
					},
				},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Ready", "status": "False"},
						map[string]any{"type": "DiskPressure", "status": "True"},
					},
					"capacity": map[string]any{"cpu": "16", "memory": "64Gi"},
					"nodeInfo": map[string]any{"kubeletVersion": "v1.27.8"},
				},
			},
		},
	})

	// clusteroperator.json
	writeFixture(t, root, "config/clusteroperator.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{"name": "authentication"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Available", "status": "True"},
						map[string]any{"type": "Degraded", "status": "False"},
						map[string]any{"type": "Progressing", "status": "False"},
					},
					"versions": []any{
						map[string]any{"name": "operator", "version": "4.14.5"},
					},
				},
			},
			map[string]any{
				"metadata": map[string]any{"name": "etcd"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Available", "status": "True"},
						map[string]any{"type": "Degraded", "status": "True", "message": "etcd cluster unhealthy"},
						map[string]any{"type": "Progressing", "status": "False"},
					},
					"versions": []any{
						map[string]any{"name": "operator", "version": "4.14.5"},
					},
				},
			},
			map[string]any{
				"metadata": map[string]any{"name": "console"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Available", "status": "True"},
						map[string]any{"type": "Degraded", "status": "False"},
						map[string]any{"type": "Progressing", "status": "True"},
					},
					"versions": []any{
						map[string]any{"name": "operator", "version": "4.14.5"},
					},
				},
			},
		},
	})

	// storage
	writeFixture(t, root, "config/storage/storageclasses.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{
					"name": "gp3-csi",
					"annotations": map[string]any{
						"storageclass.kubernetes.io/is-default-class": "true",
					},
				},
				"provisioner":       "ebs.csi.aws.com",
				"reclaimPolicy":     "Delete",
				"volumeBindingMode": "WaitForFirstConsumer",
			},
			map[string]any{
				"metadata":          map[string]any{"name": "gp2"},
				"provisioner":       "kubernetes.io/aws-ebs",
				"reclaimPolicy":     "Delete",
				"volumeBindingMode": "WaitForFirstConsumer",
			},
		},
	})

	// containers
	writeFixture(t, root, "config/pod/containers.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{
					"name":      "web-app-1",
					"namespace": "production",
				},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name": "web",
							"resources": map[string]any{
								"requests": map[string]any{"memory": "450Mi"},
								"limits":   map[string]any{"memory": "512Mi"},
							},
						},
					},
				},
				"status": map[string]any{
					"containerStatuses": []any{
						map[string]any{
							"name": "web",
							"lastState": map[string]any{
								"terminated": map[string]any{"reason": "OOMKilled"},
							},
						},
					},
				},
			},
			map[string]any{
				"metadata": map[string]any{
					"name":      "api-server",
					"namespace": "production",
				},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name": "api",
							"resources": map[string]any{
								"requests": map[string]any{"memory": "128Mi"},
								"limits":   map[string]any{"memory": "1Gi"},
							},
						},
					},
				},
			},
		},
	})

	// alerts
	writeFixture(t, root, "monitoring/alerts/alerts.json", map[string]any{
		"data": map[string]any{
			"alerts": []any{
				map[string]any{
					"labels": map[string]any{
						"alertname": "KubePodCrashLooping",
						"severity":  "critical",
					},
					"state": "firing",
					"annotations": map[string]any{
						"message": "Pod production/web-app-1 is crash looping",
					},
				},
				map[string]any{
					"labels": map[string]any{
						"alertname": "NodeDiskRunningFull",
						"severity":  "warning",
					},
					"state": "firing",
					"annotations": map[string]any{
						"message": "Node worker-1 disk is filling up",
					},
				},
				map[string]any{
					"labels": map[string]any{
						"alertname": "ClusterVersionSucceeding",
						"severity":  "info",
					},
					"state": "firing",
					"annotations": map[string]any{
						"message": "Cluster version is succeeding",
					},
				},
			},
		},
	})

	// namespaces with UID ranges
	writeFixture(t, root, "config/namespace.json", map[string]any{
		"items": []any{
			map[string]any{
				"metadata": map[string]any{
					"name": "project-a",
					"annotations": map[string]any{
						"openshift.io/sa.scc.uid-range": "1000000/10000",
					},
				},
			},
			map[string]any{
				"metadata": map[string]any{
					"name": "project-b",
					"annotations": map[string]any{
						"openshift.io/sa.scc.uid-range": "1005000/10000",
					},
				},
			},
			map[string]any{
				"metadata": map[string]any{
					"name": "project-c",
					"annotations": map[string]any{
						"openshift.io/sa.scc.uid-range": "1020000/10000",
					},
				},
			},
		},
	})

	return root
}

func TestToolSummary(t *testing.T) {
	root := setupFixture(t)
	out, err := toolSummary(root)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"4.14.5",
		"stable-4.14",
		"AWS",
		"Nodes: 3",
		"masters=1",
		"workers=2",
		"2 healthy",
		"1 degraded",
		"1 progressing",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestToolNodes(t *testing.T) {
	root := setupFixture(t)
	out, err := toolNodes(root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Nodes: 3") {
		t.Errorf("expected 3 nodes\noutput:\n%s", out)
	}
	if !strings.Contains(out, "master-0") {
		t.Errorf("missing master-0\noutput:\n%s", out)
	}
	if !strings.Contains(out, "worker-1") {
		t.Errorf("missing worker-1\noutput:\n%s", out)
	}
	if !strings.Contains(out, "NotReady") {
		t.Errorf("missing NotReady for worker-1\noutput:\n%s", out)
	}
	if !strings.Contains(out, "DiskPressure") {
		t.Errorf("missing DiskPressure issue\noutput:\n%s", out)
	}
}

func TestToolOperators(t *testing.T) {
	root := setupFixture(t)
	out, err := toolOperators(root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "ClusterOperators: 3") {
		t.Errorf("expected 3 operators\noutput:\n%s", out)
	}
	if !strings.Contains(out, "2 healthy") {
		t.Errorf("expected 2 healthy\noutput:\n%s", out)
	}
	if !strings.Contains(out, "1 degraded") {
		t.Errorf("expected 1 degraded\noutput:\n%s", out)
	}
	if !strings.Contains(out, "etcd") {
		t.Errorf("missing etcd operator\noutput:\n%s", out)
	}
}

func TestToolAlerts(t *testing.T) {
	root := setupFixture(t)

	t.Run("all", func(t *testing.T) {
		out, err := toolAlerts(root, "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Alerts: 3 total") {
			t.Errorf("expected 3 alerts\noutput:\n%s", out)
		}
		if !strings.Contains(out, "KubePodCrashLooping") {
			t.Errorf("missing critical alert\noutput:\n%s", out)
		}
		if !strings.Contains(out, "[CRITICAL]") {
			t.Errorf("missing CRITICAL section\noutput:\n%s", out)
		}
		if !strings.Contains(out, "[WARNING]") {
			t.Errorf("missing WARNING section\noutput:\n%s", out)
		}
	})

	t.Run("filter_critical", func(t *testing.T) {
		out, err := toolAlerts(root, "critical")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "KubePodCrashLooping") {
			t.Errorf("missing critical alert\noutput:\n%s", out)
		}
		if strings.Contains(out, "NodeDiskRunningFull") {
			t.Errorf("should not include warning alert\noutput:\n%s", out)
		}
	})
}

func TestToolStorage(t *testing.T) {
	root := setupFixture(t)
	out, err := toolStorage(root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "StorageClasses: 2") {
		t.Errorf("expected 2 storage classes\noutput:\n%s", out)
	}
	if !strings.Contains(out, "gp3-csi") {
		t.Errorf("missing gp3-csi\noutput:\n%s", out)
	}
	if !strings.Contains(out, "(default)") {
		t.Errorf("missing default annotation\noutput:\n%s", out)
	}
}

func TestToolMemory(t *testing.T) {
	root := setupFixture(t)
	out, err := toolMemory(root, 80)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "OOM-Killed") {
		t.Errorf("missing OOM-Killed section\noutput:\n%s", out)
	}
	if !strings.Contains(out, "web") {
		t.Errorf("missing OOM-killed container\noutput:\n%s", out)
	}
	// 450/512 = 87%, should be flagged at 80% threshold
	if !strings.Contains(out, "High Memory") {
		t.Errorf("missing high memory section\noutput:\n%s", out)
	}
}

func TestToolUIDOverlap(t *testing.T) {
	root := setupFixture(t)
	out, err := toolUIDOverlap(root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Namespaces with UID ranges: 3") {
		t.Errorf("expected 3 namespaces\noutput:\n%s", out)
	}
	// project-a [1000000-1009999] overlaps with project-b [1005000-1014999]
	if !strings.Contains(out, "OVERLAPS DETECTED") {
		t.Errorf("expected overlap detection\noutput:\n%s", out)
	}
	if !strings.Contains(out, "project-a") || !strings.Contains(out, "project-b") {
		t.Errorf("expected project-a/project-b overlap\noutput:\n%s", out)
	}
	// project-c should not overlap
	if strings.Contains(out, "project-c") {
		t.Errorf("project-c should not overlap\noutput:\n%s", out)
	}
}

func TestToolEtcd(t *testing.T) {
	root := setupFixture(t)
	out, err := toolEtcd(root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "etcd Operator Status") {
		t.Errorf("missing etcd operator status\noutput:\n%s", out)
	}
	if !strings.Contains(out, "Degraded=True") {
		t.Errorf("missing degraded status\noutput:\n%s", out)
	}
}

func TestToolHealth(t *testing.T) {
	root := setupFixture(t)
	out, err := toolHealth(root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "4.14.5") {
		t.Errorf("missing version\noutput:\n%s", out)
	}
	if !strings.Contains(out, "Issues found") {
		t.Errorf("expected issues found\noutput:\n%s", out)
	}
	if !strings.Contains(out, "Node not ready") {
		t.Errorf("expected not ready node issue\noutput:\n%s", out)
	}
	if !strings.Contains(out, "Operator degraded: etcd") {
		t.Errorf("expected etcd degraded issue\noutput:\n%s", out)
	}
}

func TestRequireRootEmpty(t *testing.T) {
	st := &state{}
	_, err := requireRoot(st)
	if err == nil {
		t.Fatal("expected error when no root set")
	}
	if !strings.Contains(err.Error(), "insights_use") {
		t.Errorf("error should mention insights_use: %v", err)
	}
}

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"512Mi", 512 * (1 << 20)},
		{"1Gi", 1 << 30},
		{"256Ki", 256 * (1 << 10)},
		{"1073741824", 1073741824},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseMemoryBytes(tt.input)
		if got != tt.want {
			t.Errorf("parseMemoryBytes(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestGetNested(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name": "test",
			"labels": map[string]any{
				"app": "web",
			},
		},
		"spec": map[string]any{
			"replicas": float64(3),
		},
	}

	if got := getNested(obj, "metadata", "name"); got != "test" {
		t.Errorf("getNested name = %q, want %q", got, "test")
	}
	if got := getNested(obj, "metadata", "labels", "app"); got != "web" {
		t.Errorf("getNested label = %q, want %q", got, "web")
	}
	if got := getNested(obj, "spec", "replicas"); got != "3" {
		t.Errorf("getNested replicas = %q, want %q", got, "3")
	}
	if got := getNested(obj, "nonexistent"); got != "" {
		t.Errorf("getNested nonexistent = %q, want empty", got)
	}
}

func TestFindInsightsRoot(t *testing.T) {
	dir := t.TempDir()
	// Create config/ inside a subdirectory (simulating archive wrapper)
	subDir := filepath.Join(dir, "insights-archive-12345")
	os.MkdirAll(filepath.Join(subDir, "config"), 0o755)

	got := findInsightsRoot(dir)
	if got != subDir {
		t.Errorf("findInsightsRoot = %q, want %q", got, subDir)
	}

	// Direct config/
	dir2 := t.TempDir()
	os.MkdirAll(filepath.Join(dir2, "config"), 0o755)
	got = findInsightsRoot(dir2)
	if got != dir2 {
		t.Errorf("findInsightsRoot direct = %q, want %q", got, dir2)
	}
}

func TestBuildTools(t *testing.T) {
	p := newPlugin()
	if p.ID != "insights" {
		t.Errorf("plugin ID = %q, want %q", p.ID, "insights")
	}
	if len(p.Tools) != 10 {
		t.Errorf("expected 10 tools, got %d", len(p.Tools))
	}

	expectedNames := []string{
		"insights_use",
		"insights_summary",
		"insights_nodes",
		"insights_operators",
		"insights_memory",
		"insights_etcd",
		"insights_storage",
		"insights_alerts",
		"insights_uid_overlap",
		"insights_health",
	}
	for i, name := range expectedNames {
		if p.Tools[i].Name != name {
			t.Errorf("tool %d name = %q, want %q", i, p.Tools[i].Name, name)
		}
	}
}
