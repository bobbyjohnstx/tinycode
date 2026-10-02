package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	bolt "go.etcd.io/bbolt"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func buildSnapshotOpen(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_snapshot_open",
		Description: "Open an etcd snapshot (.db file) for inspection. Returns DB size, total key count, and top resource types.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path to .db snapshot file"},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			return toolSnapshotOpen(st, input.Path)
		},
	}
}

func toolSnapshotOpen(st *state, path string) (string, error) {
	// Close any previously opened DB.
	st.closeDB()

	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat snapshot: %w", err)
	}
	dbSize := fi.Size()

	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return "", fmt.Errorf("open snapshot: %w", err)
	}
	st.setDB(db)

	totalKeys := 0
	typeCounts := make(map[string]int)

	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("key"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, _ []byte) error {
			totalKeys++
			key := string(k)
			if strings.HasPrefix(key, "/registry/") {
				parts := strings.SplitN(key[len("/registry/"):], "/", 2)
				if len(parts) > 0 {
					typeCounts[parts[0]]++
				}
			}
			return nil
		})
	}); err != nil {
		return "", fmt.Errorf("reading snapshot: %w", err)
	}

	// Sort types by count descending.
	type typeCount struct {
		name  string
		count int
	}
	var sorted []typeCount
	for name, count := range typeCounts {
		sorted = append(sorted, typeCount{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Snapshot Opened\n")
	fmt.Fprintf(&b, "====================\n\n")
	fmt.Fprintf(&b, "File: %s\n", path)
	fmt.Fprintf(&b, "DB size: %s\n", formatBytes(dbSize))
	fmt.Fprintf(&b, "Total keys: %d\n\n", totalKeys)

	if len(sorted) > 0 {
		fmt.Fprintf(&b, "Top resource types:\n")
		limit := 20
		if len(sorted) < limit {
			limit = len(sorted)
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(&b, "  %-40s %d\n", sorted[i].name, sorted[i].count)
		}
		if len(sorted) > limit {
			fmt.Fprintf(&b, "  ... and %d more types\n", len(sorted)-limit)
		}
	}

	return b.String(), nil
}

func buildSnapshotResources(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_snapshot_resources",
		Description: "List all Kubernetes resource types in the snapshot with counts and total storage per type.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sort_by": map[string]any{"type": "string", "description": "Sort by 'count' or 'size' (default 'count')"},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				SortBy string `json:"sort_by"`
			}
			if args != nil {
				_ = json.Unmarshal(args, &input)
			}
			if input.SortBy == "" {
				input.SortBy = "count"
			}
			db, err := requireDB(st)
			if err != nil {
				return "", err
			}
			return toolSnapshotResources(db, input.SortBy)
		},
	}
}

type resourceType struct {
	name      string
	count     int
	totalSize int64
}

func toolSnapshotResources(db *bolt.DB, sortBy string) (string, error) {
	types := make(map[string]*resourceType)

	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("key"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			key := string(k)
			if !strings.HasPrefix(key, "/registry/") {
				return nil
			}
			parts := strings.SplitN(key[len("/registry/"):], "/", 2)
			if len(parts) == 0 {
				return nil
			}
			name := parts[0]
			rt, ok := types[name]
			if !ok {
				rt = &resourceType{name: name}
				types[name] = rt
			}
			rt.count++
			rt.totalSize += int64(len(v))
			return nil
		})
	}); err != nil {
		return "", fmt.Errorf("reading snapshot: %w", err)
	}

	var sorted []*resourceType
	for _, rt := range types {
		sorted = append(sorted, rt)
	}

	switch sortBy {
	case "size":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].totalSize > sorted[j].totalSize
		})
	default:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].count > sorted[j].count
		})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Snapshot Resources\n")
	fmt.Fprintf(&b, "======================\n\n")

	if len(sorted) == 0 {
		fmt.Fprintln(&b, "No /registry/ keys found in snapshot.")
		return b.String(), nil
	}

	fmt.Fprintf(&b, "%-40s %8s %12s %10s\n", "TYPE", "COUNT", "TOTAL_SIZE", "AVG_SIZE")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 74))

	for _, rt := range sorted {
		avgSize := int64(0)
		if rt.count > 0 {
			avgSize = rt.totalSize / int64(rt.count)
		}
		fmt.Fprintf(&b, "%-40s %8d %12s %10s\n", rt.name, rt.count, formatBytes(rt.totalSize), formatBytes(avgSize))
	}

	fmt.Fprintf(&b, "\nTotal types: %d\n", len(sorted))

	return b.String(), nil
}

func buildSnapshotGet(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_snapshot_get",
		Description: "Retrieve a specific resource from the snapshot by kind, namespace, and name.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":      map[string]any{"type": "string", "description": "Resource kind (e.g. pods, configmaps, deployments)"},
				"namespace": map[string]any{"type": "string", "description": "Namespace (omit for cluster-scoped resources)"},
				"name":      map[string]any{"type": "string", "description": "Resource name"},
			},
			"required": []string{"kind", "name"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Kind      string `json:"kind"`
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			db, err := requireDB(st)
			if err != nil {
				return "", err
			}
			return toolSnapshotGet(db, input.Kind, input.Namespace, input.Name)
		},
	}
}

func toolSnapshotGet(db *bolt.DB, kind, namespace, name string) (string, error) {
	// Build key path: /registry/<kind>/<namespace>/<name> or /registry/<kind>/<name>
	var keyPath string
	if namespace != "" {
		keyPath = fmt.Sprintf("/registry/%s/%s/%s", kind, namespace, name)
	} else {
		keyPath = fmt.Sprintf("/registry/%s/%s", kind, name)
	}

	var value []byte
	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("key"))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(keyPath))
		if v != nil {
			value = make([]byte, len(v))
			copy(value, v)
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("reading snapshot: %w", err)
	}

	if value == nil {
		return fmt.Sprintf("Key not found: %s", keyPath), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Key: %s\n", keyPath)
	fmt.Fprintf(&b, "Size: %s\n\n", formatBytes(int64(len(value))))

	// Detect encoding.
	if len(value) >= 4 && bytes.Equal(value[:4], k8sMagic) {
		tm, err := decodeTypeMeta(value)
		if err != nil {
			fmt.Fprintf(&b, "Protobuf encoded (decode error: %v)\n", err)
			fmt.Fprintf(&b, "Raw size: %d bytes\n", len(value))
		} else {
			fmt.Fprintf(&b, "Encoding: protobuf\n")
			fmt.Fprintf(&b, "APIVersion: %s\n", tm.APIVersion)
			fmt.Fprintf(&b, "Kind: %s\n", tm.Kind)
			fmt.Fprintf(&b, "Raw size: %d bytes\n", len(value))
		}
	} else {
		// Try JSON pretty-print.
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, value, "", "  "); err == nil {
			fmt.Fprintf(&b, "Encoding: JSON\n\n")
			b.Write(prettyJSON.Bytes())
		} else {
			fmt.Fprintf(&b, "Encoding: unknown\n")
			fmt.Fprintf(&b, "Raw size: %d bytes\n", len(value))
		}
	}

	return b.String(), nil
}

func buildSnapshotSearch(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_snapshot_search",
		Description: "Search for resources matching a pattern across key paths in the snapshot.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Substring or regex to match against key paths"},
				"max":     map[string]any{"type": "integer", "description": "Maximum results to return (default 50)"},
			},
			"required": []string{"pattern"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Pattern string `json:"pattern"`
				Max     int    `json:"max"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if input.Max == 0 {
				input.Max = 50
			}
			db, err := requireDB(st)
			if err != nil {
				return "", err
			}
			return toolSnapshotSearch(db, input.Pattern, input.Max)
		},
	}
}

func toolSnapshotSearch(db *bolt.DB, pattern string, max int) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Fall back to substring match.
		re = nil
	}

	type match struct {
		key  string
		size int
	}
	var matches []match
	totalMatched := 0

	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("key"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			key := string(k)
			matched := false
			if re != nil {
				matched = re.MatchString(key)
			} else {
				matched = strings.Contains(key, pattern)
			}
			if matched {
				totalMatched++
				if len(matches) < max {
					matches = append(matches, match{key: key, size: len(v)})
				}
			}
			return nil
		})
	}); err != nil {
		return "", fmt.Errorf("reading snapshot: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Snapshot Search\n")
	fmt.Fprintf(&b, "====================\n\n")
	fmt.Fprintf(&b, "Pattern: %s\n", pattern)
	fmt.Fprintf(&b, "Matches: %d\n\n", totalMatched)

	if len(matches) == 0 {
		fmt.Fprintln(&b, "No matching keys found.")
		return b.String(), nil
	}

	for _, m := range matches {
		fmt.Fprintf(&b, "  %-70s %s\n", m.key, formatBytes(int64(m.size)))
	}

	if totalMatched > max {
		fmt.Fprintf(&b, "\n  ... %d more matches (showing first %d)\n", totalMatched-max, max)
	}

	return b.String(), nil
}

func buildSnapshotStorage(st *state) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "etcd_snapshot_storage",
		Description: "Storage analysis of the snapshot: largest resources, size distribution, and compaction recommendation.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			db, err := requireDB(st)
			if err != nil {
				return "", err
			}
			return toolSnapshotStorage(st, db)
		},
	}
}

func toolSnapshotStorage(st *state, db *bolt.DB) (string, error) {
	type keyEntry struct {
		key  string
		size int
	}

	var allKeys []keyEntry
	typeSizes := make(map[string]int64)
	typeCounts := make(map[string]int)
	totalValueBytes := int64(0)

	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("key"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			key := string(k)
			size := len(v)
			allKeys = append(allKeys, keyEntry{key: key, size: size})
			totalValueBytes += int64(size)

			if strings.HasPrefix(key, "/registry/") {
				parts := strings.SplitN(key[len("/registry/"):], "/", 2)
				if len(parts) > 0 {
					typeSizes[parts[0]] += int64(size)
					typeCounts[parts[0]]++
				}
			}
			return nil
		})
	}); err != nil {
		return "", fmt.Errorf("reading snapshot: %w", err)
	}

	// Sort keys by value size descending for top-20.
	sort.Slice(allKeys, func(i, j int) bool {
		return allKeys[i].size > allKeys[j].size
	})

	// Get DB file size.
	var dbFileSize int64
	st.mu.RLock()
	if st.db != nil {
		path := db.Path()
		if fi, err := os.Stat(path); err == nil {
			dbFileSize = fi.Size()
		}
	}
	st.mu.RUnlock()

	var b strings.Builder
	fmt.Fprintf(&b, "etcd Snapshot Storage Analysis\n")
	fmt.Fprintf(&b, "==============================\n\n")

	fmt.Fprintf(&b, "Total keys: %d\n", len(allKeys))
	fmt.Fprintf(&b, "Total value data: %s\n", formatBytes(totalValueBytes))
	if dbFileSize > 0 {
		fmt.Fprintf(&b, "DB file size: %s\n", formatBytes(dbFileSize))
		if totalValueBytes > 0 {
			overhead := float64(dbFileSize-totalValueBytes) / float64(dbFileSize) * 100
			fmt.Fprintf(&b, "Overhead (fragmentation): %.1f%%\n", overhead)
			if overhead > 50 {
				fmt.Fprintln(&b, "  Recommendation: consider running etcd defragmentation")
			}
		}
	}

	// Top 20 largest keys.
	fmt.Fprintf(&b, "\nTop 20 largest keys:\n")
	limit := 20
	if len(allKeys) < limit {
		limit = len(allKeys)
	}
	for i := 0; i < limit; i++ {
		fmt.Fprintf(&b, "  %-70s %s\n", allKeys[i].key, formatBytes(int64(allKeys[i].size)))
	}

	// Size distribution by resource type.
	type typeDist struct {
		name  string
		size  int64
		count int
	}
	var dist []typeDist
	for name, size := range typeSizes {
		dist = append(dist, typeDist{name: name, size: size, count: typeCounts[name]})
	}
	sort.Slice(dist, func(i, j int) bool {
		return dist[i].size > dist[j].size
	})

	if len(dist) > 0 {
		fmt.Fprintf(&b, "\nSize distribution by resource type:\n")
		fmt.Fprintf(&b, "%-40s %8s %12s\n", "TYPE", "COUNT", "TOTAL_SIZE")
		fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 62))
		for _, d := range dist {
			fmt.Fprintf(&b, "%-40s %8d %12s\n", d.name, d.count, formatBytes(d.size))
		}
	}

	return b.String(), nil
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
