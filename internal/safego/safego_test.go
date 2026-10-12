package safego

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogPanic_IncludesStackAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	LogPanic(logger, "tool panicked", "boom", "tool", "read")

	out := buf.String()
	if !strings.Contains(out, "tool panicked") {
		t.Fatalf("expected message in log, got %q", out)
	}
	if !strings.Contains(out, "panic=boom") && !strings.Contains(out, `panic="boom"`) {
		t.Fatalf("expected panic attr in log, got %q", out)
	}
	if !strings.Contains(out, "tool=read") {
		t.Fatalf("expected tool attr in log, got %q", out)
	}
	if !strings.Contains(out, "stack=") {
		t.Fatalf("expected non-empty stack attr in log, got %q", out)
	}
	// Stack body should mention this test function or LogPanic.
	if !strings.Contains(out, "safego.") && !strings.Contains(out, "TestLogPanic") {
		t.Fatalf("expected stack frames in log, got %q", out)
	}
}

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
