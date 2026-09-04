package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode-go/internal/config"
)

// ResolveBinary locates the plugin binary for the given name. It checks, in
// order:
//  1. ~/.config/tinycode/plugins/<name>
//  2. PATH for tinycode-plugin-<name>
//  3. The built-in registry
//
// Returns the absolute path to the binary or an error if not found.
func ResolveBinary(name string) (string, error) {
	// Check config plugins directory.
	configPath := filepath.Join(config.ConfigDir(), "plugins", name)
	if abs, err := filepath.Abs(configPath); err == nil {
		if isExecutable(abs) {
			return abs, nil
		}
	}

	// Check PATH.
	pathName := "tinycode-plugin-" + name
	if p, err := exec.LookPath(pathName); err == nil {
		abs, err := filepath.Abs(p)
		if err != nil {
			return p, nil
		}
		return abs, nil
	}

	// Check registry.
	if entry, ok := LookupRegistry(name); ok {
		return "", fmt.Errorf("plugin %q found in registry (%s) but not installed — install from %s", name, entry.Description, entry.Repo)
	}

	return "", fmt.Errorf("plugin %q not found in config dir, PATH, or registry", name)
}

// isExecutable checks whether a file exists and is executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&0o111 != 0
}
