package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
