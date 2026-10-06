package bus

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/bobbyjohnstx/tinycode/internal/id"
)

const (
	defaultCapacity = 4096
	historySize     = 256
)

type Event struct {
	ID         string
	Type       string
	Properties any
}

type Subscription struct {
	C      <-chan Event
	ch     chan Event
	bus    *Bus
	typ    string
	closed bool
	mu     sync.Mutex
}

// Unsubscribe removes this subscription and closes its channel.
// Lock order: never hold sub.mu while taking bus.mu.
func (s *Subscription) Unsubscribe() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	ch := s.ch
	b := s.bus
	s.mu.Unlock()

	if b != nil {
		b.removeSub(s)
	}
	close(ch)
}

type Bus struct {
	mu       sync.RWMutex
	wildcard []*Subscription
	typed    map[string][]*Subscription
	closed   bool
	drops    atomic.Uint64

	ringMu  sync.Mutex
	ring    [historySize]Event
	ringPos int
	ringLen int
}

func New() *Bus {
	return &Bus{
		typed: make(map[string][]*Subscription),
	}
}

// Drops returns the number of events dropped by sliding-buffer backpressure.
func (b *Bus) Drops() uint64 {
	return b.drops.Load()
}

func (b *Bus) Publish(eventType string, properties any) string {
	eventID, _ := id.Ascending("event")
	evt := Event{
		ID:         eventID,
		Type:       eventType,
		Properties: properties,
	}

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return eventID
	}

	n := len(b.typed[eventType]) + len(b.wildcard)
	targets := make([]*Subscription, 0, n)
	targets = append(targets, b.typed[eventType]...)
	targets = append(targets, b.wildcard...)
	b.mu.RUnlock()

	b.recordHistory(evt)

	for _, sub := range targets {
		b.send(sub, evt)
	}

	return eventID
}

// EventsSince returns events published after lastID from the recent history
// ring. found is true when lastID was present in the ring (replay is complete
// from that point). When lastID is empty, found is false and events is nil.
func (b *Bus) EventsSince(lastID string) (events []Event, found bool) {
	if lastID == "" {
		return nil, false
	}

	b.ringMu.Lock()
	defer b.ringMu.Unlock()

	if b.ringLen == 0 {
		return nil, false
	}

	start := (b.ringPos - b.ringLen + historySize) % historySize
	idx := -1
	for i := 0; i < b.ringLen; i++ {
		pos := (start + i) % historySize
		if b.ring[pos].ID == lastID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, false
	}

	n := b.ringLen - idx - 1
	if n == 0 {
		return nil, true
	}
	events = make([]Event, 0, n)
	for i := idx + 1; i < b.ringLen; i++ {
		pos := (start + i) % historySize
		events = append(events, b.ring[pos])
	}
	return events, true
}

func (b *Bus) recordHistory(evt Event) {
	b.ringMu.Lock()
	defer b.ringMu.Unlock()
	b.ring[b.ringPos] = evt
	b.ringPos = (b.ringPos + 1) % historySize
	if b.ringLen < historySize {
		b.ringLen++
	}
}

// send delivers evt to sub using a sliding buffer: when full, drop oldest
// until the newest event can be enqueued. Never holds bus.mu.
func (b *Bus) send(sub *Subscription, evt Event) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.ch <- evt:
		return
	default:
	}

	// Buffer full: drop oldest and retry until newest is enqueued.
	for {
		select {
		case <-sub.ch:
			b.drops.Add(1)
			slog.Debug("bus dropped event (buffer full)", "subscription", sub.typ, "drops", b.drops.Load())
		default:
		}
		select {
		case sub.ch <- evt:
			return
		default:
			if sub.closed {
				return
			}
			// Concurrent consumer drained between drop and send, or buffer
			// filled again; loop and retry.
		}
	}
}

func (b *Bus) Subscribe(eventType string) *Subscription {
	ch := make(chan Event, defaultCapacity)
	sub := &Subscription{
		C:   ch,
		ch:  ch,
		bus: b,
		typ: eventType,
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		sub.closed = true
		close(ch)
		return sub
	}
	b.typed[eventType] = append(b.typed[eventType], sub)
	return sub
}

func (b *Bus) SubscribeAll() *Subscription {
	ch := make(chan Event, defaultCapacity)
	sub := &Subscription{
		C:   ch,
		ch:  ch,
		bus: b,
		typ: "*",
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		sub.closed = true
		close(ch)
		return sub
	}
	b.wildcard = append(b.wildcard, sub)
	return sub
}

// Close marks the bus closed and closes all subscription channels.
// Lock order: bus.mu first (collect), then each sub.mu (close).
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := make([]*Subscription, 0, len(b.wildcard))
	subs = append(subs, b.wildcard...)
	for _, typed := range b.typed {
		subs = append(subs, typed...)
	}
	b.wildcard = nil
	b.typed = nil
	b.mu.Unlock()

	for _, sub := range subs {
		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
		sub.mu.Unlock()
	}
}

func (b *Bus) removeSub(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}

	if sub.typ == "*" {
		b.wildcard = removeSub(b.wildcard, sub)
	} else {
		if subs, ok := b.typed[sub.typ]; ok {
			b.typed[sub.typ] = removeSub(subs, sub)
			if len(b.typed[sub.typ]) == 0 {
				delete(b.typed, sub.typ)
			}
		}
	}
}

func removeSub(subs []*Subscription, target *Subscription) []*Subscription {
	for i, s := range subs {
		if s == target {
			return append(subs[:i], subs[i+1:]...)
		}
	}
	return subs
}
