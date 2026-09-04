package bus

import (
	"sync"
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("test.event")
	defer sub.Unsubscribe()

	b.Publish("test.event", map[string]string{"key": "value"})

	select {
	case evt := <-sub.C:
		if evt.Type != "test.event" {
			t.Errorf("expected type test.event, got %s", evt.Type)
		}
		props, ok := evt.Properties.(map[string]string)
		if !ok {
			t.Fatal("expected map[string]string properties")
		}
		if props["key"] != "value" {
			t.Errorf("expected value, got %s", props["key"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeAll(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.SubscribeAll()
	defer sub.Unsubscribe()

	b.Publish("type.a", nil)
	b.Publish("type.b", nil)

	types := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case evt := <-sub.C:
			types[evt.Type] = true
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}

	if !types["type.a"] || !types["type.b"] {
		t.Errorf("expected both types, got %v", types)
	}
}

func TestTypedSubscriptionFilters(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("only.this")
	defer sub.Unsubscribe()

	b.Publish("not.this", nil)
	b.Publish("only.this", "data")

	select {
	case evt := <-sub.C:
		if evt.Type != "only.this" {
			t.Errorf("expected only.this, got %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	// Should not receive the other event
	select {
	case evt := <-sub.C:
		t.Errorf("unexpected event: %s", evt.Type)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestUnsubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("test")
	sub.Unsubscribe()

	b.Publish("test", nil)

	// Channel should be closed
	_, ok := <-sub.C
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestDoubleUnsubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("test")
	sub.Unsubscribe()
	sub.Unsubscribe() // should not panic
}

func TestSlidingBuffer(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("flood")
	defer sub.Unsubscribe()

	// Publish more events than the buffer capacity
	for i := 0; i < defaultCapacity+100; i++ {
		b.Publish("flood", i)
	}

	// Should still be able to receive events (oldest were dropped)
	count := 0
	for {
		select {
		case <-sub.C:
			count++
		default:
			goto done
		}
	}
done:
	if count == 0 {
		t.Fatal("expected to receive some events")
	}
	if count > defaultCapacity {
		t.Errorf("received more events (%d) than buffer capacity (%d)", count, defaultCapacity)
	}
}

func TestConcurrentPublish(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.SubscribeAll()
	defer sub.Unsubscribe()

	const goroutines = 10
	const eventsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				b.Publish("concurrent", id*1000+j)
			}
		}(i)
	}

	wg.Wait()

	// Drain and count
	count := 0
	for {
		select {
		case <-sub.C:
			count++
		default:
			goto done2
		}
	}
done2:
	if count == 0 {
		t.Fatal("expected to receive events from concurrent publishers")
	}
}

func TestCloseStopsDelivery(t *testing.T) {
	b := New()

	sub := b.SubscribeAll()
	b.Close()

	// Channel should be closed
	_, ok := <-sub.C
	if ok {
		t.Error("expected channel to be closed after bus close")
	}

	// Publish after close should not panic
	b.Publish("after.close", nil)
}

func TestEventHasID(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.SubscribeAll()
	defer sub.Unsubscribe()

	id := b.Publish("test", nil)
	if id == "" {
		t.Error("Publish should return a non-empty event ID")
	}

	select {
	case evt := <-sub.C:
		if evt.ID == "" {
			t.Error("event should have a non-empty ID")
		}
		if evt.ID != id {
			t.Errorf("event ID %s doesn't match published ID %s", evt.ID, id)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
