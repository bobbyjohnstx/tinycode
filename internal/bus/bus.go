package bus

import (
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/id"
)

const defaultCapacity = 4096

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

func (s *Subscription) Unsubscribe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.bus.removeSub(s)
	close(s.ch)
}

type Bus struct {
	mu       sync.RWMutex
	wildcard []*Subscription
	typed    map[string][]*Subscription
	closed   bool
}

func New() *Bus {
	return &Bus{
		typed: make(map[string][]*Subscription),
	}
}

func (b *Bus) Publish(eventType string, properties any) string {
	eventID := id.Create("evt", true)
	evt := Event{
		ID:         eventID,
		Type:       eventType,
		Properties: properties,
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return eventID
	}

	if subs, ok := b.typed[eventType]; ok {
		for _, sub := range subs {
			send(sub, evt)
		}
	}
	for _, sub := range b.wildcard {
		send(sub, evt)
	}

	return eventID
}

func send(sub *Subscription, evt Event) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.ch <- evt:
	default:
		// Sliding buffer: drop oldest, then send
		select {
		case <-sub.ch:
		default:
		}
		select {
		case sub.ch <- evt:
		default:
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
	b.wildcard = append(b.wildcard, sub)
	return sub
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, sub := range b.wildcard {
		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
		sub.mu.Unlock()
	}
	for _, subs := range b.typed {
		for _, sub := range subs {
			sub.mu.Lock()
			if !sub.closed {
				sub.closed = true
				close(sub.ch)
			}
			sub.mu.Unlock()
		}
	}
	b.wildcard = nil
	b.typed = nil
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
