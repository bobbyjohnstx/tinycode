package permission

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/id"
)

var (
	ErrDenied    = errors.New("permission denied by rule")
	ErrRejected  = errors.New("user rejected permission")
	ErrNotFound  = errors.New("permission request not found")
)

type Reply string

const (
	ReplyOnce   Reply = "once"
	ReplyAlways Reply = "always"
	ReplyReject Reply = "reject"
)

type CorrectedError struct {
	Feedback string
}

func (e *CorrectedError) Error() string {
	return fmt.Sprintf("user rejected with feedback: %s", e.Feedback)
}

type DeniedError struct {
	Ruleset Ruleset
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("denied by rules: %v", e.Ruleset)
}

type Request struct {
	ID         string            `json:"id"`
	SessionID  string            `json:"sessionID"`
	Permission string            `json:"permission"`
	Patterns   []string          `json:"patterns"`
	Metadata   map[string]any    `json:"metadata"`
	Always     []string          `json:"always"`
	Tool       *ToolRef          `json:"tool,omitempty"`
}

type ToolRef struct {
	MessageID string `json:"messageID"`
	CallID    string `json:"callID"`
}

type ReplyInput struct {
	RequestID string `json:"requestID"`
	Reply     Reply  `json:"reply"`
	Message   string `json:"message,omitempty"`
}

type AskInput struct {
	ID         string
	SessionID  string
	Permission string
	Patterns   []string
	Metadata   map[string]any
	Always     []string
	Tool       *ToolRef
	Ruleset    Ruleset
}

type pendingEntry struct {
	info    Request
	replyCh chan replyResult
}

type replyResult struct {
	err error
}

type Service struct {
	mu       sync.Mutex
	bus      *bus.Bus
	pending  map[string]*pendingEntry
	approved Ruleset
	closed   bool
}

func NewService(b *bus.Bus) *Service {
	return &Service{
		bus:     b,
		pending: make(map[string]*pendingEntry),
	}
}

// Ask evaluates rules and either allows, denies, or blocks waiting for user reply.
// Returns nil if allowed, an error if denied or rejected.
// The ctx controls cancellation of the blocking wait.
func (s *Service) Ask(ctx context.Context, input AskInput) error {
	s.mu.Lock()

	needsAsk := false
	for _, pattern := range input.Patterns {
		rule := Evaluate(input.Permission, pattern, input.Ruleset, s.approved)
		if rule.Action == ActionDeny {
			matching := filterMatching(input.Permission, input.Ruleset)
			s.mu.Unlock()
			return &DeniedError{Ruleset: matching}
		}
		if rule.Action == ActionAsk {
			needsAsk = true
		}
	}

	if !needsAsk {
		s.mu.Unlock()
		return nil
	}

	reqID := input.ID
	if reqID == "" {
		var err error
		reqID, err = id.Ascending("permission")
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("generating permission ID: %w", err)
		}
	}

	info := Request{
		ID:         reqID,
		SessionID:  input.SessionID,
		Permission: input.Permission,
		Patterns:   input.Patterns,
		Metadata:   input.Metadata,
		Always:     input.Always,
		Tool:       input.Tool,
	}

	entry := &pendingEntry{
		info:    info,
		replyCh: make(chan replyResult, 1),
	}
	s.pending[reqID] = entry
	s.mu.Unlock()

	s.bus.Publish("permission.asked", info)

	select {
	case result := <-entry.replyCh:
		return result.err
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, reqID)
		s.mu.Unlock()
		return ctx.Err()
	}
}

// RespondToAsk handles a user's reply to a permission request.
func (s *Service) RespondToAsk(input ReplyInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.pending[input.RequestID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, input.RequestID)
	}

	delete(s.pending, input.RequestID)

	s.bus.Publish("permission.replied", map[string]any{
		"sessionID": entry.info.SessionID,
		"requestID": entry.info.ID,
		"reply":     string(input.Reply),
	})

	if input.Reply == ReplyReject {
		var err error
		if input.Message != "" {
			err = &CorrectedError{Feedback: input.Message}
		} else {
			err = ErrRejected
		}
		entry.replyCh <- replyResult{err: err}

		// Cascade: reject all other pending asks for the same session
		for id, other := range s.pending {
			if other.info.SessionID != entry.info.SessionID {
				continue
			}
			delete(s.pending, id)
			s.bus.Publish("permission.replied", map[string]any{
				"sessionID": other.info.SessionID,
				"requestID": other.info.ID,
				"reply":     string(ReplyReject),
			})
			other.replyCh <- replyResult{err: ErrRejected}
		}
		return nil
	}

	entry.replyCh <- replyResult{err: nil}

	if input.Reply == ReplyOnce {
		return nil
	}

	// "always" — persist the approval rules and auto-resolve matching pending asks
	for _, pattern := range entry.info.Always {
		s.approved = append(s.approved, Rule{
			Permission: entry.info.Permission,
			Pattern:    pattern,
			Action:     ActionAllow,
		})
	}

	for id, other := range s.pending {
		if other.info.SessionID != entry.info.SessionID {
			continue
		}
		allAllowed := true
		for _, pattern := range other.info.Patterns {
			if Evaluate(other.info.Permission, pattern, s.approved).Action != ActionAllow {
				allAllowed = false
				break
			}
		}
		if !allAllowed {
			continue
		}
		delete(s.pending, id)
		s.bus.Publish("permission.replied", map[string]any{
			"sessionID": other.info.SessionID,
			"requestID": other.info.ID,
			"reply":     string(ReplyAlways),
		})
		other.replyCh <- replyResult{err: nil}
	}

	return nil
}

// List returns all currently pending permission requests.
func (s *Service) List() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Request, 0, len(s.pending))
	for _, entry := range s.pending {
		result = append(result, entry.info)
	}
	return result
}

// Close rejects all pending requests and prevents new ones.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	for id, entry := range s.pending {
		delete(s.pending, id)
		entry.replyCh <- replyResult{err: ErrRejected}
	}
}

// FromConfig converts config permission rules into a Ruleset.
func FromConfig(allow, deny []string) Ruleset {
	var rules Ruleset
	for _, pattern := range allow {
		parts := splitPermissionPattern(pattern)
		rules = append(rules, Rule{
			Permission: parts[0],
			Pattern:    expandPath(parts[1]),
			Action:     ActionAllow,
		})
	}
	for _, pattern := range deny {
		parts := splitPermissionPattern(pattern)
		rules = append(rules, Rule{
			Permission: parts[0],
			Pattern:    expandPath(parts[1]),
			Action:     ActionDeny,
		})
	}
	return rules
}

func splitPermissionPattern(s string) [2]string {
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return [2]string{s, "*"}
	}
	return [2]string{s[:idx], s[idx+1:]}
}

func expandPath(pattern string) string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return pattern
	}
	if strings.HasPrefix(pattern, "~/") {
		return home + pattern[1:]
	}
	if pattern == "~" {
		return home
	}
	if strings.HasPrefix(pattern, "$HOME/") {
		return home + pattern[5:]
	}
	if pattern == "$HOME" {
		return home
	}
	return pattern
}

func filterMatching(permission string, ruleset Ruleset) Ruleset {
	var result Ruleset
	for _, rule := range ruleset {
		if WildcardMatch(permission, rule.Permission) {
			result = append(result, rule)
		}
	}
	return result
}
