package safego

import (
	"bytes"
	"log/slog"
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
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })

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

	// Give slog a moment to flush.
	time.Sleep(50 * time.Millisecond)

	logged := buf.String()
	if logged == "" {
		t.Fatal("expected log output, got empty string")
	}
	if !bytes.Contains(buf.Bytes(), []byte("recovered panic")) {
		t.Errorf("expected log to contain 'recovered panic', got: %s", logged)
	}
	if !bytes.Contains(buf.Bytes(), []byte("boom")) {
		t.Errorf("expected log to contain panic value 'boom', got: %s", logged)
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
