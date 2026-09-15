package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// auditLogDirs are the subdirectory names under a must-gather that contain
// API server audit logs.
var auditLogDirs = []string{
	"audit_logs/kube-apiserver",
	"audit_logs/openshift-apiserver",
	"audit_logs/oauth-apiserver",
}

// findAuditFiles walks the must-gather path looking for audit log files.
func findAuditFiles(root string) ([]string, error) {
	var files []string
	for _, sub := range auditLogDirs {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz") {
				files = append(files, filepath.Join(dir, name))
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no audit log files found under %s — expected JSON-lines files in audit_logs/{kube,openshift,oauth}-apiserver/", root)
	}
	return files, nil
}

// streamEvents calls fn for every audit event in the given file. It handles
// both plain and gzip-compressed files, and uses a bufio.Scanner to avoid
// loading the entire file into memory.
func streamEvents(path string, fn func(*auditEvent) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var reader io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("opening gzip %s: %w", path, err)
		}
		defer gz.Close()
		reader = gz
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Some audit logs prefix lines with a hostname; strip it.
		if line[0] != '{' {
			idx := bytesIndexByte(line, '{')
			if idx < 0 {
				continue
			}
			line = line[idx:]
		}

		var ev auditEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if err := fn(&ev); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// bytesIndexByte returns the index of the first occurrence of c in b, or -1.
func bytesIndexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// streamAllEvents streams events from all audit files under root, applying
// the filter and calling fn for each matching event.
func streamAllEvents(root string, f *filter, fn func(*auditEvent) error) error {
	files, err := findAuditFiles(root)
	if err != nil {
		return err
	}
	for _, path := range files {
		if err := streamEvents(path, func(ev *auditEvent) error {
			if f != nil && !f.matches(ev) {
				return nil
			}
			return fn(ev)
		}); err != nil {
			return fmt.Errorf("processing %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

// ---------- audit_top ----------

func toolAuditTop(root, by string, n int, f *filter) (string, error) {
	counts := map[string]int64{}
	total := int64(0)

	err := streamAllEvents(root, f, func(ev *auditEvent) error {
		total++
		var key string
		switch by {
		case "user":
			key = ev.User.Username
		case "verb":
			key = ev.Verb
		case "resource":
			key = eventResource(ev)
		case "namespace":
			key = eventNamespace(ev)
		}
		counts[key]++
		return nil
	})
	if err != nil {
		return "", err
	}

	type kv struct {
		Key   string
		Count int64
	}
	sorted := make([]kv, 0, len(counts))
	for k, c := range counts {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Top %d by %s (%d total events)\n\n", n, by, total)
	for i, item := range sorted {
		pct := float64(item.Count) / float64(total) * 100
		fmt.Fprintf(&b, "%3d. %-60s %8d  (%5.1f%%)\n", i+1, item.Key, item.Count, pct)
	}
	return b.String(), nil
}

// ---------- audit_search ----------

func toolAuditSearch(root string, max int, f *filter) (string, error) {
	var results []string
	found := 0

	err := streamAllEvents(root, f, func(ev *auditEvent) error {
		if found >= max {
			return nil
		}
		found++

		ts := ev.RequestReceivedTimestamp
		code := 0
		if ev.ResponseStatus != nil {
			code = ev.ResponseStatus.Code
		}
		res := eventResource(ev)
		ns := eventNamespace(ev)
		name := ""
		if ev.ObjectRef != nil {
			name = ev.ObjectRef.Name
		}

		line := fmt.Sprintf("[%s] %-8s %-40s ns=%-20s name=%-30s user=%-40s status=%d",
			ts, ev.Verb, res, ns, name, ev.User.Username, code)
		results = append(results, line)
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "No matching audit events found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Matching events: %d", found)
	if found >= max {
		fmt.Fprintf(&b, " (limit %d reached)", max)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)
	for _, r := range results {
		fmt.Fprintln(&b, r)
	}
	return b.String(), nil
}

// ---------- audit_timeline ----------

func toolAuditTimeline(root, interval string, f *filter) (string, error) {
	buckets := map[string]int64{}
	total := int64(0)
	parseFailures := 0

	truncate := truncateToHour
	layout := "2006-01-02 15:00"
	if interval == "minute" {
		truncate = truncateToMinute
		layout = "2006-01-02 15:04"
	}

	err := streamAllEvents(root, f, func(ev *auditEvent) error {
		total++
		t, err := parseTimestamp(ev.RequestReceivedTimestamp)
		if err != nil {
			parseFailures++
			return nil
		}
		key := truncate(t).Format(layout)
		buckets[key]++
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(buckets) == 0 {
		return "No audit events found.", nil
	}

	// Sort by time bucket key.
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Find max for spike detection.
	var maxCount int64
	var sum int64
	for _, c := range buckets {
		sum += c
		if c > maxCount {
			maxCount = c
		}
	}
	avg := sum / int64(len(buckets))

	var b strings.Builder
	fmt.Fprintf(&b, "Audit Event Timeline (by %s, %d total events, %d buckets)\n\n", interval, total, len(buckets))

	barMax := 50
	for _, k := range keys {
		c := buckets[k]
		barLen := int(float64(c) / float64(maxCount) * float64(barMax))
		if barLen < 1 && c > 0 {
			barLen = 1
		}
		bar := strings.Repeat("#", barLen)
		spike := ""
		if avg > 0 && c > avg*3 {
			spike = " <-- SPIKE"
		}
		fmt.Fprintf(&b, "%-20s %8d  %s%s\n", k, c, bar, spike)
	}

	fmt.Fprintf(&b, "\nAvg: %d/bucket, Max: %d/bucket\n", avg, maxCount)
	return b.String(), nil
}

// ---------- audit_anomalies ----------

// anomalyStats collects data in a single pass for anomaly detection.
type anomalyStats struct {
	total         int64
	authFailures  map[string]int64 // user -> count of 401/403
	deletions     map[string]int64 // user -> delete count
	privEsc       map[string]int64 // user -> privilege escalation count
	saActivity    map[string]int64 // service account -> count
	statusCounts  map[int]int64    // status code -> count
}

func newAnomalyStats() *anomalyStats {
	return &anomalyStats{
		authFailures: map[string]int64{},
		deletions:    map[string]int64{},
		privEsc:      map[string]int64{},
		saActivity:   map[string]int64{},
		statusCounts: map[int]int64{},
	}
}

func toolAuditAnomalies(root string) (string, error) {
	stats := newAnomalyStats()

	err := streamAllEvents(root, nil, func(ev *auditEvent) error {
		stats.total++
		user := ev.User.Username
		code := 0
		if ev.ResponseStatus != nil {
			code = ev.ResponseStatus.Code
		}

		stats.statusCounts[code]++

		// Auth failures
		if code == 401 || code == 403 {
			stats.authFailures[user]++
		}

		// Deletions
		if ev.Verb == "delete" {
			stats.deletions[user]++
		}

		// Privilege escalation
		if isPrivilegeEscalation(ev) {
			stats.privEsc[user]++
		}

		// Service account activity
		if isServiceAccount(user) {
			stats.saActivity[user]++
		}

		return nil
	})
	if err != nil {
		return "", err
	}
	if stats.total == 0 {
		return "No audit events found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Audit Anomaly Report (%d total events)\n", stats.total)
	fmt.Fprintf(&b, "%s\n\n", strings.Repeat("=", 50))

	// 1. Auth failures
	totalAuthFail := stats.statusCounts[401] + stats.statusCounts[403]
	fmt.Fprintf(&b, "--- Auth Failures (401/403) ---\n")
	fmt.Fprintf(&b, "Total: %d (%.1f%% of all events)\n", totalAuthFail, pct(totalAuthFail, stats.total))
	if totalAuthFail > 0 {
		printTopN(&b, stats.authFailures, 10)
	}
	fmt.Fprintln(&b)

	// 2. Mass deletions
	totalDeletions := int64(0)
	for _, c := range stats.deletions {
		totalDeletions += c
	}
	fmt.Fprintf(&b, "--- Deletions ---\n")
	fmt.Fprintf(&b, "Total: %d\n", totalDeletions)
	if totalDeletions > 0 {
		printTopN(&b, stats.deletions, 10)
	}
	fmt.Fprintln(&b)

	// 3. Privilege escalation
	totalPrivEsc := int64(0)
	for _, c := range stats.privEsc {
		totalPrivEsc += c
	}
	fmt.Fprintf(&b, "--- Privilege Escalation Attempts ---\n")
	fmt.Fprintf(&b, "Total: %d (RBAC mutations, impersonation, SA token requests)\n", totalPrivEsc)
	if totalPrivEsc > 0 {
		printTopN(&b, stats.privEsc, 10)
	}
	fmt.Fprintln(&b)

	// 4. Unusual service account activity
	fmt.Fprintf(&b, "--- Service Account Activity ---\n")
	fmt.Fprintf(&b, "Unique service accounts: %d\n", len(stats.saActivity))
	if len(stats.saActivity) > 0 {
		printTopN(&b, stats.saActivity, 10)
	}

	return b.String(), nil
}

// ---------- audit_health ----------

// healthRating represents a health check result level.
type healthRating string

const (
	ratingOK       healthRating = "OK"
	ratingWarn     healthRating = "WARN"
	ratingCritical healthRating = "CRITICAL"
)

type healthCheck struct {
	Name    string
	Rating  healthRating
	Details string
}

func toolAuditHealth(root string) (string, error) {
	stats := newAnomalyStats()
	var earliest, latest time.Time
	parseOK := false

	err := streamAllEvents(root, nil, func(ev *auditEvent) error {
		stats.total++
		user := ev.User.Username
		code := 0
		if ev.ResponseStatus != nil {
			code = ev.ResponseStatus.Code
		}
		stats.statusCounts[code]++
		if code == 401 || code == 403 {
			stats.authFailures[user]++
		}
		if ev.Verb == "delete" {
			stats.deletions[user]++
		}
		if isPrivilegeEscalation(ev) {
			stats.privEsc[user]++
		}
		if isServiceAccount(user) {
			stats.saActivity[user]++
		}

		t, err := parseTimestamp(ev.RequestReceivedTimestamp)
		if err == nil {
			if !parseOK || t.Before(earliest) {
				earliest = t
			}
			if !parseOK || t.After(latest) {
				latest = t
			}
			parseOK = true
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if stats.total == 0 {
		return "No audit events found.", nil
	}

	var checks []healthCheck

	// 1. Auth failures rate
	totalAuthFail := stats.statusCounts[401] + stats.statusCounts[403]
	authPct := pct(totalAuthFail, stats.total)
	authRating := ratingOK
	authDetail := fmt.Sprintf("%d failures (%.1f%% of %d events)", totalAuthFail, authPct, stats.total)
	if authPct > 10 {
		authRating = ratingCritical
	} else if authPct > 5 {
		authRating = ratingWarn
	}
	checks = append(checks, healthCheck{"Auth Failures (401/403)", authRating, authDetail})

	// 2. Deletion rate
	totalDel := int64(0)
	for _, c := range stats.deletions {
		totalDel += c
	}
	delPct := pct(totalDel, stats.total)
	delRating := ratingOK
	delDetail := fmt.Sprintf("%d deletions (%.1f%% of events)", totalDel, delPct)
	if totalDel > 500 {
		delRating = ratingCritical
	} else if totalDel > 100 {
		delRating = ratingWarn
	}
	checks = append(checks, healthCheck{"Deletion Activity", delRating, delDetail})

	// 3. Privilege escalation
	totalPrivEsc := int64(0)
	for _, c := range stats.privEsc {
		totalPrivEsc += c
	}
	privRating := ratingOK
	privDetail := fmt.Sprintf("%d events", totalPrivEsc)
	if totalPrivEsc > 50 {
		privRating = ratingCritical
	} else if totalPrivEsc > 10 {
		privRating = ratingWarn
	}
	checks = append(checks, healthCheck{"Privilege Escalation", privRating, privDetail})

	// 4. Event volume
	volRating := ratingOK
	volDetail := fmt.Sprintf("%d events", stats.total)
	if parseOK {
		dur := latest.Sub(earliest)
		if dur > 0 {
			perHour := float64(stats.total) / dur.Hours()
			volDetail = fmt.Sprintf("%d events over %s (%.0f/hour)", stats.total, dur.Truncate(time.Minute), perHour)
			if perHour > 100000 {
				volRating = ratingWarn
			}
		}
	}
	checks = append(checks, healthCheck{"Event Volume", volRating, volDetail})

	// 5. Server errors (5xx)
	total5xx := int64(0)
	for code, c := range stats.statusCounts {
		if code >= 500 {
			total5xx += c
		}
	}
	errPct := pct(total5xx, stats.total)
	errRating := ratingOK
	errDetail := fmt.Sprintf("%d server errors (%.1f%%)", total5xx, errPct)
	if errPct > 5 {
		errRating = ratingCritical
	} else if errPct > 1 {
		errRating = ratingWarn
	}
	checks = append(checks, healthCheck{"Server Errors (5xx)", errRating, errDetail})

	// Build output
	var b strings.Builder
	fmt.Fprintf(&b, "Audit Log Health Summary\n")
	fmt.Fprintf(&b, "%s\n\n", strings.Repeat("=", 50))

	if parseOK {
		fmt.Fprintf(&b, "Time range: %s to %s\n", earliest.Format(time.RFC3339), latest.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "Total events: %d\n\n", stats.total)

	for _, c := range checks {
		fmt.Fprintf(&b, "[%-8s] %-25s %s\n", c.Rating, c.Name, c.Details)
	}

	// Overall
	overall := ratingOK
	for _, c := range checks {
		if c.Rating == ratingCritical {
			overall = ratingCritical
			break
		}
		if c.Rating == ratingWarn {
			overall = ratingWarn
		}
	}
	fmt.Fprintf(&b, "\nOverall: [%s]\n", overall)

	return b.String(), nil
}

// ---------- helpers ----------

func pct(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

type kvPair struct {
	Key   string
	Count int64
}

func printTopN(b *strings.Builder, m map[string]int64, n int) {
	sorted := make([]kvPair, 0, len(m))
	for k, c := range m {
		sorted = append(sorted, kvPair{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	for _, kv := range sorted {
		fmt.Fprintf(b, "  %-60s %d\n", kv.Key, kv.Count)
	}
}
