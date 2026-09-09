package tool

import (
	"sort"
	"testing"
)

func TestGrepResultsSortByModTime(t *testing.T) {
	type fileResult struct {
		modTime int64
		matches []string
	}

	results := []fileResult{
		{modTime: 100, matches: []string{"old.go:1:match"}},
		{modTime: 300, matches: []string{"newest.go:1:match"}},
		{modTime: 200, matches: []string{"middle.go:1:match"}},
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].modTime > results[j].modTime
	})

	if results[0].matches[0] != "newest.go:1:match" {
		t.Errorf("expected newest first, got %s", results[0].matches[0])
	}
	if results[1].matches[0] != "middle.go:1:match" {
		t.Errorf("expected middle second, got %s", results[1].matches[0])
	}
	if results[2].matches[0] != "old.go:1:match" {
		t.Errorf("expected oldest last, got %s", results[2].matches[0])
	}
}
