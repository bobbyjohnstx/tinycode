package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

// JobStatus represents the current state of a background job.
type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// Job holds the state and result of a background task.
type Job struct {
	ID        string    `json:"id"`
	Status    JobStatus `json:"status"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`

	cancel context.CancelFunc
	done   chan struct{}
}

const jobRetentionTTL = 10 * time.Minute

// JobManager manages background jobs with goroutine lifecycle tracking.
type JobManager struct {
	mu   sync.Mutex
	jobs map[string]*Job
	seq  int
	bus  *bus.Bus
}

// NewJobManager creates a new JobManager. An optional bus may be provided
// to receive "job.completed" events when jobs finish.
func NewJobManager(b ...*bus.Bus) *JobManager {
	var eventBus *bus.Bus
	if len(b) > 0 {
		eventBus = b[0]
	}
	return &JobManager{
		jobs: make(map[string]*Job),
		bus:  eventBus,
	}
}

// pruneCompleted removes completed/failed/cancelled jobs older than jobRetentionTTL.
// Must be called with jm.mu held.
func (jm *JobManager) pruneCompleted() {
	now := time.Now()
	for id, job := range jm.jobs {
		if job.Status != JobRunning && now.Sub(job.CreatedAt) > jobRetentionTTL {
			delete(jm.jobs, id)
		}
	}
}

// Start launches fn as a background goroutine and returns the job ID.
// The context is derived from parentCtx so that cancelling the parent
// (e.g. on session abort or server shutdown) also cancels running jobs.
func (jm *JobManager) Start(parentCtx context.Context, fn func(ctx context.Context) (string, error)) string {
	jm.mu.Lock()
	jm.pruneCompleted()
	jm.seq++
	jobID := fmt.Sprintf("job_%d", jm.seq)
	ctx, cancel := context.WithCancel(parentCtx)
	job := &Job{
		ID:        jobID,
		Status:    JobRunning,
		CreatedAt: time.Now(),
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	jm.jobs[jobID] = job
	jm.mu.Unlock()

	safego.Go(func() {
		defer close(job.done)
		result, err := fn(ctx)

		jm.mu.Lock()
		if ctx.Err() != nil {
			job.Status = JobCancelled
		} else if err != nil {
			job.Status = JobFailed
			job.Error = err.Error()
		} else {
			job.Status = JobCompleted
			job.Result = result
		}
		eventBus := jm.bus
		status := job.Status
		jobResult := job.Result
		jobError := job.Error
		jm.mu.Unlock()

		if eventBus != nil {
			eventBus.Publish("job.completed", map[string]any{
				"jobID":  jobID,
				"status": string(status),
				"result": jobResult,
				"error":  jobError,
			})
		}
	})

	return jobID
}

// Get returns a copy of the job state, or nil if not found.
func (jm *JobManager) Get(id string) *Job {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	job, ok := jm.jobs[id]
	if !ok {
		return nil
	}
	// Return a copy so callers cannot race on fields.
	cp := *job
	cp.cancel = nil
	cp.done = nil
	return &cp
}

// Wait blocks until the job completes and returns its result.
func (jm *JobManager) Wait(id string) (*Job, error) {
	jm.mu.Lock()
	job, ok := jm.jobs[id]
	if !ok {
		jm.mu.Unlock()
		return nil, fmt.Errorf("job %s not found", id)
	}
	done := job.done
	jm.mu.Unlock()

	<-done
	return jm.Get(id), nil
}

// Cancel stops a running job.
func (jm *JobManager) Cancel(id string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	job, ok := jm.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	if job.Status != JobRunning {
		return fmt.Errorf("job %s is not running (status: %s)", id, job.Status)
	}
	job.cancel()
	return nil
}

// Shutdown cancels all running jobs.
func (jm *JobManager) Shutdown() {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for _, job := range jm.jobs {
		if job.Status == JobRunning {
			job.cancel()
		}
	}
}

// List returns copies of all jobs.
func (jm *JobManager) List() []*Job {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	result := make([]*Job, 0, len(jm.jobs))
	for _, job := range jm.jobs {
		cp := *job
		cp.cancel = nil
		cp.done = nil
		result = append(result, &cp)
	}
	return result
}
