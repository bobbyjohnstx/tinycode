package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/mustgather"
)

func toolClusterVersion(root *mustgather.Root) (string, error) {
	dir := root.ClusterScopedPath("config.openshift.io", "clusterversions")
	resources, err := mustgather.WalkResources(dir)
	if err != nil {
		return "", fmt.Errorf("reading cluster versions: %w", err)
	}
	if len(resources) == 0 {
		return "No ClusterVersion resources found.", nil
	}

	var b strings.Builder
	for _, r := range resources {
		obj := r.Object
		name := mustgather.GetNested(obj, "metadata", "name")
		version := mustgather.GetNested(obj, "status", "desired", "version")
		channel := mustgather.GetNested(obj, "spec", "channel")
		clusterID := mustgather.GetNested(obj, "spec", "clusterID")
		image := mustgather.GetNested(obj, "status", "desired", "image")

		fmt.Fprintf(&b, "ClusterVersion: %s\n", name)
		fmt.Fprintf(&b, "  Version: %s\n", version)
		fmt.Fprintf(&b, "  Channel: %s\n", channel)
		if clusterID != "" {
			fmt.Fprintf(&b, "  Cluster ID: %s\n", clusterID)
		}
		if image != "" {
			parts := strings.Split(image, "@")
			if len(parts) > 0 {
				fmt.Fprintf(&b, "  Image: %s\n", parts[0])
			}
		}

		history := mustgather.GetNestedSlice(obj, "status", "history")
		if len(history) > 0 {
			fmt.Fprintf(&b, "  Upgrade history:\n")
			for _, h := range history {
				hm, ok := h.(map[string]any)
				if !ok {
					continue
				}
				hv := mustgather.GetNested(hm, "version")
				hs := mustgather.GetNested(hm, "state")
				ht := mustgather.GetNested(hm, "completionTime")
				fmt.Fprintf(&b, "    %s — %s (completed: %s)\n", hv, hs, ht)
			}
		}

		conditions := mustgather.GetNestedSlice(obj, "status", "conditions")
		if len(conditions) > 0 {
			fmt.Fprintf(&b, "  Conditions:\n")
			for _, c := range conditions {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				ct := mustgather.GetNested(cm, "type")
				cs := mustgather.GetNested(cm, "status")
				cmsg := mustgather.GetNested(cm, "message")
				fmt.Fprintf(&b, "    %s=%s", ct, cs)
				if cmsg != "" && len(cmsg) < 200 {
					fmt.Fprintf(&b, " (%s)", cmsg)
				}
				fmt.Fprintln(&b)
			}
		}
	}
	return b.String(), nil
}

func toolNodes(root *mustgather.Root, roleFilter string) (string, error) {
	dir := root.ClusterScopedPath("core", "nodes")
	resources, err := mustgather.WalkResources(dir)
	if err != nil {
		return "", fmt.Errorf("reading nodes: %w", err)
	}
	if len(resources) == 0 {
		return "No Node resources found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Nodes: %d\n\n", len(resources))

	for _, r := range resources {
		obj := r.Object
		name := r.Name()

		labels, _ := obj["metadata"].(map[string]any)
		labelMap, _ := labels["labels"].(map[string]any)
		var roles []string
		for k := range labelMap {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
				roles = append(roles, role)
			}
		}
		sort.Strings(roles)

		if roleFilter != "" {
			found := false
			for _, r := range roles {
				if r == roleFilter {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		conditions := mustgather.GetNestedSlice(obj, "status", "conditions")
		ready := "Unknown"
		var issues []string
		for _, c := range conditions {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			ct := mustgather.GetNested(cm, "type")
			cs := mustgather.GetNested(cm, "status")
			if ct == "Ready" {
				if cs == "True" {
					ready = "Ready"
				} else {
					ready = "NotReady"
				}
			}
			if ct != "Ready" && cs == "True" {
				issues = append(issues, ct)
			}
		}

		cpuCap := mustgather.GetNested(obj, "status", "capacity", "cpu")
		memCap := mustgather.GetNested(obj, "status", "capacity", "memory")
		kubeletVer := mustgather.GetNested(obj, "status", "nodeInfo", "kubeletVersion")

		fmt.Fprintf(&b, "%-40s %s  roles=[%s]  kubelet=%s  cpu=%s  mem=%s",
			name, ready, strings.Join(roles, ","), kubeletVer, cpuCap, memCap)
		if len(issues) > 0 {
			fmt.Fprintf(&b, "  issues=[%s]", strings.Join(issues, ","))
		}
		fmt.Fprintln(&b)
	}
	return b.String(), nil
}

func toolOperators(root *mustgather.Root) (string, error) {
	dir := root.ClusterScopedPath("config.openshift.io", "clusteroperators")
	resources, err := mustgather.WalkResources(dir)
	if err != nil {
		return "", fmt.Errorf("reading operators: %w", err)
	}
	if len(resources) == 0 {
		return "No ClusterOperator resources found.", nil
	}

	var healthy, degraded, progressing int
	var b strings.Builder
	fmt.Fprintf(&b, "ClusterOperators: %d\n\n", len(resources))
	fmt.Fprintf(&b, "%-45s %-10s %-10s %-12s %s\n", "NAME", "AVAILABLE", "DEGRADED", "PROGRESSING", "VERSION")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 100))

	for _, r := range resources {
		obj := r.Object
		name := r.Name()
		conditions := mustgather.GetNestedSlice(obj, "status", "conditions")

		avail, deg, prog := "Unknown", "Unknown", "Unknown"
		for _, c := range conditions {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			ct := mustgather.GetNested(cm, "type")
			cs := mustgather.GetNested(cm, "status")
			switch ct {
			case "Available":
				avail = cs
			case "Degraded":
				deg = cs
			case "Progressing":
				prog = cs
			}
		}

		version := ""
		versions := mustgather.GetNestedSlice(obj, "status", "versions")
		for _, v := range versions {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if mustgather.GetNested(vm, "name") == "operator" {
				version = mustgather.GetNested(vm, "version")
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

func toolCerts(root *mustgather.Root, expiredOnly bool, days int) (string, error) {
	var allCerts []mustgather.CertInfo

	for _, ns := range root.Namespaces {
		secretsDir := filepath.Join(root.NamespacedPath(ns), "core", "secrets")
		resources, err := mustgather.WalkResources(secretsDir)
		if err != nil {
			continue
		}
		for _, r := range resources {
			source := fmt.Sprintf("%s/%s", ns, r.Name())
			certs := mustgather.ParseCertsFromSecret(r.Object, source)
			allCerts = append(allCerts, certs...)
		}
	}

	if len(allCerts) == 0 {
		return "No certificates found in must-gather secrets.", nil
	}

	sort.Slice(allCerts, func(i, j int) bool {
		return allCerts[i].DaysLeft < allCerts[j].DaysLeft
	})

	var b strings.Builder
	var expired, warning, ok int
	for _, c := range allCerts {
		if c.Expired {
			expired++
		} else if c.DaysLeft < days {
			warning++
		} else {
			ok++
		}
	}

	fmt.Fprintf(&b, "Certificates: %d total — %d expired, %d expiring within %d days, %d OK\n\n", len(allCerts), expired, warning, days, ok)

	for _, c := range allCerts {
		if expiredOnly && !c.Expired && c.DaysLeft >= days {
			continue
		}
		status := "OK"
		if c.Expired {
			status = "EXPIRED"
		} else if c.DaysLeft < days {
			status = "EXPIRING"
		}
		fmt.Fprintf(&b, "[%s] %s (days left: %d, expires: %s, source: %s)\n",
			status, c.Subject, c.DaysLeft, c.NotAfter.Format("2006-01-02"), c.Source)
	}
	return b.String(), nil
}

func toolNodeLogs(root *mustgather.Root, node, pattern string, max int) (string, error) {
	logsDir := root.HostServiceLogsPath()
	if _, err := os.Stat(logsDir); err != nil {
		return "No host_service_logs directory found in must-gather.", nil
	}

	var searchPat *regexp.Regexp
	if pattern != "" {
		var err error
		searchPat, err = regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("invalid pattern: %w", err)
		}
	}

	logFiles, err := mustgather.FindLogFiles(logsDir)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	totalFound := 0

	for _, lf := range logFiles {
		if node != "" && !strings.Contains(lf, node) {
			continue
		}

		rel, _ := filepath.Rel(root.Path, lf)

		if searchPat == nil {
			entries, err := mustgather.ReadLogs(lf, 20)
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "=== %s (%d lines shown) ===\n", rel, len(entries))
			for _, e := range entries {
				fmt.Fprintln(&b, e.Raw)
			}
			fmt.Fprintln(&b)
		} else {
			remaining := max - totalFound
			if remaining <= 0 {
				break
			}
			entries, err := mustgather.SearchLogs(lf, searchPat, remaining)
			if err != nil {
				continue
			}
			if len(entries) > 0 {
				fmt.Fprintf(&b, "=== %s (%d matches) ===\n", rel, len(entries))
				for _, e := range entries {
					fmt.Fprintln(&b, e.Raw)
				}
				fmt.Fprintln(&b)
				totalFound += len(entries)
			}
		}
	}

	if totalFound == 0 && searchPat != nil {
		return fmt.Sprintf("No matches for pattern %q in host service logs.", pattern), nil
	}
	return b.String(), nil
}

func toolOvn(root *mustgather.Root) (string, error) {
	var b strings.Builder

	var netpolCount int
	for _, ns := range root.Namespaces {
		npDir := filepath.Join(root.NamespacedPath(ns), "networking.k8s.io", "networkpolicies")
		resources, err := mustgather.WalkResources(npDir)
		if err != nil {
			continue
		}
		netpolCount += len(resources)
	}
	fmt.Fprintf(&b, "NetworkPolicies: %d across all namespaces\n", netpolCount)

	egressDir := root.ClusterScopedPath("k8s.ovn.org", "egressips")
	egressIPs, err := mustgather.WalkResources(egressDir)
	if err == nil && len(egressIPs) > 0 {
		fmt.Fprintf(&b, "EgressIPs: %d\n", len(egressIPs))
		for _, r := range egressIPs {
			fmt.Fprintf(&b, "  %s\n", r.Name())
		}
	} else {
		fmt.Fprintln(&b, "EgressIPs: none found")
	}

	ovnNS := root.NamespacedPath("openshift-ovn-kubernetes")
	podsDir := filepath.Join(ovnNS, "pods")
	if entries, err := os.ReadDir(podsDir); err == nil {
		errorPat := regexp.MustCompile(`(?i)error|failed|timeout`)
		fmt.Fprintf(&b, "\nOVN pods: %d\n", len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			logFiles, _ := mustgather.FindLogFiles(filepath.Join(podsDir, entry.Name()))
			for _, lf := range logFiles {
				counts, _ := mustgather.CountLogPatterns(lf, map[string]*regexp.Regexp{"errors": errorPat})
				if counts["errors"] > 0 {
					rel, _ := filepath.Rel(root.Path, lf)
					fmt.Fprintf(&b, "  %s: %d error lines\n", rel, counts["errors"])
				}
			}
		}
	}

	return b.String(), nil
}

func toolPrometheus(root *mustgather.Root, metric string) (string, error) {
	monDir := root.MonitoringPath()
	if _, err := os.Stat(monDir); err != nil {
		return "No monitoring directory found in must-gather.", nil
	}

	matches, err := mustgather.FindResourceFiles(monDir, "*.json")
	if err != nil {
		return "", err
	}

	textFiles, _ := mustgather.FindResourceFiles(monDir, "*.txt")
	matches = append(matches, textFiles...)

	if len(matches) == 0 {
		return "No metrics files found in monitoring directory. Prometheus data may be in a tar.gz archive — use insights_use to extract it.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Metrics files: %d\n\n", len(matches))

	if metric != "" {
		pat := regexp.MustCompile(metric)
		for _, f := range matches {
			rel, _ := filepath.Rel(root.Path, f)
			entries, err := mustgather.SearchLogs(f, pat, 50)
			if err != nil {
				continue
			}
			if len(entries) > 0 {
				fmt.Fprintf(&b, "=== %s (%d matches) ===\n", rel, len(entries))
				for _, e := range entries {
					fmt.Fprintln(&b, e.Raw)
				}
				fmt.Fprintln(&b)
			}
		}
	} else {
		for _, f := range matches {
			rel, _ := filepath.Rel(root.Path, f)
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "  %s (%d bytes)\n", rel, info.Size())
		}
	}

	return b.String(), nil
}

func toolMachineConfig(root *mustgather.Root) (string, error) {
	var b strings.Builder

	mcpDir := root.ClusterScopedPath("machineconfiguration.openshift.io", "machineconfigpools")
	mcps, err := mustgather.WalkResources(mcpDir)
	if err == nil && len(mcps) > 0 {
		fmt.Fprintf(&b, "MachineConfigPools: %d\n\n", len(mcps))
		for _, r := range mcps {
			obj := r.Object
			name := r.Name()
			updated := mustgather.GetNested(obj, "status", "updatedMachineCount")
			ready := mustgather.GetNested(obj, "status", "readyMachineCount")
			total := mustgather.GetNested(obj, "status", "machineCount")
			degraded := mustgather.GetNested(obj, "status", "degradedMachineCount")
			currentConfig := mustgather.GetNested(obj, "status", "configuration", "name")

			fmt.Fprintf(&b, "  %s: %s/%s ready, %s/%s updated, %s degraded, config=%s\n",
				name, ready, total, updated, total, degraded, currentConfig)

			conditions := mustgather.GetNestedSlice(obj, "status", "conditions")
			for _, c := range conditions {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				ct := mustgather.GetNested(cm, "type")
				cs := mustgather.GetNested(cm, "status")
				if (ct == "Updated" || ct == "Updating" || ct == "Degraded") && cs == "True" {
					msg := mustgather.GetNested(cm, "message")
					fmt.Fprintf(&b, "    %s=%s", ct, cs)
					if msg != "" && len(msg) < 150 {
						fmt.Fprintf(&b, " (%s)", msg)
					}
					fmt.Fprintln(&b)
				}
			}
		}
	} else {
		fmt.Fprintln(&b, "No MachineConfigPool resources found.")
	}

	mcDir := root.ClusterScopedPath("machineconfiguration.openshift.io", "machineconfigs")
	mcs, err := mustgather.WalkResources(mcDir)
	if err == nil {
		fmt.Fprintf(&b, "\nMachineConfigs: %d total\n", len(mcs))
	}

	return b.String(), nil
}

func toolPods(root *mustgather.Root, namespace, statusFilter string) (string, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	} else {
		namespaces = root.Namespaces
	}

	type podInfo struct {
		Name      string
		Namespace string
		Phase     string
		Restarts  int64
		Node      string
	}

	var pods []podInfo
	for _, ns := range namespaces {
		podsFile := filepath.Join(root.NamespacedPath(ns), "core", "pods.yaml")
		resources, err := mustgather.ReadResourceList(podsFile)
		if err != nil {
			continue
		}
		for _, r := range resources {
			obj := r.Object
			phase := mustgather.GetNested(obj, "status", "phase")
			if statusFilter != "" && !strings.EqualFold(phase, statusFilter) {
				continue
			}

			var totalRestarts int64
			containerStatuses := mustgather.GetNestedSlice(obj, "status", "containerStatuses")
			for _, cs := range containerStatuses {
				csm, ok := cs.(map[string]any)
				if !ok {
					continue
				}
				rc, _ := csm["restartCount"].(float64)
				totalRestarts += int64(rc)
			}

			pods = append(pods, podInfo{
				Name:      r.Name(),
				Namespace: r.Namespace(),
				Phase:     phase,
				Restarts:  totalRestarts,
				Node:      mustgather.GetNested(obj, "spec", "nodeName"),
			})
		}
	}

	if len(pods) == 0 {
		if statusFilter != "" {
			return fmt.Sprintf("No pods with status %q found.", statusFilter), nil
		}
		return "No pod resources found in must-gather.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Pods: %d\n\n", len(pods))

	phases := make(map[string]int)
	for _, p := range pods {
		phases[p.Phase]++
	}
	for phase, count := range phases {
		fmt.Fprintf(&b, "  %s: %d\n", phase, count)
	}
	fmt.Fprintln(&b)

	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Phase != "Running" && pods[j].Phase == "Running" {
			return true
		}
		if pods[i].Phase == "Running" && pods[j].Phase != "Running" {
			return false
		}
		return pods[i].Restarts > pods[j].Restarts
	})

	shown := 0
	for _, p := range pods {
		if shown >= 200 {
			fmt.Fprintf(&b, "\n... and %d more pods\n", len(pods)-200)
			break
		}
		fmt.Fprintf(&b, "%-60s %-12s restarts=%-4d node=%s\n",
			p.Namespace+"/"+p.Name, p.Phase, p.Restarts, p.Node)
		shown++
	}
	return b.String(), nil
}

func toolEvents(root *mustgather.Root, namespace, eventType, reason string) (string, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	} else {
		namespaces = root.Namespaces
	}

	type eventInfo struct {
		Namespace string
		Name      string
		Type      string
		Reason    string
		Message   string
		Count     int64
		Object    string
	}

	var events []eventInfo
	for _, ns := range namespaces {
		eventsFile := filepath.Join(root.NamespacedPath(ns), "core", "events.yaml")
		resources, err := mustgather.ReadResourceList(eventsFile)
		if err != nil {
			continue
		}
		for _, r := range resources {
			obj := r.Object
			eType := mustgather.GetNested(obj, "type")
			eReason := mustgather.GetNested(obj, "reason")

			if eventType != "" && !strings.EqualFold(eType, eventType) {
				continue
			}
			if reason != "" {
				pat, err := regexp.Compile("(?i)" + reason)
				if err != nil || !pat.MatchString(eReason) {
					continue
				}
			}

			count, _ := obj["count"].(float64)
			involvedObj := mustgather.GetNested(obj, "involvedObject", "name")
			involvedKind := mustgather.GetNested(obj, "involvedObject", "kind")

			events = append(events, eventInfo{
				Namespace: ns,
				Name:      r.Name(),
				Type:      eType,
				Reason:    eReason,
				Message:   mustgather.GetNested(obj, "message"),
				Count:     int64(count),
				Object:    fmt.Sprintf("%s/%s", involvedKind, involvedObj),
			})
		}
	}

	if len(events) == 0 {
		return "No matching events found in must-gather.", nil
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Type == "Warning" && events[j].Type != "Warning" {
			return true
		}
		if events[i].Type != "Warning" && events[j].Type == "Warning" {
			return false
		}
		return events[i].Count > events[j].Count
	})

	var b strings.Builder
	fmt.Fprintf(&b, "Events: %d\n\n", len(events))

	shown := 0
	for _, e := range events {
		if shown >= 200 {
			fmt.Fprintf(&b, "\n... and %d more events\n", len(events)-200)
			break
		}
		msg := e.Message
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		fmt.Fprintf(&b, "[%s] %s/%s: %s (count=%d, object=%s)\n  %s\n",
			e.Type, e.Namespace, e.Name, e.Reason, e.Count, e.Object, msg)
		shown++
	}
	return b.String(), nil
}

func toolHealth(root *mustgather.Root) (string, error) {
	var b strings.Builder
	var issues []string

	fmt.Fprintf(&b, "Must-Gather Health Summary\n")
	fmt.Fprintf(&b, "=========================\n\n")

	cvOut, err := toolClusterVersion(root)
	if err == nil && cvOut != "" {
		lines := strings.Split(cvOut, "\n")
		for _, l := range lines {
			if strings.Contains(l, "Version:") || strings.Contains(l, "Channel:") {
				fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(l))
			}
		}
	}
	fmt.Fprintln(&b)

	nodeOut, err := toolNodes(root, "")
	if err == nil {
		for _, line := range strings.Split(nodeOut, "\n") {
			if strings.Contains(line, "NotReady") {
				issues = append(issues, fmt.Sprintf("Node not ready: %s", strings.Fields(line)[0]))
			}
		}
		nodeCount := strings.Count(nodeOut, "\n") - 2
		fmt.Fprintf(&b, "Nodes: %d\n", nodeCount)
	}

	opOut, err := toolOperators(root)
	if err == nil {
		for _, line := range strings.Split(opOut, "\n") {
			if strings.Contains(line, "Summary:") {
				fmt.Fprintf(&b, "Operators: %s\n", strings.TrimPrefix(strings.TrimSpace(line), "Summary: "))
			}
		}
		for _, line := range strings.Split(opOut, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[2] == "True" {
				issues = append(issues, fmt.Sprintf("Operator degraded: %s", fields[0]))
			}
		}
	}

	certOut, err := toolCerts(root, true, 30)
	if err == nil {
		for _, line := range strings.Split(certOut, "\n") {
			if strings.Contains(line, "Certificates:") {
				fmt.Fprintf(&b, "%s\n", strings.TrimSpace(line))
			}
			if strings.Contains(line, "[EXPIRED]") {
				issues = append(issues, fmt.Sprintf("Certificate expired: %s", line))
			}
		}
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

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "For detailed etcd diagnostics, use the etcd-diag plugin (etcd_diag_health for log analysis, etcd_snapshot_open for offline snapshot inspection). For HAProxy/ingress inspection, use the ingress-inspect plugin.")

	return b.String(), nil
}
