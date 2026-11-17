package tool

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

const (
	spillSubdir   = "tool-output"
	spillMaxAge   = 7 * 24 * time.Hour
	spillMaxFiles = 100
)

// spillDir returns the spill directory path under DataDir().
func spillDir() string {
	return filepath.Join(config.DataDir(), spillSubdir)
}

// SpillIfTruncated writes the full output to a retained file when truncation
// occurred, and appends a path hint to the preview. If not truncated, returns
// the content unchanged.
func SpillIfTruncated(sessionID, fullOutput string, tr *TruncateResult) string {
	if !tr.Truncated {
		return tr.Content
	}

	dir := spillDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("spill: mkdir failed", "dir", dir, "error", err)
		return tr.Content
	}

	filename := fmt.Sprintf("%s-%d.txt", sessionID, time.Now().UnixNano())
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, []byte(fullOutput), 0o644); err != nil {
		slog.Error("spill: write failed", "path", path, "error", err)
		return tr.Content
	}

	slog.Info("spill: wrote tool output", "path", path, "bytes", len(fullOutput))

	// Best-effort cleanup in the background; errors are logged, never fatal.
	cleanupSpillDir(dir)

	return tr.Content + fmt.Sprintf("\n\n[Full output saved to %s. Use `read` to see more.]", path)
}

// cleanupSpillDir removes files older than spillMaxAge and trims total count
// to spillMaxFiles. Errors are logged but never returned.
func cleanupSpillDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Error("spill: cleanup readdir failed", "dir", dir, "error", err)
		return
	}

	type fileAge struct {
		path    string
		modTime time.Time
	}

	now := time.Now()
	var files []fileAge
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f := fileAge{path: filepath.Join(dir, e.Name()), modTime: info.ModTime()}

		// Remove files older than spillMaxAge.
		if now.Sub(f.modTime) > spillMaxAge {
			if rerr := os.Remove(f.path); rerr != nil {
				slog.Error("spill: cleanup remove failed", "path", f.path, "error", rerr)
			}
			continue
		}
		files = append(files, f)
	}

	// If still over the cap, remove oldest first.
	if len(files) > spillMaxFiles {
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.Before(files[j].modTime)
		})
		excess := len(files) - spillMaxFiles
		for i := 0; i < excess; i++ {
			if rerr := os.Remove(files[i].path); rerr != nil {
				slog.Error("spill: cleanup remove failed", "path", files[i].path, "error", rerr)
			}
		}
	}
}
