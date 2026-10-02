package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/mustgather"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "ocp-must-gather" {
		t.Errorf("got %q, want %q", p.ID, "ocp-must-gather")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	wantNames := []string{
		"mg_use", "mg_cluster_version", "mg_nodes", "mg_operators",
		"mg_certs", "mg_node_logs",
		"mg_ovn", "mg_prometheus", "mg_machine_config", "mg_pods",
		"mg_events", "mg_health",
	}
	if len(p.Tools) != len(wantNames) {
		t.Fatalf("got %d tools, want %d", len(p.Tools), len(wantNames))
	}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] %q Execute is nil", i, want)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin()
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
	}
}

// setupTestMustGather creates a minimal must-gather directory structure
// for testing.
func setupTestMustGather(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// cluster-scoped-resources
	mkFile(t, dir, "cluster-scoped-resources/config.openshift.io/clusterversions/version.yaml", clusterVersionYAML)
	mkFile(t, dir, "cluster-scoped-resources/core/nodes/master-0.yaml", nodeYAML)
	mkFile(t, dir, "cluster-scoped-resources/config.openshift.io/clusteroperators/etcd.yaml", clusterOperatorYAML)
	mkFile(t, dir, "cluster-scoped-resources/machineconfiguration.openshift.io/machineconfigpools/master.yaml", machineConfigPoolYAML)

	// namespaces
	mkFile(t, dir, "namespaces/openshift-etcd/pods/etcd-master-0/etcd/etcd/logs/current.log", etcdLogData)
	mkFile(t, dir, "namespaces/openshift-etcd/core/pods.yaml", etcdPodsYAML)
	mkFile(t, dir, "namespaces/openshift-etcd/core/events.yaml", eventsYAML)
	mkFile(t, dir, "namespaces/default/core/pods.yaml", defaultPodsYAML)
	mkFile(t, dir, "namespaces/default/core/events.yaml", defaultEventsYAML)

	return dir
}

func mkFile(t *testing.T, base string, rel string, content string) {
	t.Helper()
	full := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestToolClusterVersion(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolClusterVersion(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "4.14.6")
	assertContains(t, out, "stable-4.14")
	assertContains(t, out, "Completed")
}

func TestToolNodes(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all nodes", func(t *testing.T) {
		out, err := toolNodes(root, "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "master-0")
		assertContains(t, out, "Ready")
	})

	t.Run("filter by role", func(t *testing.T) {
		out, err := toolNodes(root, "master")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "master-0")
	})

	t.Run("filter by nonexistent role", func(t *testing.T) {
		out, err := toolNodes(root, "infra")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Nodes:")
	})
}

func TestToolOperators(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolOperators(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd")
	assertContains(t, out, "True")
}

func TestToolPods(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all pods", func(t *testing.T) {
		out, err := toolPods(root, "", "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Pods:")
	})

	t.Run("filter by namespace", func(t *testing.T) {
		out, err := toolPods(root, "openshift-etcd", "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "etcd")
	})

	t.Run("filter by status", func(t *testing.T) {
		out, err := toolPods(root, "", "Failed")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "crash-pod")
	})
}

func TestToolEvents(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all events", func(t *testing.T) {
		out, err := toolEvents(root, "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Events:")
	})

	t.Run("filter by type", func(t *testing.T) {
		out, err := toolEvents(root, "", "Warning", "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Warning")
	})
}

func TestToolMachineConfig(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolMachineConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "master")
	assertContains(t, out, "3/3 ready")
}

func TestToolHealth(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolHealth(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Must-Gather Health Summary")
	assertContains(t, out, "4.14.6")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !containsStr(haystack, needle) {
		t.Errorf("output does not contain %q\noutput:\n%s", needle, truncate(haystack, 500))
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findStr(s, substr))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Test fixtures

const clusterVersionYAML = `apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: stable-4.14
  clusterID: abc-123
status:
  desired:
    version: "4.14.6"
    image: "quay.io/openshift-release-dev/ocp-release@sha256:abc123"
  history:
    - version: "4.14.6"
      state: Completed
      completionTime: "2024-01-15T10:30:00Z"
    - version: "4.14.5"
      state: Completed
      completionTime: "2024-01-01T08:00:00Z"
  conditions:
    - type: Available
      status: "True"
      message: "Done applying 4.14.6"
    - type: Progressing
      status: "False"
`

const nodeYAML = `apiVersion: v1
kind: Node
metadata:
  name: master-0
  labels:
    node-role.kubernetes.io/master: ""
    node-role.kubernetes.io/control-plane: ""
status:
  conditions:
    - type: Ready
      status: "True"
    - type: MemoryPressure
      status: "False"
    - type: DiskPressure
      status: "False"
  capacity:
    cpu: "16"
    memory: "65536Mi"
  nodeInfo:
    kubeletVersion: v1.27.8+4fab27b
`

const clusterOperatorYAML = `apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: etcd
status:
  conditions:
    - type: Available
      status: "True"
    - type: Degraded
      status: "False"
    - type: Progressing
      status: "False"
  versions:
    - name: operator
      version: "4.14.6"
`

const machineConfigPoolYAML = `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigPool
metadata:
  name: master
status:
  machineCount: 3
  readyMachineCount: 3
  updatedMachineCount: 3
  degradedMachineCount: 0
  configuration:
    name: rendered-master-abc123
  conditions:
    - type: Updated
      status: "True"
      message: "All nodes are updated"
`

const etcdLogData = `I0115 10:30:45.123456       1 server.go:456] etcd member started
I0115 10:31:00.234567       1 raft.go:789] raft applied index 12345
W0115 10:32:15.345678       1 server.go:789] apply request took too long (120ms)
I0115 10:33:00.456789       1 server.go:456] compaction finished successfully
W0115 10:34:30.567890       1 backend.go:123] slow fdatasync (45ms)
I0115 10:35:00.678901       1 raft.go:123] elected leader at term 5
`

const etcdPodsYAML = `apiVersion: v1
kind: PodList
items:
  - metadata:
      name: etcd-master-0
      namespace: openshift-etcd
    spec:
      nodeName: master-0
    status:
      phase: Running
      containerStatuses:
        - name: etcd
          restartCount: 0
`

const eventsYAML = `apiVersion: v1
kind: EventList
items:
  - metadata:
      name: etcd-master-0.event1
      namespace: openshift-etcd
    type: Warning
    reason: BackOff
    message: "Back-off restarting failed container"
    count: 3
    involvedObject:
      kind: Pod
      name: etcd-master-0
  - metadata:
      name: etcd-master-0.event2
      namespace: openshift-etcd
    type: Normal
    reason: Pulled
    message: "Successfully pulled image"
    count: 1
    involvedObject:
      kind: Pod
      name: etcd-master-0
`

const defaultPodsYAML = `apiVersion: v1
kind: PodList
items:
  - metadata:
      name: web-app-1
      namespace: default
    spec:
      nodeName: worker-0
    status:
      phase: Running
      containerStatuses:
        - name: web
          restartCount: 0
  - metadata:
      name: crash-pod
      namespace: default
    spec:
      nodeName: worker-0
    status:
      phase: Failed
      containerStatuses:
        - name: app
          restartCount: 5
`

const defaultEventsYAML = `apiVersion: v1
kind: EventList
items:
  - metadata:
      name: web-app-1.event1
      namespace: default
    type: Normal
    reason: Scheduled
    message: "Successfully assigned default/web-app-1 to worker-0"
    count: 1
    involvedObject:
      kind: Pod
      name: web-app-1
`
