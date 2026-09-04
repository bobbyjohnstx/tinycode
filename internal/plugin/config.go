package plugin

import (
	"encoding/json"
	"fmt"
)

// PluginSpec describes a plugin to load, parsed from the config's "plugins"
// array. Entries can be a bare string ("notify") or a tuple ["name", {opts}].
type PluginSpec struct {
	Name    string
	Options map[string]any
}

// UnmarshalJSON handles both string and tuple forms:
//
//	"notify"                     -> PluginSpec{Name: "notify"}
//	["code-review", {"auto": true}] -> PluginSpec{Name: "code-review", Options: {"auto": true}}
func (ps *PluginSpec) UnmarshalJSON(data []byte) error {
	// Try string first.
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		ps.Name = name
		return nil
	}

	// Try tuple [name, options].
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return fmt.Errorf("plugin spec must be a string or [name, options] tuple: %w", err)
	}
	if len(tuple) < 1 || len(tuple) > 2 {
		return fmt.Errorf("plugin spec tuple must have 1 or 2 elements, got %d", len(tuple))
	}

	if err := json.Unmarshal(tuple[0], &ps.Name); err != nil {
		return fmt.Errorf("plugin spec name must be a string: %w", err)
	}
	if len(tuple) == 2 {
		ps.Options = make(map[string]any)
		if err := json.Unmarshal(tuple[1], &ps.Options); err != nil {
			return fmt.Errorf("plugin spec options must be an object: %w", err)
		}
	}

	return nil
}

// ParsePluginConfig parses a list of raw JSON plugin entries into PluginSpecs.
func ParsePluginConfig(raw []json.RawMessage) ([]PluginSpec, error) {
	specs := make([]PluginSpec, 0, len(raw))
	for i, entry := range raw {
		var spec PluginSpec
		if err := json.Unmarshal(entry, &spec); err != nil {
			return nil, fmt.Errorf("plugin[%d]: %w", i, err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}
