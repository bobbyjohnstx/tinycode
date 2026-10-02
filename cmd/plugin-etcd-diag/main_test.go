package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/mustgather"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "etcd-diag" {
		t.Errorf("got %q, want %q", p.ID, "etcd-diag")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	wantNames := []string{
		"etcd_diag_stats", "etcd_diag_errors", "etcd_diag_timeline",
		"etcd_diag_compare", "etcd_diag_live", "etcd_diag_health",
		"etcd_snapshot_open", "etcd_snapshot_resources", "etcd_snapshot_get",
		"etcd_snapshot_search", "etcd_snapshot_storage",
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

// setupTestMustGather creates a must-gather directory with etcd log data.
func setupTestMustGather(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Required structure markers
	mkFile(t, dir, "cluster-scoped-resources/.keep", "")
	mkFile(t, dir, "namespaces/.keep", "")

	// etcd pod logs
	mkFile(t, dir, "namespaces/openshift-etcd/pods/etcd-master-0/etcd/etcd/logs/current.log", etcdLogMaster0)
	mkFile(t, dir, "namespaces/openshift-etcd/pods/etcd-master-1/etcd/etcd/logs/current.log", etcdLogMaster1)
	mkFile(t, dir, "namespaces/openshift-etcd/pods/etcd-master-2/etcd/etcd/logs/current.log", etcdLogMaster2)

	return dir
}

func mkFile(t *testing.T, base, rel, content string) {
	t.Helper()
	full := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output does not contain %q\noutput:\n%s", needle, truncate(haystack, 500))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func TestToolStats(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolStats(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Slow applies:")
	assertContains(t, out, "Slow fsyncs:")
	assertContains(t, out, "Compactions:")
	// master-0 has 2 slow applies, master-1 has 3 => total 5
	assertContains(t, out, "Slow applies: 5")
	// master-0 has 1, master-1 has 2, master-2 has 0 => total 3
	assertContains(t, out, "Slow fsyncs: 3")
	assertContains(t, out, "min=")
	assertContains(t, out, "max=")
}

func TestToolErrors(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolErrors(root, 50)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd Error Analysis")
	assertContains(t, out, "Network Errors:")
}

func TestToolTimeline(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolTimeline(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd Event Timeline")
	assertContains(t, out, "leader_election")
	assertContains(t, out, "compaction")
}

func TestToolCompare(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolCompare(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Cross-Pod Comparison")
	assertContains(t, out, "etcd-master-0")
	assertContains(t, out, "etcd-master-1")
	assertContains(t, out, "etcd-master-2")
	assertContains(t, out, "SLOW_APPLY")
	assertContains(t, out, "SLOW_FSYNC")
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
	assertContains(t, out, "etcd Health Report")
	assertContains(t, out, "[OK]")
	assertContains(t, out, "Slow applies:")
	assertContains(t, out, "Slow fsyncs:")
	assertContains(t, out, "Leader elections:")
	assertContains(t, out, "Overall:")
}

func TestToolLive(t *testing.T) {
	p := newPlugin()
	var liveTool *struct{ fn func() (string, error) }
	for _, tool := range p.Tools {
		if tool.Name == "etcd_diag_live" {
			exec := tool.Execute
			liveTool = &struct{ fn func() (string, error) }{
				fn: func() (string, error) { return exec(nil, nil, plugin.ToolContext{}) },
			}
			break
		}
	}
	if liveTool == nil {
		t.Fatal("etcd_diag_live tool not found")
	}
	out, err := liveTool.fn()
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "not yet implemented")
}

func TestParseDurationMs(t *testing.T) {
	tests := []struct {
		line string
		want float64
		ok   bool
	}{
		{"apply request took too long (120ms)", 120.0, true},
		{"slow fdatasync (45ms)", 45.0, true},
		{"slow fdatasync (1.5s)", 1500.0, true},
		{"compaction finished (200ms)", 200.0, true},
		{"no duration here", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseDurationMs(tt.line)
		if ok != tt.ok {
			t.Errorf("parseDurationMs(%q): ok = %v, want %v", tt.line, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("parseDurationMs(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestComputeStats(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		s := computeStats(nil)
		if s.Count != 0 {
			t.Errorf("Count = %d, want 0", s.Count)
		}
	})
	t.Run("single value", func(t *testing.T) {
		s := computeStats([]float64{42.0})
		if s.Min != 42 || s.Max != 42 || s.Avg != 42 || s.Median != 42 {
			t.Errorf("stats = %+v, want all 42", s)
		}
	})
	t.Run("multiple values", func(t *testing.T) {
		s := computeStats([]float64{10, 20, 30, 40, 50})
		if s.Min != 10 {
			t.Errorf("Min = %v, want 10", s.Min)
		}
		if s.Max != 50 {
			t.Errorf("Max = %v, want 50", s.Max)
		}
		if s.Median != 30 {
			t.Errorf("Median = %v, want 30", s.Median)
		}
		if s.Avg != 30 {
			t.Errorf("Avg = %v, want 30", s.Avg)
		}
	})
}

func TestHealthThresholds(t *testing.T) {
	dir := t.TempDir()

	mkFile(t, dir, "cluster-scoped-resources/.keep", "")
	mkFile(t, dir, "namespaces/.keep", "")

	// Create heavy slow-apply log to trigger WARN
	var lines strings.Builder
	for i := 0; i < 15; i++ {
		lines.WriteString("W0115 10:30:45.123456       1 server.go:789] apply request took too long (120ms)\n")
	}
	mkFile(t, dir, "namespaces/openshift-etcd/pods/etcd-master-0/etcd/etcd/logs/current.log", lines.String())

	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := toolHealth(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "[WARN]")
	assertContains(t, out, "Overall: [WARN]")
}

func TestNoEtcdLogs(t *testing.T) {
	dir := t.TempDir()
	mkFile(t, dir, "cluster-scoped-resources/.keep", "")
	mkFile(t, dir, "namespaces/.keep", "")

	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolStats(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "No etcd pod logs found")
}

// Fixture data

const etcdLogMaster0 = `I0115 10:30:45.123456       1 server.go:456] etcd member started
I0115 10:31:00.234567       1 raft.go:789] raft applied index 12345
W0115 10:32:15.345678       1 server.go:789] apply request took too long (120ms)
W0115 10:33:00.456789       1 server.go:789] apply request took too long (85ms)
I0115 10:33:30.567890       1 server.go:456] compaction finished successfully (200ms)
W0115 10:34:30.678901       1 backend.go:123] slow fdatasync (45ms)
I0115 10:35:00.789012       1 raft.go:123] elected leader at term 5
W0115 10:36:00.890123       1 peer.go:456] failed to send message to peer
`

const etcdLogMaster1 = `I0115 10:30:50.111111       1 server.go:456] etcd member started
W0115 10:31:15.222222       1 server.go:789] apply request took too long (150ms)
W0115 10:31:45.333333       1 server.go:789] apply request took too long (90ms)
W0115 10:32:00.444444       1 server.go:789] apply request took too long (200ms)
W0115 10:32:30.555555       1 backend.go:123] slow fdatasync (60ms)
W0115 10:33:00.666666       1 backend.go:123] slow fdatasync (35ms)
I0115 10:33:30.777777       1 server.go:456] compaction finished successfully (180ms)
I0115 10:34:00.888888       1 raft.go:123] elected leader at term 6
I0115 10:35:00.999999       1 server.go:456] defragmentation started
`

const etcdLogMaster2 = `I0115 10:30:55.111111       1 server.go:456] etcd member started
I0115 10:31:00.222222       1 raft.go:789] raft applied index 12345
I0115 10:32:00.333333       1 server.go:456] compaction finished successfully (150ms)
I0115 10:33:00.444444       1 raft.go:123] elected leader at term 5
I0115 10:34:00.555555       1 raft.go:123] lost leader at term 6
`
