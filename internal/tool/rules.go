package tool

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// ScopedRule is a rule file with path globs that determine when it's attached
// to tool results.
type ScopedRule struct {
	Path    string   // display path
	Content string   // rule body
	Globs   []string // filepath.Match globs, matched against project-relative paths
}

// SafeScopedRules holds scoped rule files and tracks which have been attached.
// Safe for concurrent access.
type SafeScopedRules struct {
	mu       sync.Mutex
	rules    []ScopedRule
	attached map[string]bool // rule paths already attached
}

// NewSafeScopedRules creates a SafeScopedRules from the given rules.
func NewSafeScopedRules(rules []ScopedRule) *SafeScopedRules {
	return &SafeScopedRules{
		rules:    rules,
		attached: make(map[string]bool),
	}
}

// MatchAndAttach checks if the given absolute file path matches any pending
// scoped rule's globs. projectDir is the session root used to compute the
// relative path for glob matching. Returns the formatted rule text to prepend
// to tool output, or "" if no rules match.
//
// Globs are matched per path segment using filepath.Match against the
// project-relative path. Each rule is attached at most once per session.
func (s *SafeScopedRules) MatchAndAttach(absPath, projectDir string) string {
	if s == nil {
		return ""
	}

	// Resolve symlinks in projectDir so it matches the resolved absPath
	// (e.g. /tmp vs /private/tmp on macOS).
	resolvedDir := projectDir
	if resolvedDir != "" {
		if r, err := filepath.EvalSymlinks(resolvedDir); err == nil {
			resolvedDir = r
		}
	}

	relPath := absPath
	if resolvedDir != "" {
		if r, err := filepath.Rel(resolvedDir, absPath); err == nil && !strings.HasPrefix(r, "..") {
			relPath = r
		} else {
			return "" // outside project
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var parts []string
	for _, rule := range s.rules {
		if s.attached[rule.Path] {
			continue
		}
		if matchesAnyGlob(relPath, rule.Globs) {
			s.attached[rule.Path] = true
			parts = append(parts, fmt.Sprintf("[Rule from %s]\n%s", rule.Path, rule.Content))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n\n---\n\n"
}

// matchesAnyGlob reports whether relPath matches any of the given globs.
// Each glob is matched using filepath.Match against the full relative path.
func matchesAnyGlob(relPath string, globs []string) bool {
	for _, glob := range globs {
		if matched, _ := filepath.Match(glob, relPath); matched {
			return true
		}
		// Also match against just the base name for simple globs like "*.go".
		if matched, _ := filepath.Match(glob, filepath.Base(relPath)); matched {
			return true
		}
	}
	return false
}
