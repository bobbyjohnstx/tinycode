package mustgather

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Well-known subdirectories in a must-gather archive.
var wellKnownDirs = []string{
	"cluster-scoped-resources",
	"namespaces",
}

// OptionalDirs are commonly present but not required.
var optionalDirs = []string{
	"host_service_logs",
	"monitoring",
	"audit_logs",
}

// Root represents a must-gather directory and provides access to its
// subdirectories, resources, and log files.
type Root struct {
	Path       string
	Present    []string
	Missing    []string
	Namespaces []string
}

// Use validates and opens a must-gather directory at the given path. It
// discovers which well-known subdirectories are present and enumerates
// available namespaces.
func Use(path string) (*Root, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("must-gather path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}

	// Auto-detect nested must-gather directories (e.g., must-gather.local.XXX/quay.io/.../must-gather/)
	resolved := resolveRoot(abs)

	r := &Root{Path: resolved}

	for _, d := range append(wellKnownDirs, optionalDirs...) {
		full := filepath.Join(resolved, d)
		if fi, err := os.Stat(full); err == nil && fi.IsDir() {
			r.Present = append(r.Present, d)
		} else {
			r.Missing = append(r.Missing, d)
		}
	}

	hasRequired := false
	for _, d := range wellKnownDirs {
		for _, p := range r.Present {
			if p == d {
				hasRequired = true
				break
			}
		}
	}
	if !hasRequired {
		return nil, fmt.Errorf("not a valid must-gather directory: missing both %s", strings.Join(wellKnownDirs, " and "))
	}

	r.Namespaces = discoverNamespaces(resolved)
	return r, nil
}

// resolveRoot handles the common case where the user points at a top-level
// must-gather.local.* directory which contains a single image-specific
// subdirectory that holds the actual resources. Walks up to 4 levels deep
// (e.g., must-gather.local.XXX/quay.io/org/image/cluster-scoped-resources).
func resolveRoot(path string) string {
	if hasMustGatherMarker(path) {
		return path
	}
	if found := walkForRoot(path, 4); found != "" {
		return found
	}
	return path
}

func hasMustGatherMarker(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && (e.Name() == "cluster-scoped-resources" || e.Name() == "namespaces") {
			return true
		}
	}
	return false
}

func walkForRoot(dir string, depth int) string {
	if depth <= 0 {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if hasMustGatherMarker(child) {
			return child
		}
		if found := walkForRoot(child, depth-1); found != "" {
			return found
		}
	}
	return ""
}

func discoverNamespaces(root string) []string {
	nsDir := filepath.Join(root, "namespaces")
	entries, err := os.ReadDir(nsDir)
	if err != nil {
		return nil
	}
	var ns []string
	for _, e := range entries {
		if e.IsDir() {
			ns = append(ns, e.Name())
		}
	}
	return ns
}

// ClusterScopedPath returns the filesystem path for a cluster-scoped resource
// type, e.g., "config.openshift.io/clusterversions".
func (r *Root) ClusterScopedPath(apiGroup, resourceType string) string {
	return filepath.Join(r.Path, "cluster-scoped-resources", apiGroup, resourceType)
}

// NamespacedPath returns the filesystem path for a namespaced resource dir.
func (r *Root) NamespacedPath(namespace string) string {
	return filepath.Join(r.Path, "namespaces", namespace)
}

// HostServiceLogsPath returns the path to host service logs.
func (r *Root) HostServiceLogsPath() string {
	return filepath.Join(r.Path, "host_service_logs")
}

// AuditLogsPath returns the path to audit logs.
func (r *Root) AuditLogsPath() string {
	return filepath.Join(r.Path, "audit_logs")
}

// MonitoringPath returns the path to monitoring data.
func (r *Root) MonitoringPath() string {
	return filepath.Join(r.Path, "monitoring")
}

// Summary returns a text summary of the must-gather directory structure.
func (r *Root) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Must-gather root: %s\n", r.Path)
	fmt.Fprintf(&b, "Present directories: %s\n", strings.Join(r.Present, ", "))
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "Missing directories: %s\n", strings.Join(r.Missing, ", "))
	}
	fmt.Fprintf(&b, "Namespaces: %d", len(r.Namespaces))
	if len(r.Namespaces) > 0 && len(r.Namespaces) <= 20 {
		fmt.Fprintf(&b, " (%s)", strings.Join(r.Namespaces, ", "))
	}
	return b.String()
}
