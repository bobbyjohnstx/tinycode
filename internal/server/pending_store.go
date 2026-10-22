package server

import (
	"sync"
	"time"
)

const pendingTTL = 10 * time.Minute

// PendingPermission represents a tool permission awaiting user approval.
type PendingPermission struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"sessionID"`
	Permission  string         `json:"permission,omitempty"`
	Patterns    []string       `json:"patterns,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Always      []string       `json:"always,omitempty"`
	Tool        string         `json:"tool,omitempty"`
	Description string         `json:"description,omitempty"`
	Args        any            `json:"args,omitempty"`
	createdAt   time.Time
}

// PermissionStore tracks pending permissions in memory.
type PermissionStore struct {
	mu       sync.RWMutex
	pending  map[string]PendingPermission
	onExpire func(PendingPermission)
}

func NewPermissionStore() *PermissionStore {
	return &PermissionStore{
		pending: make(map[string]PendingPermission),
	}
}

// SetOnExpire registers a callback invoked when TTL expiry removes an entry.
// The callback must reject the ask (RespondToAsk / bus) — never a silent drop.
func (ps *PermissionStore) SetOnExpire(fn func(PendingPermission)) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.onExpire = fn
}

func (ps *PermissionStore) Add(p PendingPermission) {
	ps.mu.Lock()
	p.createdAt = time.Now()
	ps.pending[p.ID] = p
	expired, onExpire := ps.cleanupLocked()
	ps.mu.Unlock()
	for _, e := range expired {
		if onExpire != nil {
			onExpire(e)
		}
	}
}

func (ps *PermissionStore) Get(id string) (PendingPermission, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.pending[id]
	return p, ok
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

// ExpireNow runs TTL cleanup (for tests / periodic sweep).
func (ps *PermissionStore) ExpireNow() {
	ps.mu.Lock()
	expired, onExpire := ps.cleanupLocked()
	ps.mu.Unlock()
	for _, e := range expired {
		if onExpire != nil {
			onExpire(e)
		}
	}
}

func (ps *PermissionStore) cleanupLocked() (expired []PendingPermission, onExpire func(PendingPermission)) {
	cutoff := time.Now().Add(-pendingTTL)
	onExpire = ps.onExpire
	for id, p := range ps.pending {
		if p.createdAt.Before(cutoff) {
			expired = append(expired, p)
			delete(ps.pending, id)
		}
	}
	return expired, onExpire
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
	mu       sync.RWMutex
	pending  map[string]PendingQuestion
	onExpire func(PendingQuestion)
}

func NewQuestionStore() *QuestionStore {
	return &QuestionStore{
		pending: make(map[string]PendingQuestion),
	}
}

// SetOnExpire registers a callback invoked when TTL expiry removes an entry.
func (qs *QuestionStore) SetOnExpire(fn func(PendingQuestion)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onExpire = fn
}

func (qs *QuestionStore) Add(q PendingQuestion) {
	qs.mu.Lock()
	q.createdAt = time.Now()
	qs.pending[q.ID] = q
	expired, onExpire := qs.cleanupLocked()
	qs.mu.Unlock()
	for _, e := range expired {
		if onExpire != nil {
			onExpire(e)
		}
	}
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

// ExpireNow runs TTL cleanup (for tests / periodic sweep).
func (qs *QuestionStore) ExpireNow() {
	qs.mu.Lock()
	expired, onExpire := qs.cleanupLocked()
	qs.mu.Unlock()
	for _, e := range expired {
		if onExpire != nil {
			onExpire(e)
		}
	}
}

func (qs *QuestionStore) cleanupLocked() (expired []PendingQuestion, onExpire func(PendingQuestion)) {
	cutoff := time.Now().Add(-pendingTTL)
	onExpire = qs.onExpire
	for id, q := range qs.pending {
		if q.createdAt.Before(cutoff) {
			expired = append(expired, q)
			delete(qs.pending, id)
		}
	}
	return expired, onExpire
}
