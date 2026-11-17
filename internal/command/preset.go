package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/skill"
)

// PresetStore manages prompt presets saved as a JSON map in the user config directory.
type PresetStore struct {
	path string
}

// NewPresetStore creates a store that reads/writes presets from the user config dir.
func NewPresetStore() *PresetStore {
	return &PresetStore{
		path: filepath.Join(config.ConfigDir(), "presets.json"),
	}
}

// NewPresetStoreAt creates a store with an explicit path (for testing).
func NewPresetStoreAt(path string) *PresetStore {
	return &PresetStore{path: path}
}

func (s *PresetStore) load() (map[string]string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}
	var presets map[string]string
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, fmt.Errorf("invalid presets file: %w", err)
	}
	return presets, nil
}

func (s *PresetStore) save(presets map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// List returns sorted preset names.
func (s *PresetStore) List() ([]string, error) {
	presets, err := s.load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Get returns the body for a preset name, or empty string if not found.
func (s *PresetStore) Get(name string) (string, bool) {
	presets, err := s.load()
	if err != nil {
		return "", false
	}
	body, ok := presets[name]
	return body, ok
}

// Save stores a preset. Returns an error if the name clashes with a builtin
// command or discovered skill.
func (s *PresetStore) Save(name, body, configDir, projectDir string) error {
	if name == "" {
		return fmt.Errorf("preset name cannot be empty")
	}
	if isReservedName(name, configDir, projectDir) {
		return fmt.Errorf("name %q conflicts with a builtin command or skill", name)
	}
	presets, err := s.load()
	if err != nil {
		return err
	}
	presets[name] = body
	return s.save(presets)
}

// Delete removes a preset by name.
func (s *PresetStore) Delete(name string) error {
	presets, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := presets[name]; !ok {
		return fmt.Errorf("preset %q not found", name)
	}
	delete(presets, name)
	return s.save(presets)
}

// builtinNames are command names that presets must not shadow.
var builtinNames = map[string]bool{
	"swarm": true, "work-loop": true, "goal": true,
	"debug": true, "trace": true, "plan": true,
	"verify": true, "test": true, "review": true,
	"exit": true, "compact": true, "connect": true,
	"theme": true, "export": true, "export-html": true,
	"copy": true, "auto-approve": true, "clear-queue": true,
	"help": true, "editor": true, "shell": true,
	"paste-image": true, "diagnostics": true, "privacy": true,
	"mcp": true, "scoped-models": true, "undo": true, "redo": true,
	"diff": true, "changes": true, "archive": true, "branch": true,
	"btw": true, "effort": true, "rename": true, "thinking": true,
	"rewind": true, "hooks": true, "context": true,
	"markdown-style": true, "verbose": true, "warnings": true,
	"preset": true, "init": true,
}

// isReservedName checks if a name clashes with builtins or discovered skills.
func isReservedName(name, configDir, projectDir string) bool {
	if builtinNames[name] {
		return true
	}
	if _, ok := delegateAgents[name]; ok {
		return true
	}
	for _, s := range skill.DiscoverWithPaths(configDir, projectDir, nil) {
		if s.Name == name || s.ID == name {
			return true
		}
	}
	return false
}

// ExpandPreset checks if a slash command matches a saved preset and returns
// the expanded text. suffix is any text after the preset name, appended with
// a newline. Returns ("", false) if no preset matches.
func ExpandPreset(name, suffix, presetPath string) (string, bool) {
	store := &PresetStore{path: presetPath}
	body, ok := store.Get(name)
	if !ok {
		return "", false
	}
	suffix = strings.TrimSpace(suffix)
	if suffix != "" {
		return body + "\n" + suffix, true
	}
	return body, true
}
