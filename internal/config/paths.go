package config

import (
	"os"
	"path/filepath"
)

func ConfigDir() string {
	return configDirs()[0]
}

// configDirs returns config directories in priority order.
// TINYCODE_CONFIG_DIR takes precedence, then XDG_CONFIG_HOME,
// then the default ~/.config/tinycode.
func configDirs() []string {
	if v := os.Getenv("TINYCODE_CONFIG_DIR"); v != "" {
		return []string{v}
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return []string{filepath.Join(v, "tinycode")}
	}
	home, _ := os.UserHomeDir()
	return []string{filepath.Join(home, ".config", "tinycode")}
}

// DataDir returns the data directory for tinycode.
// TINYCODE_DATA_DIR takes precedence, then XDG_DATA_HOME,
// then the default ~/.local/share/tinycode.
func DataDir() string {
	if v := os.Getenv("TINYCODE_DATA_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "tinycode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "tinycode")
}

func GlobalConfigFile() string {
	dir := ConfigDir()
	candidates := []string{
		filepath.Join(dir, "tinycode.jsonc"),
		filepath.Join(dir, "tinycode.json"),
		filepath.Join(dir, "config.json"),
	}
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	return candidates[0]
}

func ProjectConfigFiles(name, directory string) []string {
	var files []string
	dir := directory
	for {
		for _, ext := range []string{".jsonc", ".json"} {
			candidate := filepath.Join(dir, name+ext)
			if _, err := os.Stat(candidate); err == nil {
				files = append(files, candidate)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Reverse so innermost (closest to project) wins in merge order
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}
	return files
}

func ProjectDotDirs(directory string) []string {
	var dirs []string
	dir := directory
	for {
		candidate := filepath.Join(dir, ".tinycode")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			dirs = append(dirs, candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Reverse: outermost first
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}
