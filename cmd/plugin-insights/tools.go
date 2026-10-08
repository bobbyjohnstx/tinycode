package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// --- Archive helpers ---

var (
	maxArchiveBytes int64 = 1 << 30
	maxArchiveFiles       = 100_000
	maxFileBytes    int64 = 500 << 20
)

func extractTarGz(archivePath, destDir string) (int, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return 0, fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	count := 0
	var total int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("reading tar: %w", err)
		}
		count++
		if count > maxArchiveFiles {
			return count, fmt.Errorf("archive exceeds %d files", maxArchiveFiles)
		}
		target := filepath.Join(destDir, header.Name)
		if !isWithinDir(destDir, target) {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return count, fmt.Errorf("creating dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return count, fmt.Errorf("creating parent dir: %w", err)
			}
			out, err := os.Create(target)
			if err != nil {
				return count, fmt.Errorf("creating file: %w", err)
			}
			n, err := io.Copy(out, io.LimitReader(tr, maxFileBytes+1))
			out.Close()
			if err != nil {
				os.Remove(target)
				return count, fmt.Errorf("extracting file: %w", err)
			}
			if n > maxFileBytes {
				os.Remove(target)
				return count, fmt.Errorf("file %s exceeds %d byte limit", header.Name, maxFileBytes)
			}
			total += n
			if total > maxArchiveBytes {
				os.Remove(target)
				return count, fmt.Errorf("archive exceeds %d bytes", maxArchiveBytes)
			}
		}
	}
	return count, nil
}

func isWithinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// findInsightsRoot locates the directory containing config/ after extraction.
// The archive may have a top-level directory wrapper.
func findInsightsRoot(dir string) string {
	if hasConfigDir(dir) {
		return dir
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}
	for _, e := range entries {
		if e.IsDir() {
			sub := filepath.Join(dir, e.Name())
			if hasConfigDir(sub) {
				return sub
			}
		}
	}
	return dir
}

func hasConfigDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "config"))
	return err == nil && info.IsDir()
}

// --- JSON helpers ---

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	return obj, nil
}

func readItems(path string) ([]map[string]any, error) {
	obj, err := readJSONFile(path)
	if err != nil {
		return nil, err
	}
	items, ok := obj["items"]
	if !ok {
		return []map[string]any{obj}, nil
	}
	list, ok := items.([]any)
	if !ok {
		return []map[string]any{obj}, nil
	}
	var result []map[string]any
	for _, item := range list {
		m, ok := item.(map[string]any)
		if ok {
			result = append(result, m)
		}
	}
	return result, nil
}

func findFile(root string, names ...string) string {
	for _, name := range names {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func getNested(obj map[string]any, fields ...string) string {
	current := obj
	for i, f := range fields {
		val, ok := current[f]
		if !ok {
			return ""
		}
		if i == len(fields)-1 {
			switch v := val.(type) {
			case string:
				return v
			case float64:
				if v == float64(int64(v)) {
					return fmt.Sprintf("%d", int64(v))
				}
				return fmt.Sprintf("%g", v)
			case bool:
				return fmt.Sprintf("%t", v)
			default:
				return fmt.Sprintf("%v", v)
			}
		}
		next, ok := val.(map[string]any)
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func getNestedSlice(obj map[string]any, fields ...string) []any {
	current := obj
	for i, f := range fields {
		val, ok := current[f]
		if !ok {
			return nil
		}
		if i == len(fields)-1 {
			s, ok := val.([]any)
			if !ok {
				return nil
			}
			return s
		}
		next, ok := val.(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return nil
}

func getNestedMap(obj map[string]any, fields ...string) map[string]any {
	current := obj
	for _, f := range fields {
		val, ok := current[f]
		if !ok {
			return nil
		}
		next, ok := val.(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// parseMemoryBytes parses k8s memory quantity strings (e.g., "512Mi", "1Gi", "1073741824").
func parseMemoryBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"Ti", 1 << 40},
		{"Gi", 1 << 30},
		{"Mi", 1 << 20},
		{"Ki", 1 << 10},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			numStr := strings.TrimSuffix(s, sf.suffix)
			val, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0
			}
			return int64(val * float64(sf.mult))
		}
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// --- Tool implementations ---

func toolUse(archive string) (string, string, error) {
	dest, err := os.MkdirTemp("", "insights-*")
	if err != nil {
		return "", "", fmt.Errorf("creating temp dir: %w", err)
	}
	count, err := extractTarGz(archive, dest)
	if err != nil {
		os.RemoveAll(dest)
		return "", "", err
	}
	root := findInsightsRoot(dest)

	if !hasConfigDir(root) {
		os.RemoveAll(dest)
		return "", "", fmt.Errorf("invalid Insights archive: no config/ directory found")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Extracted %d files to %s\n", count, root)

	knownFiles := []string{
		"config/clusterversion.json",
		"config/node.json",
		"config/nodes.json",
		"config/clusteroperator.json",
		"config/infrastructure.json",
		"config/proxy.json",
		"config/storage/storageclasses.json",
		"config/pod/containers.json",
		"config/namespace.json",
		"config/namespaces.json",
		"monitoring/alerts/alerts.json",
	}
	fmt.Fprintf(&b, "\nDiscovered data:\n")
	for _, cf := range knownFiles {
		if _, err := os.Stat(filepath.Join(root, cf)); err == nil {
			fmt.Fprintf(&b, "  %s\n", cf)
		}
	}
	return b.String(), root, nil
}

func toolSummary(rootDir string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Insights Archive Summary\n")
	fmt.Fprintf(&b, "========================\n\n")

	// Cluster version
	if cvPath := findFile(rootDir, "config/clusterversion.json"); cvPath != "" {
		items, err := readItems(cvPath)
		if err == nil && len(items) > 0 {
			cv := items[0]
			fmt.Fprintf(&b, "Cluster Version: %s\n", getNested(cv, "status", "desired", "version"))
			fmt.Fprintf(&b, "Channel: %s\n", getNested(cv, "spec", "channel"))
			if id := getNested(cv, "spec", "clusterID"); id != "" {
				fmt.Fprintf(&b, "Cluster ID: %s\n", id)
			}
		}
	}

	// Platform
	if infraPath := findFile(rootDir, "config/infrastructure.json"); infraPath != "" {
		items, err := readItems(infraPath)
		if err == nil && len(items) > 0 {
			if p := getNested(items[0], "status", "platform"); p != "" {
				fmt.Fprintf(&b, "Platform: %s\n", p)
			}
			if n := getNested(items[0], "status", "infrastructureName"); n != "" {
				fmt.Fprintf(&b, "Infrastructure: %s\n", n)
			}
		}
	}

	// Node count
	if nodePath := findFile(rootDir, "config/node.json", "config/nodes.json"); nodePath != "" {
		items, err := readItems(nodePath)
		if err == nil {
			var masters, workers int
			for _, node := range items {
				labels := getNestedMap(node, "metadata", "labels")
				if _, ok := labels["node-role.kubernetes.io/master"]; ok {
					masters++
				}
				if _, ok := labels["node-role.kubernetes.io/worker"]; ok {
					workers++
				}
			}
			fmt.Fprintf(&b, "Nodes: %d (masters=%d, workers=%d)\n", len(items), masters, workers)
		}
	}

	// Operator health
	if opPath := findFile(rootDir, "config/clusteroperator.json", "config/clusteroperators.json"); opPath != "" {
		items, err := readItems(opPath)
		if err == nil {
			var healthy, degraded, progressing int
			for _, op := range items {
				isDegraded, isAvailable := false, false
				for _, c := range getNestedSlice(op, "status", "conditions") {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					switch getNested(cm, "type") {
					case "Degraded":
						if getNested(cm, "status") == "True" {
							isDegraded = true
						}
					case "Available":
						if getNested(cm, "status") == "True" {
							isAvailable = true
						}
					case "Progressing":
						if getNested(cm, "status") == "True" {
							progressing++
						}
					}
				}
				if isDegraded {
					degraded++
				} else if isAvailable {
					healthy++
				}
			}
			fmt.Fprintf(&b, "Operators: %d total (%d healthy, %d degraded, %d progressing)\n",
				len(items), healthy, degraded, progressing)
		}
	}

	return b.String(), nil
}

func toolNodes(rootDir string) (string, error) {
	nodePath := findFile(rootDir, "config/node.json", "config/nodes.json")
	if nodePath == "" {
		return "No node data found in Insights archive.", nil
	}
	items, err := readItems(nodePath)
	if err != nil {
		return "", fmt.Errorf("reading node data: %w", err)
	}
	if len(items) == 0 {
		return "No nodes found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Nodes: %d\n\n", len(items))

	for _, node := range items {
		name := getNested(node, "metadata", "name")
		labels := getNestedMap(node, "metadata", "labels")

		var roles []string
		for k := range labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				roles = append(roles, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
			}
		}
		sort.Strings(roles)

		ready := "Unknown"
		var issues []string
		for _, c := range getNestedSlice(node, "status", "conditions") {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			ct := getNested(cm, "type")
			cs := getNested(cm, "status")
			if ct == "Ready" {
				if cs == "True" {
					ready = "Ready"
				} else {
					ready = "NotReady"
				}
			} else if cs == "True" {
				issues = append(issues, ct)
			}
		}

		cpuCap := getNested(node, "status", "capacity", "cpu")
		memCap := getNested(node, "status", "capacity", "memory")
		kubelet := getNested(node, "status", "nodeInfo", "kubeletVersion")

		fmt.Fprintf(&b, "%-40s %s  roles=[%s]  kubelet=%s  cpu=%s  mem=%s",
			name, ready, strings.Join(roles, ","), kubelet, cpuCap, memCap)
		if len(issues) > 0 {
			fmt.Fprintf(&b, "  issues=[%s]", strings.Join(issues, ","))
		}
		fmt.Fprintln(&b)
	}
	return b.String(), nil
}

func toolOperators(rootDir string) (string, error) {
	opPath := findFile(rootDir, "config/clusteroperator.json", "config/clusteroperators.json")
	if opPath == "" {
		return "No ClusterOperator data found in Insights archive.", nil
	}
	items, err := readItems(opPath)
	if err != nil {
		return "", fmt.Errorf("reading operator data: %w", err)
	}
	if len(items) == 0 {
		return "No ClusterOperators found.", nil
	}

	var healthy, degraded, progressing int
	var b strings.Builder
	fmt.Fprintf(&b, "ClusterOperators: %d\n\n", len(items))
	fmt.Fprintf(&b, "%-45s %-10s %-10s %-12s %s\n", "NAME", "AVAILABLE", "DEGRADED", "PROGRESSING", "VERSION")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 100))

	for _, op := range items {
		name := getNested(op, "metadata", "name")
		avail, deg, prog := "Unknown", "Unknown", "Unknown"
		for _, c := range getNestedSlice(op, "status", "conditions") {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			switch getNested(cm, "type") {
			case "Available":
				avail = getNested(cm, "status")
			case "Degraded":
				deg = getNested(cm, "status")
			case "Progressing":
				prog = getNested(cm, "status")
			}
		}

		version := ""
		for _, v := range getNestedSlice(op, "status", "versions") {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if getNested(vm, "name") == "operator" {
				version = getNested(vm, "version")
				break
			}
		}

		if deg == "True" {
			degraded++
		} else if avail == "True" {
			healthy++
		}
		if prog == "True" {
			progressing++
		}
		fmt.Fprintf(&b, "%-45s %-10s %-10s %-12s %s\n", name, avail, deg, prog, version)
	}

	fmt.Fprintf(&b, "\nSummary: %d healthy, %d degraded, %d progressing\n", healthy, degraded, progressing)
	return b.String(), nil
}

func toolMemory(rootDir string, threshold int) (string, error) {
	containerPath := findFile(rootDir, "config/pod/containers.json")
	if containerPath == "" {
		return "No container data found in Insights archive (config/pod/containers.json).", nil
	}

	obj, err := readJSONFile(containerPath)
	if err != nil {
		return "", fmt.Errorf("reading container data: %w", err)
	}

	// Handle both items[] list and direct containers list
	var containers []map[string]any
	if items, ok := obj["items"]; ok {
		list, _ := items.([]any)
		for _, item := range list {
			m, ok := item.(map[string]any)
			if ok {
				containers = append(containers, m)
			}
		}
	} else if cs, ok := obj["containers"]; ok {
		list, _ := cs.([]any)
		for _, item := range list {
			m, ok := item.(map[string]any)
			if ok {
				containers = append(containers, m)
			}
		}
	}

	if len(containers) == 0 {
		return "No container entries found in containers.json.", nil
	}

	type memInfo struct {
		Namespace string
		Pod       string
		Container string
		Request   int64
		Limit     int64
		Pct       int
		OOMKilled bool
	}

	var high []memInfo
	var oomKilled []memInfo

	for _, c := range containers {
		ns := getNested(c, "metadata", "namespace")
		if ns == "" {
			ns = getNested(c, "namespace")
		}
		pod := getNested(c, "metadata", "name")
		if pod == "" {
			pod = getNested(c, "pod")
		}

		// Check nested container specs for resources
		specs := getNestedSlice(c, "spec", "containers")
		statuses := getNestedSlice(c, "status", "containerStatuses")

		// Check for OOMKilled in statuses
		for _, s := range statuses {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			cName := getNested(sm, "name")
			reason := getNested(sm, "lastState", "terminated", "reason")
			if reason == "OOMKilled" {
				oomKilled = append(oomKilled, memInfo{
					Namespace: ns, Pod: pod, Container: cName, OOMKilled: true,
				})
			}
		}

		// Check resource requests vs limits
		for _, spec := range specs {
			sm, ok := spec.(map[string]any)
			if !ok {
				continue
			}
			cName := getNested(sm, "name")
			reqMem := parseMemoryBytes(getNested(sm, "resources", "requests", "memory"))
			limMem := parseMemoryBytes(getNested(sm, "resources", "limits", "memory"))
			if limMem > 0 && reqMem > 0 {
				pct := int(reqMem * 100 / limMem)
				if pct >= threshold {
					high = append(high, memInfo{
						Namespace: ns, Pod: pod, Container: cName,
						Request: reqMem, Limit: limMem, Pct: pct,
					})
				}
			}
		}

		// Also handle flat container format (Insights-specific)
		reqMem := parseMemoryBytes(getNested(c, "resources", "requests", "memory"))
		limMem := parseMemoryBytes(getNested(c, "resources", "limits", "memory"))
		cName := getNested(c, "name")
		if cName == "" {
			cName = getNested(c, "container")
		}
		if limMem > 0 && reqMem > 0 {
			pct := int(reqMem * 100 / limMem)
			if pct >= threshold {
				high = append(high, memInfo{
					Namespace: ns, Pod: pod, Container: cName,
					Request: reqMem, Limit: limMem, Pct: pct,
				})
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Memory Analysis (threshold: %d%%)\n", threshold)
	fmt.Fprintf(&b, "Containers analyzed: %d\n\n", len(containers))

	if len(oomKilled) > 0 {
		fmt.Fprintf(&b, "OOM-Killed Containers: %d\n", len(oomKilled))
		for _, m := range oomKilled {
			fmt.Fprintf(&b, "  %s/%s container=%s\n", m.Namespace, m.Pod, m.Container)
		}
		fmt.Fprintln(&b)
	}

	if len(high) > 0 {
		sort.Slice(high, func(i, j int) bool { return high[i].Pct > high[j].Pct })
		fmt.Fprintf(&b, "High Memory (request >= %d%% of limit): %d\n", threshold, len(high))
		for _, m := range high {
			fmt.Fprintf(&b, "  %s/%s container=%s  request=%dMi limit=%dMi (%d%%)\n",
				m.Namespace, m.Pod, m.Container,
				m.Request/(1<<20), m.Limit/(1<<20), m.Pct)
		}
	} else if len(oomKilled) == 0 {
		fmt.Fprintln(&b, "No high memory usage or OOM-killed containers detected.")
	}

	return b.String(), nil
}

func toolEtcd(rootDir string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "etcd Analysis\n")
	fmt.Fprintf(&b, "=============\n\n")

	// Check etcd operator status from clusteroperators
	opPath := findFile(rootDir, "config/clusteroperator.json", "config/clusteroperators.json")
	if opPath != "" {
		items, err := readItems(opPath)
		if err == nil {
			for _, op := range items {
				name := getNested(op, "metadata", "name")
				if name != "etcd" {
					continue
				}

				fmt.Fprintf(&b, "etcd Operator Status:\n")
				for _, c := range getNestedSlice(op, "status", "conditions") {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					ct := getNested(cm, "type")
					cs := getNested(cm, "status")
					msg := getNested(cm, "message")
					fmt.Fprintf(&b, "  %s=%s", ct, cs)
					if msg != "" && len(msg) < 200 {
						fmt.Fprintf(&b, " (%s)", msg)
					}
					fmt.Fprintln(&b)
				}

				for _, v := range getNestedSlice(op, "status", "versions") {
					vm, ok := v.(map[string]any)
					if !ok {
						continue
					}
					fmt.Fprintf(&b, "  %s: %s\n", getNested(vm, "name"), getNested(vm, "version"))
				}
				fmt.Fprintln(&b)
			}
		}
	}

	// Check for etcd-specific config files
	etcdFiles := []string{
		"config/etcd/member.json",
		"config/etcd/status.json",
	}
	for _, ef := range etcdFiles {
		path := filepath.Join(rootDir, ef)
		if _, err := os.Stat(path); err == nil {
			obj, err := readJSONFile(path)
			if err == nil {
				fmt.Fprintf(&b, "%s:\n", ef)
				data, _ := json.MarshalIndent(obj, "  ", "  ")
				fmt.Fprintf(&b, "  %s\n\n", string(data))
			}
		}
	}

	if b.Len() <= len("etcd Analysis\n=============\n\n") {
		fmt.Fprintln(&b, "Limited etcd data available in this Insights archive.")
		fmt.Fprintln(&b, "For detailed etcd analysis, use a full must-gather archive with the ocp-must-gather plugin.")
	}

	return b.String(), nil
}

func toolStorage(rootDir string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Storage Analysis\n")
	fmt.Fprintf(&b, "================\n\n")

	// Storage classes
	scPath := findFile(rootDir,
		"config/storage/storageclasses.json",
		"config/storageclass.json",
		"config/storageclasses.json")
	if scPath != "" {
		items, err := readItems(scPath)
		if err == nil && len(items) > 0 {
			fmt.Fprintf(&b, "StorageClasses: %d\n", len(items))
			for _, sc := range items {
				name := getNested(sc, "metadata", "name")
				provisioner := getNested(sc, "provisioner")
				reclaim := getNested(sc, "reclaimPolicy")
				binding := getNested(sc, "volumeBindingMode")

				annotations := getNestedMap(sc, "metadata", "annotations")
				isDefault := ""
				if annotations != nil {
					if v, ok := annotations["storageclass.kubernetes.io/is-default-class"]; ok {
						if vs, ok := v.(string); ok && vs == "true" {
							isDefault = " (default)"
						}
					}
				}

				fmt.Fprintf(&b, "  %-40s provisioner=%s reclaim=%s binding=%s%s\n",
					name, provisioner, reclaim, binding, isDefault)
			}
			fmt.Fprintln(&b)
		}
	} else {
		fmt.Fprintln(&b, "No StorageClass data found.")
	}

	// Persistent volumes
	pvPath := findFile(rootDir,
		"config/persistentvolume.json",
		"config/persistentvolumes.json",
		"config/storage/persistentvolumes.json")
	if pvPath != "" {
		items, err := readItems(pvPath)
		if err == nil && len(items) > 0 {
			phases := make(map[string]int)
			var totalCap int64
			for _, pv := range items {
				phase := getNested(pv, "status", "phase")
				phases[phase]++
				cap := getNested(pv, "spec", "capacity", "storage")
				totalCap += parseMemoryBytes(cap)
			}
			fmt.Fprintf(&b, "PersistentVolumes: %d (total capacity: %dGi)\n", len(items), totalCap/(1<<30))
			for phase, count := range phases {
				fmt.Fprintf(&b, "  %s: %d\n", phase, count)
			}
			fmt.Fprintln(&b)
		}
	}

	// Persistent volume claims
	pvcPath := findFile(rootDir,
		"config/persistentvolumeclaim.json",
		"config/persistentvolumeclaims.json")
	if pvcPath != "" {
		items, err := readItems(pvcPath)
		if err == nil && len(items) > 0 {
			phases := make(map[string]int)
			for _, pvc := range items {
				phase := getNested(pvc, "status", "phase")
				phases[phase]++
			}
			fmt.Fprintf(&b, "PersistentVolumeClaims: %d\n", len(items))
			for phase, count := range phases {
				fmt.Fprintf(&b, "  %s: %d\n", phase, count)
			}
		}
	}

	return b.String(), nil
}

func toolAlerts(rootDir string, severity string) (string, error) {
	alertPath := findFile(rootDir, "monitoring/alerts/alerts.json")
	if alertPath == "" {
		return "No alerts data found in Insights archive (monitoring/alerts/alerts.json).", nil
	}

	obj, err := readJSONFile(alertPath)
	if err != nil {
		return "", fmt.Errorf("reading alerts: %w", err)
	}

	type alertInfo struct {
		Name     string
		Severity string
		State    string
		Message  string
	}

	var alerts []alertInfo

	// Prometheus alertmanager format: data.alerts[] or alerts[] or items[]
	var alertList []any
	if data := getNestedSlice(obj, "data", "alerts"); data != nil {
		alertList = data
	} else if data := getNestedSlice(obj, "alerts"); data != nil {
		alertList = data
	} else if data, ok := obj["items"]; ok {
		if list, ok := data.([]any); ok {
			alertList = list
		}
	} else if data, ok := obj["data"]; ok {
		// data might be a direct array
		if list, ok := data.([]any); ok {
			alertList = list
		}
	}

	for _, a := range alertList {
		am, ok := a.(map[string]any)
		if !ok {
			continue
		}
		labels := getNestedMap(am, "labels")
		name := getNested(am, "labels", "alertname")
		if name == "" {
			name = getNested(am, "name")
		}
		sev := ""
		if labels != nil {
			sev = getNested(labels, "severity")
		}
		st := getNested(am, "state")
		if st == "" {
			st = getNested(am, "status", "state")
		}
		msg := getNested(am, "annotations", "message")
		if msg == "" {
			msg = getNested(am, "annotations", "description")
		}

		if severity != "" && !strings.EqualFold(sev, severity) {
			continue
		}

		alerts = append(alerts, alertInfo{
			Name: name, Severity: sev, State: st, Message: msg,
		})
	}

	if len(alerts) == 0 {
		if severity != "" {
			return fmt.Sprintf("No %s alerts found.", severity), nil
		}
		return "No alerts found in Insights archive.", nil
	}

	// Group by severity
	groups := make(map[string][]alertInfo)
	for _, a := range alerts {
		sev := a.Severity
		if sev == "" {
			sev = "none"
		}
		groups[sev] = append(groups[sev], a)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Alerts: %d total\n\n", len(alerts))

	// Order: critical, warning, info, none
	order := []string{"critical", "warning", "info", "none"}
	for _, sev := range order {
		group, ok := groups[sev]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "[%s] %d alerts:\n", strings.ToUpper(sev), len(group))
		for _, a := range group {
			fmt.Fprintf(&b, "  %s", a.Name)
			if a.State != "" {
				fmt.Fprintf(&b, " (state=%s)", a.State)
			}
			fmt.Fprintln(&b)
			if a.Message != "" {
				msg := a.Message
				if len(msg) > 150 {
					msg = msg[:150] + "..."
				}
				fmt.Fprintf(&b, "    %s\n", msg)
			}
		}
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

func toolUIDOverlap(rootDir string) (string, error) {
	nsPath := findFile(rootDir, "config/namespace.json", "config/namespaces.json")
	if nsPath == "" {
		return "No namespace data found in Insights archive for UID range analysis.", nil
	}

	items, err := readItems(nsPath)
	if err != nil {
		return "", fmt.Errorf("reading namespace data: %w", err)
	}

	type uidRange struct {
		Namespace string
		Start     int64
		End       int64
	}

	var ranges []uidRange
	for _, ns := range items {
		name := getNested(ns, "metadata", "name")
		annotations := getNestedMap(ns, "metadata", "annotations")
		if annotations == nil {
			continue
		}
		rangeStr, ok := annotations["openshift.io/sa.scc.uid-range"]
		if !ok {
			continue
		}
		rs, ok := rangeStr.(string)
		if !ok {
			continue
		}
		parts := strings.SplitN(rs, "/", 2)
		if len(parts) != 2 {
			continue
		}
		start, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		ranges = append(ranges, uidRange{
			Namespace: name,
			Start:     start,
			End:       start + size - 1,
		})
	}

	if len(ranges) == 0 {
		return "No UID range annotations found on namespaces.", nil
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start < ranges[j].Start })

	type overlap struct {
		NS1, NS2     string
		Start1, End1 int64
		Start2, End2 int64
	}

	var overlaps []overlap
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[i].End >= ranges[j].Start && ranges[i].Start <= ranges[j].End {
				overlaps = append(overlaps, overlap{
					NS1: ranges[i].Namespace, NS2: ranges[j].Namespace,
					Start1: ranges[i].Start, End1: ranges[i].End,
					Start2: ranges[j].Start, End2: ranges[j].End,
				})
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "UID Range Analysis\n")
	fmt.Fprintf(&b, "Namespaces with UID ranges: %d\n\n", len(ranges))

	if len(overlaps) == 0 {
		fmt.Fprintln(&b, "No UID range overlaps detected.")
	} else {
		fmt.Fprintf(&b, "UID RANGE OVERLAPS DETECTED: %d\n\n", len(overlaps))
		for _, o := range overlaps {
			fmt.Fprintf(&b, "  %s [%d-%d] overlaps with %s [%d-%d]\n",
				o.NS1, o.Start1, o.End1, o.NS2, o.Start2, o.End2)
		}
	}

	return b.String(), nil
}

func toolHealth(rootDir string) (string, error) {
	var b strings.Builder
	var issues []string

	fmt.Fprintf(&b, "Insights Archive Health Summary\n")
	fmt.Fprintf(&b, "===============================\n\n")

	// Cluster version
	if cvPath := findFile(rootDir, "config/clusterversion.json"); cvPath != "" {
		items, err := readItems(cvPath)
		if err == nil && len(items) > 0 {
			fmt.Fprintf(&b, "Version: %s\n", getNested(items[0], "status", "desired", "version"))
			fmt.Fprintf(&b, "Channel: %s\n", getNested(items[0], "spec", "channel"))
		}
	}

	// Nodes
	if nodePath := findFile(rootDir, "config/node.json", "config/nodes.json"); nodePath != "" {
		items, err := readItems(nodePath)
		if err == nil {
			fmt.Fprintf(&b, "Nodes: %d\n", len(items))
			for _, node := range items {
				name := getNested(node, "metadata", "name")
				for _, c := range getNestedSlice(node, "status", "conditions") {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					if getNested(cm, "type") == "Ready" && getNested(cm, "status") != "True" {
						issues = append(issues, fmt.Sprintf("Node not ready: %s", name))
					}
				}
			}
		}
	}

	// Operators
	if opPath := findFile(rootDir, "config/clusteroperator.json", "config/clusteroperators.json"); opPath != "" {
		items, err := readItems(opPath)
		if err == nil {
			var healthy, degraded int
			for _, op := range items {
				name := getNested(op, "metadata", "name")
				for _, c := range getNestedSlice(op, "status", "conditions") {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					ct := getNested(cm, "type")
					cs := getNested(cm, "status")
					if ct == "Degraded" && cs == "True" {
						degraded++
						issues = append(issues, fmt.Sprintf("Operator degraded: %s", name))
					}
					if ct == "Available" && cs == "True" {
						healthy++
					}
				}
			}
			fmt.Fprintf(&b, "Operators: %d healthy, %d degraded\n", healthy, degraded)
		}
	}

	// Alerts
	if alertPath := findFile(rootDir, "monitoring/alerts/alerts.json"); alertPath != "" {
		alertOut, err := toolAlerts(rootDir, "critical")
		if err == nil && strings.Contains(alertOut, "[CRITICAL]") {
			for _, line := range strings.Split(alertOut, "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "Alerts:") {
					issues = append(issues, fmt.Sprintf("Critical alert: %s", line))
				}
			}
		}
	}

	// UID overlaps
	uidOut, err := toolUIDOverlap(rootDir)
	if err == nil && strings.Contains(uidOut, "OVERLAPS DETECTED") {
		issues = append(issues, "UID range overlaps detected (run insights_uid_overlap for details)")
	}

	fmt.Fprintln(&b)
	if len(issues) == 0 {
		fmt.Fprintln(&b, "No critical issues detected.")
	} else {
		fmt.Fprintf(&b, "Issues found: %d\n", len(issues))
		for i, issue := range issues {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, issue)
		}
	}

	return b.String(), nil
}
