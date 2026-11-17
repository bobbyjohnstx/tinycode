package session

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/frontmatter"
)

// RuleFile represents a discovered rule from .tinycode/rules/*.md or
// ~/.config/tinycode/rules/*.md. Rules without Globs are unscoped and
// included in the system prompt. Rules with Globs are scoped and attached
// to tool results when a file path matches.
type RuleFile struct {
	Path    string   // display path (relative to project root when possible)
	Content string   // body after frontmatter
	Globs   []string // path globs from frontmatter; empty = unscoped
}

// DiscoverRules scans .tinycode/rules/*.md in the project directory and
// optionally ~/.config/tinycode/rules/*.md for user-level rules. Each file
// is parsed for optional frontmatter with a "paths" field (list of globs).
// Results are sorted alphabetically by filename within each source.
// Files that cannot be read are silently skipped.
func DiscoverRules(projectDir, configDir string) []RuleFile {
	var rules []RuleFile

	// User-level rules (loaded first, outermost)
	if configDir != "" {
		userRulesDir := filepath.Join(configDir, "rules")
		rules = append(rules, scanRulesDir(userRulesDir, projectDir)...)
	}

	// Project-level rules
	if projectDir != "" {
		projectRulesDir := filepath.Join(projectDir, ".tinycode", "rules")
		rules = append(rules, scanRulesDir(projectRulesDir, projectDir)...)
	}

	return rules
}

// scanRulesDir reads *.md files from dir, parses frontmatter, and returns
// RuleFile entries sorted by filename.
func scanRulesDir(dir, projectDir string) []RuleFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	// Sort entries by name for deterministic order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var rules []RuleFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read rule file", "path", path, "error", err)
			continue
		}

		content := string(data)
		fm, body := frontmatter.Parse(content)
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}

		displayPath := path
		if projectDir != "" {
			if rel, err := filepath.Rel(projectDir, path); err == nil {
				displayPath = rel
			}
		}

		rule := RuleFile{
			Path:    displayPath,
			Content: body,
		}

		// Parse paths globs from frontmatter.
		if fm != nil {
			rule.Globs = parsePathsGlobs(fm)
		}

		rules = append(rules, rule)
	}

	return rules
}

// parsePathsGlobs extracts glob patterns from the frontmatter "paths" field.
// Supports both a list ([]string) and a single string value.
func parsePathsGlobs(fm map[string]any) []string {
	val, ok := fm["paths"]
	if !ok {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return v
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}
