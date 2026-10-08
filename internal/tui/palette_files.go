package tui

import (
	"os"
	"path/filepath"
	"strings"
)

// buildFilePaletteItems walks the cwd and returns PaletteItems for files,
// respecting depth and count limits and skipping common large directories.
func buildFilePaletteItems(cwd string) []PaletteItem {
	const maxDepth = 3
	const maxFiles = 100

	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		".next":        true,
		"__pycache__":  true,
		".cache":       true,
	}

	var items []PaletteItem

	_ = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		rel, relErr := filepath.Rel(cwd, path)
		if relErr != nil {
			return nil
		}

		// Skip root itself.
		if rel == "." {
			return nil
		}

		// Calculate depth.
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden directories and known large directories.
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		items = append(items, PaletteItem{
			Label:    rel,
			Category: "file",
			Value:    "@" + rel,
		})

		if len(items) >= maxFiles {
			return filepath.SkipAll
		}

		return nil
	})

	return items
}
