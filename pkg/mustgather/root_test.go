package mustgather

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUse(t *testing.T) {
	t.Run("valid must-gather dir", func(t *testing.T) {
		dir := setupMG(t)
		root, err := Use(dir)
		if err != nil {
			t.Fatal(err)
		}
		if root.Path != dir {
			t.Errorf("path = %q, want %q", root.Path, dir)
		}
		if len(root.Present) == 0 {
			t.Error("expected some present dirs")
		}
		found := false
		for _, p := range root.Present {
			if p == "cluster-scoped-resources" {
				found = true
			}
		}
		if !found {
			t.Error("cluster-scoped-resources not in Present")
		}
	})

	t.Run("auto-resolves nested dir", func(t *testing.T) {
		outer := t.TempDir()
		inner := filepath.Join(outer, "quay.io", "org", "must-gather")
		mkDir(t, filepath.Join(inner, "cluster-scoped-resources"))
		mkDir(t, filepath.Join(inner, "namespaces"))

		root, err := Use(outer)
		if err != nil {
			t.Fatal(err)
		}
		if root.Path != inner {
			t.Errorf("path = %q, want %q", root.Path, inner)
		}
	})

	t.Run("invalid dir", func(t *testing.T) {
		_, err := Use("/nonexistent/path")
		if err == nil {
			t.Error("expected error for nonexistent path")
		}
	})

	t.Run("dir without required subdirs", func(t *testing.T) {
		dir := t.TempDir()
		mkDir(t, filepath.Join(dir, "random"))
		_, err := Use(dir)
		if err == nil {
			t.Error("expected error for dir without required subdirs")
		}
	})

	t.Run("discovers namespaces", func(t *testing.T) {
		dir := setupMG(t)
		root, err := Use(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(root.Namespaces) != 2 {
			t.Errorf("got %d namespaces, want 2", len(root.Namespaces))
		}
	})
}

func TestClusterScopedPath(t *testing.T) {
	dir := setupMG(t)
	root, _ := Use(dir)
	got := root.ClusterScopedPath("config.openshift.io", "clusterversions")
	want := filepath.Join(dir, "cluster-scoped-resources", "config.openshift.io", "clusterversions")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSummary(t *testing.T) {
	dir := setupMG(t)
	root, _ := Use(dir)
	summary := root.Summary()
	if summary == "" {
		t.Error("empty summary")
	}
}

func setupMG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mkDir(t, filepath.Join(dir, "cluster-scoped-resources", "config.openshift.io", "clusterversions"))
	mkDir(t, filepath.Join(dir, "cluster-scoped-resources", "core", "nodes"))
	mkDir(t, filepath.Join(dir, "namespaces", "default"))
	mkDir(t, filepath.Join(dir, "namespaces", "openshift-etcd"))
	return dir
}

func mkDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
