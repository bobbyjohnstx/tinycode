package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// TrustEntry represents the trust state of a project hook.
type TrustEntry struct {
	Trusted  bool   `json:"trusted"`
	Disabled bool   `json:"disabled,omitempty"`
	Command  string `json:"command,omitempty"`
	Event    string `json:"event,omitempty"`
}

// HookTrustStore manages trust state for project shell hooks.
// Trust is keyed by SHA-256 hash of (command + event + project directory),
// so changing any of those invalidates previous trust.
type HookTrustStore struct {
	mu      sync.RWMutex
	entries map[string]TrustEntry
	path    string
	warned  map[string]bool
}

// NewHookTrustStore creates a trust store backed by hook-trust.json in dataDir.
func NewHookTrustStore(dataDir string) *HookTrustStore {
	path := filepath.Join(dataDir, "hook-trust.json")
	store := &HookTrustStore{
		entries: make(map[string]TrustEntry),
		path:    path,
		warned:  make(map[string]bool),
	}
	store.load()
	return store
}

// HookHash returns the SHA-256 hex digest of (command + event + projectDir).
func HookHash(command, eventName, projectDir string) string {
	h := sha256.New()
	h.Write([]byte(command))
	h.Write([]byte(eventName))
	h.Write([]byte(projectDir))
	return hex.EncodeToString(h.Sum(nil))
}

// AgentHookHash returns the SHA-256 hex digest of (command + event + projectDir + agentName).
// Including the agent name ensures that editing the agent file invalidates trust.
func AgentHookHash(command, eventName, projectDir, agentName string) string {
	h := sha256.New()
	h.Write([]byte(command))
	h.Write([]byte(eventName))
	h.Write([]byte(projectDir))
	h.Write([]byte(agentName))
	return hex.EncodeToString(h.Sum(nil))
}

// IsTrusted reports whether the hook identified by (command, event, projectDir)
// has been explicitly trusted by the user.
func (s *HookTrustStore) IsTrusted(command, eventName, projectDir string) bool {
	hash := HookHash(command, eventName, projectDir)
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[hash]
	return ok && entry.Trusted
}

// IsDisabled reports whether the hook has been explicitly disabled.
func (s *HookTrustStore) IsDisabled(command, eventName, projectDir string) bool {
	hash := HookHash(command, eventName, projectDir)
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[hash]
	return ok && entry.Disabled
}

// IsDisabledAgent reports whether an agent-scoped hook has been explicitly disabled.
func (s *HookTrustStore) IsDisabledAgent(command, eventName, projectDir, agentName string) bool {
	hash := AgentHookHash(command, eventName, projectDir, agentName)
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[hash]
	return ok && entry.Disabled
}

// Trust marks the hook as trusted and persists the change.
func (s *HookTrustStore) Trust(command, eventName, projectDir string) error {
	hash := HookHash(command, eventName, projectDir)
	s.mu.Lock()
	s.entries[hash] = TrustEntry{Trusted: true, Command: command, Event: eventName}
	s.mu.Unlock()
	return s.save()
}

// Disable marks the hook as disabled and persists the change.
func (s *HookTrustStore) Disable(command, eventName, projectDir string) error {
	hash := HookHash(command, eventName, projectDir)
	s.mu.Lock()
	entry := s.entries[hash]
	entry.Disabled = true
	entry.Command = command
	entry.Event = eventName
	s.entries[hash] = entry
	s.mu.Unlock()
	return s.save()
}

// Status returns "trusted", "disabled", or "untrusted" for the given hook.
func (s *HookTrustStore) Status(command, eventName, projectDir string) string {
	hash := HookHash(command, eventName, projectDir)
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[hash]
	if !ok {
		return "untrusted"
	}
	if entry.Disabled {
		return "disabled"
	}
	if entry.Trusted {
		return "trusted"
	}
	return "untrusted"
}

// WarnOnce logs a warning the first time an untrusted hook is skipped.
// Subsequent calls for the same hash are silent.
func (s *HookTrustStore) WarnOnce(command, eventName, projectDir string, logger *slog.Logger) {
	hash := HookHash(command, eventName, projectDir)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.warned[hash] {
		return
	}
	s.warned[hash] = true
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("skipping untrusted project hook",
		"event", eventName,
		"command", command,
		"project", projectDir,
	)
}

func (s *HookTrustStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.entries)
}

func (s *HookTrustStore) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.entries, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshaling trust store: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating trust store dir: %w", err)
	}
	return os.WriteFile(s.path, data, 0o644)
}
