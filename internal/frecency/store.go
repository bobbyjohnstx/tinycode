package frecency

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	maxTimestamps = 100
	maxEntries    = 500
)

// Entry tracks visit history for a single item.
type Entry struct {
	Visits     int     `json:"visits"`
	Timestamps []int64 `json:"timestamps"` // unix milliseconds, max 100
}

// Store tracks frecency (frequency + recency) scores for items.
// It is safe for concurrent use.
type Store struct {
	mu    sync.RWMutex
	data  map[string]Entry
	path  string
	dirty bool
}

// New creates a new Store that persists to the given path.
func New(path string) *Store {
	return &Store{
		data: make(map[string]Entry),
		path: path,
	}
}

// Record adds a visit for the given item at the current time.
func (s *Store) Record(item string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := s.data[item]
	e.Visits++
	e.Timestamps = append(e.Timestamps, time.Now().UnixMilli())
	if len(e.Timestamps) > maxTimestamps {
		e.Timestamps = e.Timestamps[len(e.Timestamps)-maxTimestamps:]
	}
	s.data[item] = e
	s.dirty = true
}

// Score computes the frecency score for an item. Returns 0 for unknown items.
func (s *Store) Score(item string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.data[item]
	if !ok {
		return 0
	}
	now := time.Now().UnixMilli()
	var score float64
	for _, ts := range entry.Timestamps {
		age := now - ts
		switch {
		case age < 4*60*60*1000: // 4 hours
			score += 100
		case age < 24*60*60*1000: // 1 day
			score += 80
		case age < 7*24*60*60*1000: // 1 week
			score += 60
		case age < 30*24*60*60*1000: // 1 month
			score += 30
		default:
			score += 10
		}
	}
	return score
}

// Sort returns a copy of items sorted by descending frecency score.
// Items with no frecency history appear at the end in their original order.
func (s *Store) Sort(items []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UnixMilli()
	scores := make(map[string]float64, len(items))
	for _, item := range items {
		scores[item] = s.scoreLocked(item, now)
	}

	result := make([]string, len(items))
	copy(result, items)

	sort.SliceStable(result, func(i, j int) bool {
		si, sj := scores[result[i]], scores[result[j]]
		return si > sj
	})
	return result
}

// scoreLocked computes the score without locking. Caller must hold at least RLock.
func (s *Store) scoreLocked(item string, now int64) float64 {
	entry, ok := s.data[item]
	if !ok {
		return 0
	}
	var score float64
	for _, ts := range entry.Timestamps {
		age := now - ts
		switch {
		case age < 4*60*60*1000:
			score += 100
		case age < 24*60*60*1000:
			score += 80
		case age < 7*24*60*60*1000:
			score += 60
		case age < 30*24*60*60*1000:
			score += 30
		default:
			score += 10
		}
	}
	return score
}

// Load reads frecency data from disk. Returns nil if the file does not exist.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	data := make(map[string]Entry)
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	s.data = data
	s.dirty = false
	return nil
}

// Save writes frecency data to disk. It prunes to maxEntries before saving.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty {
		return nil
	}

	s.pruneLocked()

	raw, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// Shutdown performs a final save of dirty data.
func (s *Store) Shutdown() {
	_ = s.Save()
}

// pruneLocked removes the lowest-scoring entries when over maxEntries.
// Caller must hold write lock.
func (s *Store) pruneLocked() {
	if len(s.data) <= maxEntries {
		return
	}

	now := time.Now().UnixMilli()
	type scored struct {
		key   string
		score float64
	}
	all := make([]scored, 0, len(s.data))
	for k := range s.data {
		all = append(all, scored{key: k, score: s.scoreLocked(k, now)})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].score > all[j].score
	})
	// Keep top maxEntries, delete the rest.
	for _, item := range all[maxEntries:] {
		delete(s.data, item.key)
	}
}
