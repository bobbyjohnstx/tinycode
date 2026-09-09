package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestJobManager_StartAndGet(t *testing.T) {
	jm := NewJobManager()
	id := jm.Start(func(_ context.Context) (string, error) {
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
	id := jm.Start(func(_ context.Context) (string, error) {
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
	id := jm.Start(func(ctx context.Context) (string, error) {
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

	id1 := jm.Start(func(_ context.Context) (string, error) {
		<-blocker
		return "a", nil
	})
	id2 := jm.Start(func(_ context.Context) (string, error) {
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
	id := jm.Start(func(_ context.Context) (string, error) {
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
			id := jm.Start(func(_ context.Context) (string, error) {
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
