package mustgather

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Resource is a lightweight representation of a Kubernetes resource parsed
// from a must-gather YAML/JSON file. No k8s API machinery required.
type Resource struct {
	Object map[string]any
}

// Name returns metadata.name.
func (r *Resource) Name() string { return GetNested(r.Object, "metadata", "name") }

// Namespace returns metadata.namespace.
func (r *Resource) Namespace() string { return GetNested(r.Object, "metadata", "namespace") }

// Kind returns the resource kind.
func (r *Resource) Kind() string { return GetNested(r.Object, "kind") }

// ReadResource reads a single Kubernetes resource from a YAML or JSON file.
func ReadResource(path string) (*Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading resource: %w", err)
	}
	return parseResource(data)
}

// ReadResourceList reads a file that may contain a single resource or a List
// and returns all resources found.
func ReadResourceList(path string) ([]*Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading resource list: %w", err)
	}
	return parseResourceList(data)
}

func parseResource(data []byte) (*Resource, error) {
	obj, err := decodeToMap(data)
	if err != nil {
		return nil, err
	}
	return &Resource{Object: obj}, nil
}

func parseResourceList(data []byte) ([]*Resource, error) {
	obj, err := decodeToMap(data)
	if err != nil {
		return nil, err
	}

	items, ok := obj["items"]
	if !ok {
		return []*Resource{{Object: obj}}, nil
	}

	itemList, ok := items.([]any)
	if !ok {
		return []*Resource{{Object: obj}}, nil
	}

	var result []*Resource
	for _, item := range itemList {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, &Resource{Object: m})
	}
	return result, nil
}

// decodeToMap parses YAML or JSON data into a map.
func decodeToMap(data []byte) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// Try JSON first (faster)
	if trimmed[0] == '{' || trimmed[0] == '[' {
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
		return obj, nil
	}

	// Fall back to YAML
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	return obj, nil
}

// WalkResources walks a directory and reads all YAML/JSON files as resources.
func WalkResources(dir string) ([]*Resource, error) {
	var resources []*Resource

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		objs, err := ReadResourceList(path)
		if err != nil {
			return nil
		}
		resources = append(resources, objs...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", dir, err)
	}
	return resources, nil
}

// FindResourceFiles searches for files matching a name pattern.
func FindResourceFiles(root, pattern string) ([]string, error) {
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return nil
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

// GetNested extracts a string from a nested map path.
func GetNested(obj map[string]any, fields ...string) string {
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

// GetNestedInt extracts an int64 from a nested map path.
func GetNestedInt(obj map[string]any, fields ...string) int64 {
	current := obj
	for i, f := range fields {
		val, ok := current[f]
		if !ok {
			return 0
		}
		if i == len(fields)-1 {
			switch v := val.(type) {
			case float64:
				return int64(v)
			case int:
				return int64(v)
			case int64:
				return v
			default:
				return 0
			}
		}
		next, ok := val.(map[string]any)
		if !ok {
			return 0
		}
		current = next
	}
	return 0
}

// GetNestedBool extracts a bool from a nested map path.
func GetNestedBool(obj map[string]any, fields ...string) bool {
	current := obj
	for i, f := range fields {
		val, ok := current[f]
		if !ok {
			return false
		}
		if i == len(fields)-1 {
			b, ok := val.(bool)
			return ok && b
		}
		next, ok := val.(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	return false
}

// GetNestedSlice extracts a slice from a nested map path.
func GetNestedSlice(obj map[string]any, fields ...string) []any {
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

// GetNestedMap extracts a sub-map from a nested map path.
func GetNestedMap(obj map[string]any, fields ...string) map[string]any {
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
