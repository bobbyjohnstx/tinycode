package server

import (
	"sync"
	"time"
)

const pendingTTL = 10 * time.Minute

// PendingPermission represents a tool permission awaiting user approval.
type PendingPermission struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"sessionID"`
	Tool        string    `json:"tool"`
	Description string    `json:"description"`
	Args        any       `json:"args,omitempty"`
	createdAt   time.Time
}

// PermissionStore tracks pending permissions in memory.
type PermissionStore struct {
	mu      sync.RWMutex
	pending map[string]PendingPermission
}

func NewPermissionStore() *PermissionStore {
	return &PermissionStore{
		pending: make(map[string]PendingPermission),
	}
}

func (ps *PermissionStore) Add(p PendingPermission) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p.createdAt = time.Now()
	ps.pending[p.ID] = p
	ps.cleanupLocked()
}

func (ps *PermissionStore) Remove(id string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.pending, id)
}

func (ps *PermissionStore) List() []PendingPermission {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	result := make([]PendingPermission, 0, len(ps.pending))
	for _, p := range ps.pending {
		result = append(result, p)
	}
	return result
}

func (ps *PermissionStore) cleanupLocked() {
	cutoff := time.Now().Add(-pendingTTL)
	for id, p := range ps.pending {
		if p.createdAt.Before(cutoff) {
			delete(ps.pending, id)
		}
	}
}

// PendingQuestion represents a question awaiting user input.
type PendingQuestion struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionID"`
	Question  string    `json:"question"`
	Options   []string  `json:"options,omitempty"`
	createdAt time.Time
}

// QuestionStore tracks pending questions in memory.
type QuestionStore struct {
	mu      sync.RWMutex
	pending map[string]PendingQuestion
}

func NewQuestionStore() *QuestionStore {
	return &QuestionStore{
		pending: make(map[string]PendingQuestion),
	}
}

func (qs *QuestionStore) Add(q PendingQuestion) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	q.createdAt = time.Now()
	qs.pending[q.ID] = q
	qs.cleanupLocked()
}

func (qs *QuestionStore) Get(id string) (PendingQuestion, bool) {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	q, ok := qs.pending[id]
	return q, ok
}

func (qs *QuestionStore) Remove(id string) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	delete(qs.pending, id)
}

func (qs *QuestionStore) List() []PendingQuestion {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	result := make([]PendingQuestion, 0, len(qs.pending))
	for _, q := range qs.pending {
		result = append(result, q)
	}
	return result
}

func (qs *QuestionStore) cleanupLocked() {
	cutoff := time.Now().Add(-pendingTTL)
	for id, q := range qs.pending {
		if q.createdAt.Before(cutoff) {
			delete(qs.pending, id)
		}
	}
}
