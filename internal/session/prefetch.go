package session

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// defaultPrefetchMaxBytes is the total size cap when prefetch_max_bytes
	// is not specified in the agent frontmatter.
	defaultPrefetchMaxBytes = 100 * 1024 // 100KB

	// maxSingleFileBytes caps individual file size during prefetch.
	// Files larger than this are skipped regardless of the total budget.
	maxSingleFileBytes = 50 * 1024 // 50KB
)

// PrefetchFiles walks directory matching files against the given glob pattern
// and returns their contents formatted for injection into the system prompt.
// pattern is expected to be a brace-expansion glob like "**/*.{go,py,js}".
// When the total size of matched files exceeds maxBytes, it returns empty
// results (prefetch is skipped). Binary files and files > 50KB are skipped.
// Returns the formatted text and the list of absolute paths that were included.
func PrefetchFiles(directory, pattern string, maxBytes int) (string, []string) {
	if directory == "" || pattern == "" {
		return "", nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultPrefetchMaxBytes
	}

	exts := parseExtensions(pattern)
	if len(exts) == 0 {
		return "", nil
	}

	type fileEntry struct {
		absPath string
		relPath string
		content string
	}

	var entries []fileEntry
	var totalBytes int

	_ = filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			name := d.Name()
			// Skip hidden directories and common non-source directories.
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if !exts[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := info.Size()
		if size == 0 || size > maxSingleFileBytes {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Skip binary files.
		if !utf8.Valid(data) {
			return nil
		}

		totalBytes += len(data)
		if totalBytes > maxBytes {
			// Signal early termination — total exceeded.
			return fmt.Errorf("prefetch size exceeded")
		}

		rel, _ := filepath.Rel(directory, path)
		if rel == "" {
			rel = path
		}

		entries = append(entries, fileEntry{
			absPath: path,
			relPath: rel,
			content: string(data),
		})

		return nil
	})

	// If total exceeded the budget, skip prefetch entirely.
	if totalBytes > maxBytes {
		return "", nil
	}

	if len(entries) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Project files (prefetched)\n\n")

	var paths []string
	for _, e := range entries {
		lang := langFromExt(filepath.Ext(e.relPath))
		fmt.Fprintf(&sb, "### %s\n```%s\n%s\n```\n\n", e.relPath, lang, strings.TrimRight(e.content, "\n"))
		paths = append(paths, e.absPath)
	}

	return sb.String(), paths
}

// parseExtensions extracts file extensions from a glob pattern like
// "**/*.{go,py,js}" or "**/*.go". Returns a set of extensions including
// the leading dot.
func parseExtensions(pattern string) map[string]bool {
	exts := make(map[string]bool)

	// Handle brace expansion: **/*.{go,py,js}
	if idx := strings.Index(pattern, "{"); idx >= 0 {
		end := strings.Index(pattern[idx:], "}")
		if end > 0 {
			inner := pattern[idx+1 : idx+end]
			for _, ext := range strings.Split(inner, ",") {
				ext = strings.TrimSpace(ext)
				if ext != "" {
					if !strings.HasPrefix(ext, ".") {
						ext = "." + ext
					}
					exts[ext] = true
				}
			}
			return exts
		}
	}

	// Handle simple pattern: **/*.go
	if idx := strings.LastIndex(pattern, "."); idx >= 0 {
		ext := pattern[idx:]
		if ext != ".*" && ext != "." {
			exts[ext] = true
		}
	}

	return exts
}

// langFromExt returns a markdown code-fence language hint for common extensions.
func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".jsx":
		return "jsx"
	case ".tsx":
		return "tsx"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".sh", ".bash":
		return "bash"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".sql":
		return "sql"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp":
		return "cpp"
	case ".rs":
		return "rust"
	default:
		return ""
	}
}
