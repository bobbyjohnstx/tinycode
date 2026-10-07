package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveMCPConfigPath returns the config file path that MCP CLI writes should
// target. When project is false, prefers an existing global config file; if
// none exists, returns ConfigDir()/tinycode.json. When project is true, prefers
// the innermost existing project config under directory; if none exists,
// returns directory/.tinycode/tinycode.json.
func ResolveMCPConfigPath(project bool, directory string) string {
	if project {
		if directory == "" {
			directory, _ = os.Getwd()
		}
		paths := projectConfigPaths(directory)
		if len(paths) > 0 {
			return paths[len(paths)-1]
		}
		return filepath.Join(directory, ".tinycode", "tinycode.json")
	}

	dir := ConfigDir()
	for _, name := range []string{"tinycode.jsonc", "tinycode.json", "config.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, "tinycode.json")
}

// WriteMCPServer loads path (creating an empty object if missing), sets
// mcp[name] to cfg, and writes indented JSON. Comments in JSONC files are not
// preserved on write.
func WriteMCPServer(path, name string, cfg MCPConfig) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("mcp server name is required")
	}

	root, err := loadConfigMap(path)
	if err != nil {
		return err
	}

	entry, err := mcpConfigToMap(cfg)
	if err != nil {
		return err
	}

	mcp := mcpMap(root)
	mcp[name] = entry
	root["mcp"] = mcp

	return writeConfigMap(path, root)
}

// UpdateMCPServerHeaders sets or clears Authorization (and merges other
// headers) on an existing mcp[name] entry. Missing servers return an error.
// When clearAuth is true, Authorization is removed. When authValue is
// non-empty, Authorization is set to that value.
func UpdateMCPServerHeaders(path, name string, headers map[string]string, authValue string, clearAuth bool) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("mcp server name is required")
	}

	root, err := loadConfigMap(path)
	if err != nil {
		return err
	}

	mcp := mcpMap(root)
	raw, ok := mcp[name]
	if !ok {
		return fmt.Errorf("mcp server %q not found in %s", name, path)
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("mcp server %q has invalid config shape", name)
	}

	h, _ := entry["headers"].(map[string]any)
	if h == nil {
		h = map[string]any{}
	}
	for k, v := range headers {
		h[k] = v
	}
	if clearAuth {
		delete(h, "Authorization")
	} else if authValue != "" {
		h["Authorization"] = authValue
	}
	if len(h) == 0 {
		delete(entry, "headers")
	} else {
		entry["headers"] = h
	}
	mcp[name] = entry
	root["mcp"] = mcp

	return writeConfigMap(path, root)
}

func loadConfigMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}

	jsonStr, err := ParseJSONC(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing JSONC %s: %w", path, err)
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &root); err != nil {
		return nil, fmt.Errorf("parsing config JSON %s: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}

func writeConfigMap(path string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}
	return nil
}

func mcpMap(root map[string]any) map[string]any {
	raw, ok := root["mcp"]
	if !ok || raw == nil {
		return map[string]any{}
	}
	switch m := raw.(type) {
	case map[string]any:
		return m
	default:
		return map[string]any{}
	}
}

func mcpConfigToMap(cfg MCPConfig) (map[string]any, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling mcp config: %w", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("encoding mcp config: %w", err)
	}
	if entry == nil {
		entry = map[string]any{}
	}
	return entry, nil
}
