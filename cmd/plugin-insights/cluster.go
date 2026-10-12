package main

import (
	"fmt"
	"sort"
	"strings"
)

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
