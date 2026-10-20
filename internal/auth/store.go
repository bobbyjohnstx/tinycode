// Package auth persists provider credentials to DataDir()/auth.json (mode 0600).
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

// Store is a concurrency-safe provider credential file backed by auth.json.
type Store struct {
	mu    sync.RWMutex
	path  string
	creds map[string]map[string]string // providerID -> key/value pairs
}

// DefaultPath returns DataDir()/auth.json.
func DefaultPath() string {
	return filepath.Join(config.DataDir(), "auth.json")
}

// Open loads credentials from path. A missing file yields an empty store.
// Corrupt JSON yields an empty store and a non-nil error (caller may ignore).
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	s := &Store{
		path:  path,
		creds: make(map[string]map[string]string),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("reading auth store: %w", err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.creds); err != nil {
		s.creds = make(map[string]map[string]string)
		return s, fmt.Errorf("parsing auth store: %w", err)
	}
	if s.creds == nil {
		s.creds = make(map[string]map[string]string)
	}
	return s, nil
}

// Path returns the backing file path.
func (s *Store) Path() string {
	return s.path
}

// Get returns a copy of credentials for providerID.
func (s *Store) Get(providerID string) (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.creds[providerID]
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out, true
}

// APIKey returns the apiKey/api_key for providerID, or "".
func (s *Store) APIKey(providerID string) string {
	creds, ok := s.Get(providerID)
	if !ok {
		return ""
	}
	return KeyFromCreds(creds)
}

// KeyFromCreds extracts apiKey or api_key from a credential map.
func KeyFromCreds(creds map[string]string) string {
	if creds == nil {
		return ""
	}
	if k := creds["apiKey"]; k != "" {
		return k
	}
	return creds["api_key"]
}

// Set stores credentials for providerID and persists to disk.
func (s *Store) Set(providerID string, creds map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]string, len(creds))
	for k, v := range creds {
		cp[k] = v
	}
	s.creds[providerID] = cp
	return s.persistLocked()
}

// Delete removes credentials for providerID and persists to disk.
func (s *Store) Delete(providerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.creds, providerID)
	return s.persistLocked()
}

// All returns a deep copy of all credentials.
func (s *Store) All() map[string]map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]map[string]string, len(s.creds))
	for id, c := range s.creds {
		cp := make(map[string]string, len(c))
		for k, v := range c {
			cp[k] = v
		}
		out[id] = cp
	}
	return out
}

func (s *Store) persistLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating auth dir: %w", err)
	}
	data, err := json.MarshalIndent(s.creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding auth store: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".auth-*.tmp")
	if err != nil {
		return fmt.Errorf("creating auth temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing auth temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing auth temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod auth temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming auth store: %w", err)
	}
	return nil
}

// ResolveAPIKey returns the first non-empty credential:
// env (envVars) → options apiKey/api_key → store.
func ResolveAPIKey(envVars []string, options map[string]any, store *Store, providerID string) string {
	for _, name := range envVars {
		if name == "" {
			continue
		}
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	if options != nil {
		if k, ok := options["apiKey"].(string); ok && k != "" {
			return k
		}
		if k, ok := options["api_key"].(string); ok && k != "" {
			return k
		}
	}
	if store != nil {
		return store.APIKey(providerID)
	}
	return ""
}
