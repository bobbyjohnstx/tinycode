package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/pkg/mustgather"
)

// etcd log patterns
var (
	slowApplyPat  = regexp.MustCompile(`apply request took too long`)
	slowFsyncPat  = regexp.MustCompile(`slow fdatasync`)
	leaderPat     = regexp.MustCompile(`elected leader|became leader|lost leader`)
	compactionPat = regexp.MustCompile(`compaction finished`)
	defragPat     = regexp.MustCompile(`defragment`)
	networkPat    = regexp.MustCompile(`failed to send|peer.*unreachable`)
	storagePat    = regexp.MustCompile(`database space exceeded|mvcc: required revision`)
	authPat       = regexp.MustCompile(`auth.*fail|authentication.*error|permission denied`)
	raftErrPat    = regexp.MustCompile(`raft:.*error|raft.*failed`)
	durationPat   = regexp.MustCompile(`\((\d+(?:\.\d+)?)(ms|s)\)`)
)

// findEtcdLogFiles returns all etcd log files from the must-gather.
func findEtcdLogFiles(root *mustgather.Root) []string {
	etcdNS := root.NamespacedPath("openshift-etcd")
	podsDir := filepath.Join(etcdNS, "pods")

	entries, err := os.ReadDir(podsDir)
	if err != nil {
		return nil
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "etcd-") {
			continue
		}
		files, _ := mustgather.FindLogFiles(filepath.Join(podsDir, entry.Name()))
		logFiles = append(logFiles, files...)
	}
	return logFiles
}

// podNameFromLogPath extracts the pod name from a log file path.
func podNameFromLogPath(logPath string) string {
	parts := strings.Split(logPath, string(filepath.Separator))
	for i, p := range parts {
		if p == "pods" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return filepath.Base(logPath)
}

// parseDurationMs extracts a duration in milliseconds from a log line.
func parseDurationMs(line string) (float64, bool) {
	m := durationPat.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	var val float64
	fmt.Sscanf(m[1], "%f", &val)
	if m[2] == "s" {
		val *= 1000
	}
	return val, true
}

// durationStats computes min, max, median, average from a slice of values.
type durationStats struct {
	Min    float64
	Max    float64
	Median float64
	Avg    float64
	Count  int
}

func computeStats(vals []float64) durationStats {
	if len(vals) == 0 {
		return durationStats{}
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 && len(sorted) > 1 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	return durationStats{
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		Median: median,
		Avg:    sum / float64(len(sorted)),
		Count:  len(sorted),
	}
}

func toolStats(root *mustgather.Root) (string, error) {
	logFiles := findEtcdLogFiles(root)
	if len(logFiles) == 0 {
		return "No etcd pod logs found in must-gather.", nil
	}

	var slowApplyDurations []float64
	var slowFsyncDurations []float64
	var compactionDurations []float64
	totalSlowApply := 0
	totalSlowFsync := 0
	totalCompactions := 0

	for _, lf := range logFiles {
		entries, err := mustgather.ReadLogs(lf, 0)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if slowApplyPat.MatchString(e.Raw) {
				totalSlowApply++
				if dur, ok := parseDurationMs(e.Raw); ok {
					slowApplyDurations = append(slowApplyDurations, dur)
				}
			}
			if slowFsyncPat.MatchString(e.Raw) {
				totalSlowFsync++
				if dur, ok := parseDurationMs(e.Raw); ok {
					slowFsyncDurations = append(slowFsyncDurations, dur)
				}
			}
			if compactionPat.MatchString(e.Raw) {
				totalCompactions++
				if dur, ok := parseDurationMs(e.Raw); ok {
					compactionDurations = append(compactionDurations, dur)
				}
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Performance Statistics\n")
	fmt.Fprintf(&b, "==========================\n\n")
	fmt.Fprintf(&b, "Log files analyzed: %d\n\n", len(logFiles))

	fmt.Fprintf(&b, "Slow applies: %d\n", totalSlowApply)
	if len(slowApplyDurations) > 0 {
		s := computeStats(slowApplyDurations)
		fmt.Fprintf(&b, "  Duration (ms): min=%.1f  max=%.1f  median=%.1f  avg=%.1f\n", s.Min, s.Max, s.Median, s.Avg)
	}

	fmt.Fprintf(&b, "\nSlow fsyncs: %d\n", totalSlowFsync)
	if len(slowFsyncDurations) > 0 {
		s := computeStats(slowFsyncDurations)
		fmt.Fprintf(&b, "  Duration (ms): min=%.1f  max=%.1f  median=%.1f  avg=%.1f\n", s.Min, s.Max, s.Median, s.Avg)
	}

	fmt.Fprintf(&b, "\nCompactions: %d\n", totalCompactions)
	if len(compactionDurations) > 0 {
		s := computeStats(compactionDurations)
		fmt.Fprintf(&b, "  Duration (ms): min=%.1f  max=%.1f  median=%.1f  avg=%.1f\n", s.Min, s.Max, s.Median, s.Avg)
	}

	return b.String(), nil
}

func toolErrors(root *mustgather.Root, max int) (string, error) {
	logFiles := findEtcdLogFiles(root)
	if len(logFiles) == 0 {
		return "No etcd pod logs found in must-gather.", nil
	}

	type errorCategory struct {
		name    string
		pattern *regexp.Regexp
		entries []string
	}

	categories := []*errorCategory{
		{name: "Auth Failures", pattern: authPat},
		{name: "Storage Errors", pattern: storagePat},
		{name: "Raft Errors", pattern: raftErrPat},
		{name: "Network Errors", pattern: networkPat},
	}

	totalErrors := 0
	for _, lf := range logFiles {
		pod := podNameFromLogPath(lf)
		entries, err := mustgather.ReadLogs(lf, 0)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if totalErrors >= max {
				break
			}
			for _, cat := range categories {
				if cat.pattern.MatchString(e.Raw) {
					cat.entries = append(cat.entries, fmt.Sprintf("[%s] %s", pod, e.Raw))
					totalErrors++
					break
				}
			}
		}
		if totalErrors >= max {
			break
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Error Analysis\n")
	fmt.Fprintf(&b, "===================\n\n")

	if totalErrors == 0 {
		fmt.Fprintln(&b, "No categorized errors found in etcd logs.")
		return b.String(), nil
	}

	for _, cat := range categories {
		if len(cat.entries) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s: %d\n", cat.name, len(cat.entries))
		for _, entry := range cat.entries {
			line := entry
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			fmt.Fprintf(&b, "  %s\n", line)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintf(&b, "Total categorized errors: %d (limit: %d)\n", totalErrors, max)
	return b.String(), nil
}

// timelineEvent represents a significant etcd event.
type timelineEvent struct {
	timestamp string
	pod       string
	eventType string
	detail    string
}

func toolTimeline(root *mustgather.Root) (string, error) {
	logFiles := findEtcdLogFiles(root)
	if len(logFiles) == 0 {
		return "No etcd pod logs found in must-gather.", nil
	}

	type eventMatcher struct {
		name    string
		pattern *regexp.Regexp
	}

	matchers := []eventMatcher{
		{"leader_election", leaderPat},
		{"compaction", compactionPat},
		{"defragmentation", defragPat},
		{"slow_apply", slowApplyPat},
		{"slow_fsync", slowFsyncPat},
		{"network_issue", networkPat},
		{"storage_issue", storagePat},
	}

	// klog timestamp pattern: [IWEF]MMDD HH:MM:SS.xxxxxx
	klogTimePat := regexp.MustCompile(`^([IWEF])(\d{4})\s+(\d{2}:\d{2}:\d{2}\.\d+)`)

	var events []timelineEvent

	for _, lf := range logFiles {
		pod := podNameFromLogPath(lf)
		entries, err := mustgather.ReadLogs(lf, 0)
		if err != nil {
			continue
		}
		for _, e := range entries {
			for _, m := range matchers {
				if !m.pattern.MatchString(e.Raw) {
					continue
				}
				ts := ""
				if km := klogTimePat.FindStringSubmatch(e.Raw); km != nil {
					ts = fmt.Sprintf("%s %s", km[2], km[3])
				}
				detail := e.Raw
				if len(detail) > 150 {
					detail = detail[:150] + "..."
				}
				events = append(events, timelineEvent{
					timestamp: ts,
					pod:       pod,
					eventType: m.name,
					detail:    detail,
				})
				break
			}
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].timestamp < events[j].timestamp
	})

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Event Timeline\n")
	fmt.Fprintf(&b, "===================\n\n")

	if len(events) == 0 {
		fmt.Fprintln(&b, "No significant events found in etcd logs.")
		return b.String(), nil
	}

	fmt.Fprintf(&b, "Total events: %d\n\n", len(events))
	// Count by type
	typeCounts := make(map[string]int)
	for _, ev := range events {
		typeCounts[ev.eventType]++
	}
	for _, m := range []string{"leader_election", "compaction", "defragmentation", "slow_apply", "slow_fsync", "network_issue", "storage_issue"} {
		if c, ok := typeCounts[m]; ok {
			fmt.Fprintf(&b, "  %s: %d\n", m, c)
		}
	}
	fmt.Fprintln(&b)

	for _, ev := range events {
		fmt.Fprintf(&b, "[%s] %-20s %-20s %s\n", ev.timestamp, ev.eventType, ev.pod, ev.detail)
	}
	return b.String(), nil
}

// podMetrics holds per-pod etcd metrics for comparison.
type podMetrics struct {
	pod              string
	slowApplyCount   int
	slowFsyncCount   int
	leaderChanges    int
	compactions      int
	networkErrors    int
	storageErrors    int
	maxApplyDuration float64
	maxFsyncDuration float64
}

func toolCompare(root *mustgather.Root) (string, error) {
	etcdNS := root.NamespacedPath("openshift-etcd")
	podsDir := filepath.Join(etcdNS, "pods")

	entries, err := os.ReadDir(podsDir)
	if err != nil {
		return "No etcd pod data found in must-gather.", nil
	}

	var metrics []podMetrics

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "etcd-") {
			continue
		}
		pm := podMetrics{pod: entry.Name()}

		logFiles, _ := mustgather.FindLogFiles(filepath.Join(podsDir, entry.Name()))
		for _, lf := range logFiles {
			logEntries, err := mustgather.ReadLogs(lf, 0)
			if err != nil {
				continue
			}
			for _, e := range logEntries {
				if slowApplyPat.MatchString(e.Raw) {
					pm.slowApplyCount++
					if dur, ok := parseDurationMs(e.Raw); ok && dur > pm.maxApplyDuration {
						pm.maxApplyDuration = dur
					}
				}
				if slowFsyncPat.MatchString(e.Raw) {
					pm.slowFsyncCount++
					if dur, ok := parseDurationMs(e.Raw); ok && dur > pm.maxFsyncDuration {
						pm.maxFsyncDuration = dur
					}
				}
				if leaderPat.MatchString(e.Raw) {
					pm.leaderChanges++
				}
				if compactionPat.MatchString(e.Raw) {
					pm.compactions++
				}
				if networkPat.MatchString(e.Raw) {
					pm.networkErrors++
				}
				if storagePat.MatchString(e.Raw) {
					pm.storageErrors++
				}
			}
		}
		metrics = append(metrics, pm)
	}

	if len(metrics) == 0 {
		return "No etcd pods found in must-gather.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Cross-Pod Comparison\n")
	fmt.Fprintf(&b, "========================\n\n")
	fmt.Fprintf(&b, "Pods analyzed: %d\n\n", len(metrics))

	// Table header
	fmt.Fprintf(&b, "%-30s %10s %10s %10s %10s %10s %10s\n",
		"POD", "SLOW_APPLY", "SLOW_FSYNC", "LEADER", "COMPACT", "NET_ERR", "STOR_ERR")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 92))

	for _, pm := range metrics {
		fmt.Fprintf(&b, "%-30s %10d %10d %10d %10d %10d %10d\n",
			pm.pod, pm.slowApplyCount, pm.slowFsyncCount,
			pm.leaderChanges, pm.compactions,
			pm.networkErrors, pm.storageErrors)
	}

	fmt.Fprintf(&b, "\nMax durations:\n")
	fmt.Fprintf(&b, "%-30s %10s %10s\n", "POD", "APPLY(ms)", "FSYNC(ms)")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 52))
	for _, pm := range metrics {
		fmt.Fprintf(&b, "%-30s %10.1f %10.1f\n", pm.pod, pm.maxApplyDuration, pm.maxFsyncDuration)
	}

	// Flag outliers
	if len(metrics) > 1 {
		avgApply := 0.0
		avgFsync := 0.0
		for _, pm := range metrics {
			avgApply += float64(pm.slowApplyCount)
			avgFsync += float64(pm.slowFsyncCount)
		}
		avgApply /= float64(len(metrics))
		avgFsync /= float64(len(metrics))

		var outliers []string
		for _, pm := range metrics {
			if avgApply > 0 && float64(pm.slowApplyCount) > avgApply*2 {
				outliers = append(outliers, fmt.Sprintf("%s has %.0fx more slow applies than average", pm.pod, float64(pm.slowApplyCount)/avgApply))
			}
			if avgFsync > 0 && float64(pm.slowFsyncCount) > avgFsync*2 {
				outliers = append(outliers, fmt.Sprintf("%s has %.0fx more slow fsyncs than average", pm.pod, float64(pm.slowFsyncCount)/avgFsync))
			}
		}
		if len(outliers) > 0 {
			fmt.Fprintf(&b, "\nOutliers (node-specific issues):\n")
			for _, o := range outliers {
				fmt.Fprintf(&b, "  - %s\n", o)
			}
		}
	}

	return b.String(), nil
}

func toolHealth(root *mustgather.Root) (string, error) {
	logFiles := findEtcdLogFiles(root)

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Health Report\n")
	fmt.Fprintf(&b, "==================\n\n")

	if len(logFiles) == 0 {
		fmt.Fprintln(&b, "[WARN] No etcd pod logs found in must-gather.")
		return b.String(), nil
	}

	// Gather metrics across all pods
	totalSlowApply := 0
	totalSlowFsync := 0
	totalLeaderChanges := 0
	totalNetworkErrors := 0
	totalStorageErrors := 0
	var maxApplyDur float64
	var maxFsyncDur float64
	logAge := time.Duration(0)

	klogTimePat := regexp.MustCompile(`^[IWEF](\d{4})\s+(\d{2}:\d{2}:\d{2})`)
	var firstTime, lastTime string

	for _, lf := range logFiles {
		entries, err := mustgather.ReadLogs(lf, 0)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if m := klogTimePat.FindStringSubmatch(e.Raw); m != nil {
				ts := m[1] + " " + m[2]
				if firstTime == "" || ts < firstTime {
					firstTime = ts
				}
				if ts > lastTime {
					lastTime = ts
				}
			}

			if slowApplyPat.MatchString(e.Raw) {
				totalSlowApply++
				if dur, ok := parseDurationMs(e.Raw); ok && dur > maxApplyDur {
					maxApplyDur = dur
				}
			}
			if slowFsyncPat.MatchString(e.Raw) {
				totalSlowFsync++
				if dur, ok := parseDurationMs(e.Raw); ok && dur > maxFsyncDur {
					maxFsyncDur = dur
				}
			}
			if leaderPat.MatchString(e.Raw) {
				totalLeaderChanges++
			}
			if networkPat.MatchString(e.Raw) {
				totalNetworkErrors++
			}
			if storagePat.MatchString(e.Raw) {
				totalStorageErrors++
			}
		}
	}

	_ = logAge // used for future rate computation

	fmt.Fprintf(&b, "Log files: %d\n", len(logFiles))
	if firstTime != "" && lastTime != "" {
		fmt.Fprintf(&b, "Log time range: %s to %s\n", firstTime, lastTime)
	}
	fmt.Fprintln(&b)

	// Slow applies
	status := "[OK]"
	if totalSlowApply > 100 {
		status = "[CRITICAL]"
	} else if totalSlowApply > 10 {
		status = "[WARN]"
	}
	fmt.Fprintf(&b, "%s Slow applies: %d", status, totalSlowApply)
	if maxApplyDur > 0 {
		fmt.Fprintf(&b, " (max: %.1fms)", maxApplyDur)
	}
	fmt.Fprintln(&b)

	// Slow fsyncs
	status = "[OK]"
	if totalSlowFsync > 100 {
		status = "[CRITICAL]"
	} else if totalSlowFsync > 10 {
		status = "[WARN]"
	}
	fmt.Fprintf(&b, "%s Slow fsyncs: %d", status, totalSlowFsync)
	if maxFsyncDur > 0 {
		fmt.Fprintf(&b, " (max: %.1fms)", maxFsyncDur)
	}
	fmt.Fprintln(&b)

	// Leader elections
	status = "[OK]"
	if totalLeaderChanges > 10 {
		status = "[CRITICAL]"
	} else if totalLeaderChanges > 3 {
		status = "[WARN]"
	}
	fmt.Fprintf(&b, "%s Leader elections: %d\n", status, totalLeaderChanges)

	// Network
	status = "[OK]"
	if totalNetworkErrors > 50 {
		status = "[CRITICAL]"
	} else if totalNetworkErrors > 5 {
		status = "[WARN]"
	}
	fmt.Fprintf(&b, "%s Network errors: %d\n", status, totalNetworkErrors)

	// Storage
	status = "[OK]"
	if totalStorageErrors > 0 {
		status = "[CRITICAL]"
	}
	fmt.Fprintf(&b, "%s Storage errors: %d\n", status, totalStorageErrors)

	// Overall
	fmt.Fprintln(&b)
	if totalStorageErrors > 0 || totalSlowApply > 100 || totalSlowFsync > 100 || totalLeaderChanges > 10 || totalNetworkErrors > 50 {
		fmt.Fprintln(&b, "Overall: [CRITICAL] — etcd cluster has significant issues requiring investigation.")
	} else if totalSlowApply > 10 || totalSlowFsync > 10 || totalLeaderChanges > 3 || totalNetworkErrors > 5 {
		fmt.Fprintln(&b, "Overall: [WARN] — etcd cluster shows signs of performance degradation.")
	} else {
		fmt.Fprintln(&b, "Overall: [OK] — etcd cluster appears healthy.")
	}

	return b.String(), nil
}
