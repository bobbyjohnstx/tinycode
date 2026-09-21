package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
)

func TestJobManager_StartAndGet(t *testing.T) {
	jm := NewJobManager()
	id := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "done", nil
	})

	if id == "" {
		t.Fatal("expected non-empty job ID")
	}

	// Wait for completion
	job, err := jm.Wait(id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if job.Status != JobCompleted {
		t.Errorf("expected completed, got %s", job.Status)
	}
	if job.Result != "done" {
		t.Errorf("expected result 'done', got %q", job.Result)
	}
}

func TestJobManager_FailedJob(t *testing.T) {
	jm := NewJobManager()
	id := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "", fmt.Errorf("something broke")
	})

	job, err := jm.Wait(id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if job.Status != JobFailed {
		t.Errorf("expected failed, got %s", job.Status)
	}
	if job.Error != "something broke" {
		t.Errorf("expected error message, got %q", job.Error)
	}
}

func TestJobManager_Cancel(t *testing.T) {
	jm := NewJobManager()
	started := make(chan struct{})
	id := jm.Start(context.Background(), func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})

	<-started
	if err := jm.Cancel(id); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	job, err := jm.Wait(id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if job.Status != JobCancelled {
		t.Errorf("expected cancelled, got %s", job.Status)
	}
}

func TestJobManager_List(t *testing.T) {
	jm := NewJobManager()
	blocker := make(chan struct{})

	id1 := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		<-blocker
		return "a", nil
	})
	id2 := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		<-blocker
		return "b", nil
	})

	jobs := jm.List()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	ids := map[string]bool{id1: false, id2: false}
	for _, j := range jobs {
		ids[j.ID] = true
	}
	for id, found := range ids {
		if !found {
			t.Errorf("job %s not found in list", id)
		}
	}

	close(blocker)
	jm.Wait(id1)
	jm.Wait(id2)
}

func TestJobManager_GetNotFound(t *testing.T) {
	jm := NewJobManager()
	job := jm.Get("nonexistent")
	if job != nil {
		t.Error("expected nil for nonexistent job")
	}
}

func TestJobManager_WaitNotFound(t *testing.T) {
	jm := NewJobManager()
	_, err := jm.Wait("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestJobManager_CancelNotRunning(t *testing.T) {
	jm := NewJobManager()
	id := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "ok", nil
	})
	jm.Wait(id)

	err := jm.Cancel(id)
	if err == nil {
		t.Error("expected error when cancelling completed job")
	}
}

func TestJobManager_ConcurrentAccess(t *testing.T) {
	jm := NewJobManager()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := jm.Start(context.Background(), func(_ context.Context) (string, error) {
				time.Sleep(time.Millisecond)
				return fmt.Sprintf("result-%d", n), nil
			})
			jm.Get(id)
			jm.List()
			jm.Wait(id)
		}(i)
	}

	wg.Wait()

	jobs := jm.List()
	if len(jobs) != 50 {
		t.Errorf("expected 50 jobs, got %d", len(jobs))
	}
}

func TestJobManager_ParentContextCancel(t *testing.T) {
	jm := NewJobManager()
	parentCtx, parentCancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	id := jm.Start(parentCtx, func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})

	<-started
	parentCancel()

	job, err := jm.Wait(id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if job.Status != JobCancelled {
		t.Errorf("expected cancelled after parent cancel, got %s", job.Status)
	}
}

func TestJobManager_Shutdown(t *testing.T) {
	jm := NewJobManager()
	const n = 3
	started := make([]chan struct{}, n)
	ids := make([]string, n)

	for i := 0; i < n; i++ {
		started[i] = make(chan struct{})
		ch := started[i]
		ids[i] = jm.Start(context.Background(), func(ctx context.Context) (string, error) {
			close(ch)
			<-ctx.Done()
			return "", ctx.Err()
		})
	}

	for _, ch := range started {
		<-ch
	}

	jm.Shutdown()

	for i, id := range ids {
		job, err := jm.Wait(id)
		if err != nil {
			t.Fatalf("Wait for job %d failed: %v", i, err)
		}
		if job.Status != JobCancelled {
			t.Errorf("job %d: expected cancelled after Shutdown, got %s", i, job.Status)
		}
	}
}

func TestJobManager_PruneCompleted(t *testing.T) {
	jm := NewJobManager()

	// Start a job and wait for completion.
	id1 := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "old", nil
	})
	jm.Wait(id1)

	// Backdate CreatedAt beyond the TTL to simulate expiry.
	jm.mu.Lock()
	jm.jobs[id1].CreatedAt = time.Now().Add(-jobRetentionTTL - time.Second)
	jm.mu.Unlock()

	// Start a new job — this triggers pruneCompleted.
	id2 := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "new", nil
	})
	jm.Wait(id2)

	// Old job should be evicted.
	if got := jm.Get(id1); got != nil {
		t.Errorf("expected old job %s to be pruned, but it still exists", id1)
	}
	// New job should still exist.
	if got := jm.Get(id2); got == nil {
		t.Errorf("expected new job %s to exist", id2)
	}
}

func TestJobManager_CompletionEvent(t *testing.T) {
	b := bus.New()
	jm := NewJobManager(b)

	sub := b.Subscribe("job.completed")
	defer sub.Unsubscribe()

	id := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "hello", nil
	})
	jm.Wait(id)

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatalf("expected map properties, got %T", evt.Properties)
		}
		if props["jobID"] != id {
			t.Errorf("expected jobID %s, got %v", id, props["jobID"])
		}
		if props["status"] != "completed" {
			t.Errorf("expected status completed, got %v", props["status"])
		}
		if props["result"] != "hello" {
			t.Errorf("expected result hello, got %v", props["result"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job.completed event")
	}
}

func TestJobManager_FailedCompletionEvent(t *testing.T) {
	b := bus.New()
	jm := NewJobManager(b)

	sub := b.Subscribe("job.completed")
	defer sub.Unsubscribe()

	id := jm.Start(context.Background(), func(_ context.Context) (string, error) {
		return "", fmt.Errorf("boom")
	})
	jm.Wait(id)

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatalf("expected map properties, got %T", evt.Properties)
		}
		if props["jobID"] != id {
			t.Errorf("expected jobID %s, got %v", id, props["jobID"])
		}
		if props["status"] != "failed" {
			t.Errorf("expected status failed, got %v", props["status"])
		}
		if props["error"] != "boom" {
			t.Errorf("expected error boom, got %v", props["error"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job.completed event")
	}
}
