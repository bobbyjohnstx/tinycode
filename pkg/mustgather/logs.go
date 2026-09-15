package mustgather

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LogEntry represents a single line from a must-gather log file.
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Raw       string
}

// Common timestamp patterns found in OpenShift component logs.
var timestampPatterns = []*regexp.Regexp{
	// ISO 8601: 2024-01-15T10:30:45.123456Z
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[\.\d]*Z?)`),
	// klog: I0115 10:30:45.123456
	regexp.MustCompile(`^([IWEF])(\d{4})\s+(\d{2}:\d{2}:\d{2}\.\d+)`),
	// journald: Jan 15 10:30:45
	regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`),
}

var klogLevels = map[byte]string{
	'I': "INFO",
	'W': "WARN",
	'E': "ERROR",
	'F': "FATAL",
}

// ReadLogs reads a log file and returns parsed entries. If maxLines is 0,
// all lines are returned.
func ReadLogs(path string, maxLines int) ([]LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening log: %w", err)
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	count := 0
	for scanner.Scan() {
		if maxLines > 0 && count >= maxLines {
			break
		}
		line := scanner.Text()
		entries = append(entries, parseLine(line))
		count++
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scanning log: %w", err)
	}
	return entries, nil
}

// SearchLogs reads a log file and returns only lines matching the pattern.
func SearchLogs(path string, pattern *regexp.Regexp, maxResults int) ([]LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening log: %w", err)
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		if maxResults > 0 && len(entries) >= maxResults {
			break
		}
		line := scanner.Text()
		if pattern.MatchString(line) {
			entries = append(entries, parseLine(line))
		}
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scanning log: %w", err)
	}
	return entries, nil
}

// FindLogFiles searches for log files under a must-gather directory.
func FindLogFiles(root string) ([]string, error) {
	var logs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if strings.HasSuffix(name, ".log") || name == "current.log" ||
			strings.Contains(name, "journal") || strings.HasPrefix(name, "log") {
			logs = append(logs, path)
		}
		return nil
	})
	return logs, err
}

// CountLogPatterns counts occurrences of each pattern in a log file.
func CountLogPatterns(path string, patterns map[string]*regexp.Regexp) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening log: %w", err)
	}
	defer f.Close()

	counts := make(map[string]int, len(patterns))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		for name, pat := range patterns {
			if pat.MatchString(line) {
				counts[name]++
			}
		}
	}
	return counts, scanner.Err()
}

func parseLine(line string) LogEntry {
	entry := LogEntry{Raw: line, Message: line}

	// Try klog format first (most common in OCP components)
	if len(line) > 0 {
		if level, ok := klogLevels[line[0]]; ok {
			m := timestampPatterns[1].FindStringSubmatch(line)
			if m != nil {
				entry.Level = level
				rest := strings.TrimLeft(line[len(m[0]):], " ")
				entry.Message = rest
				return entry
			}
		}
	}

	// Try ISO 8601
	m := timestampPatterns[0].FindStringSubmatch(line)
	if m != nil {
		ts := m[1]
		if !strings.HasSuffix(ts, "Z") {
			ts += "Z"
		}
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			entry.Timestamp = t
		}
		entry.Message = strings.TrimSpace(line[len(m[0]):])
		return entry
	}

	return entry
}
