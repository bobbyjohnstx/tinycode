package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

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
