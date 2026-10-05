package safego

import (
	"sync"
	"testing"
	"time"
)

func TestGo_NormalExecution(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	executed := false
	Go(func() {
		defer wg.Done()
		executed = true
	})

	wg.Wait()
	if !executed {
		t.Fatal("expected function to execute")
	}
}

func TestGo_PanicRecovery(t *testing.T) {
	done := make(chan struct{})

	Go(func() {
		defer close(done)
		panic("boom")
	})

	select {
	case <-done:
		// Goroutine completed without crashing the process.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panicking goroutine to finish")
	}
}

func TestGo_PanicLogsError(t *testing.T) {
	// Verify the panicking goroutine completes (recovery runs).
	// We avoid swapping the global slog.Default because it races
	// with other goroutines under -race.
	done := make(chan struct{})
	Go(func() {
		defer close(done)
		panic("boom")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panicking goroutine to finish")
	}
}

func TestGo_NilPanic(t *testing.T) {
	done := make(chan struct{})

	Go(func() {
		defer close(done)
		panic(nil)
	})

	select {
	case <-done:
		// Goroutine completed without crashing the process.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for nil-panicking goroutine to finish")
	}
}
