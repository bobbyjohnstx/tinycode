package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
