package jellyfinsync

import (
	"context"
	"sync"
	"time"

	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
)

const DefaultJobTimeout = 10 * time.Minute

type JobState string

const (
	JobIdle      JobState = "idle"
	JobRunning   JobState = "running"
	JobCompleted JobState = "completed"
	JobFailed    JobState = "failed"
)

const failedJobMessage = "Jellyfin import could not be completed. Try again."

type JobStatus struct {
	State   JobState `json:"state"`
	Result  *Result  `json:"result,omitempty"`
	Message string   `json:"message,omitempty"`
}

type Syncer interface {
	Sync(context.Context, string, Source, []jellyfin.LibraryMapping) (Result, error)
}

// JobManager owns short-lived, in-memory Jellyfin imports for the lifetime of
// the Gradeium process. Its context is derived from the application context,
// never from the HTTP request that starts an import.
type JobManager struct {
	ctx     context.Context
	cancel  context.CancelFunc
	syncer  Syncer
	timeout time.Duration

	mu      sync.Mutex
	jobs    map[string]JobStatus
	stopped bool
	wait    sync.WaitGroup
}

func NewJobManager(root context.Context, service Syncer, timeout time.Duration) *JobManager {
	if timeout <= 0 {
		timeout = DefaultJobTimeout
	}
	ctx, cancel := context.WithCancel(root)
	return &JobManager{
		ctx:     ctx,
		cancel:  cancel,
		syncer:  service,
		timeout: timeout,
		jobs:    make(map[string]JobStatus),
	}
}

// Start begins an import unless this user already has one running. The bool is
// true only when a new goroutine was launched.
func (manager *JobManager) Start(userID string, source Source, mappings []jellyfin.LibraryMapping) (JobStatus, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if current := manager.jobs[userID]; current.State == JobRunning {
		return cloneJobStatus(current), false
	}
	if manager.stopped {
		failed := JobStatus{State: JobFailed, Message: failedJobMessage}
		manager.jobs[userID] = failed
		return failed, false
	}

	status := JobStatus{State: JobRunning}
	manager.jobs[userID] = status
	ownedMappings := append([]jellyfin.LibraryMapping(nil), mappings...)
	manager.wait.Add(1)
	go manager.run(userID, source, ownedMappings)
	return status, true
}

func (manager *JobManager) Status(userID string) JobStatus {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	status, ok := manager.jobs[userID]
	if !ok {
		return JobStatus{State: JobIdle}
	}
	return cloneJobStatus(status)
}

// Close cancels active imports and waits for their goroutines to exit. The app
// calls this before closing PostgreSQL resources.
func (manager *JobManager) Close() {
	manager.mu.Lock()
	if !manager.stopped {
		manager.stopped = true
		manager.cancel()
	}
	manager.mu.Unlock()
	manager.wait.Wait()
}

func (manager *JobManager) run(userID string, source Source, mappings []jellyfin.LibraryMapping) {
	defer manager.wait.Done()
	ctx, cancel := context.WithTimeout(manager.ctx, manager.timeout)
	defer cancel()

	result, err := manager.syncer.Sync(ctx, userID, source, mappings)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err != nil {
		manager.jobs[userID] = JobStatus{State: JobFailed, Message: failedJobMessage}
		return
	}
	manager.jobs[userID] = JobStatus{State: JobCompleted, Result: &result}
}

func cloneJobStatus(status JobStatus) JobStatus {
	copy := status
	if status.Result != nil {
		result := *status.Result
		result.Issues = append([]Issue(nil), status.Result.Issues...)
		copy.Result = &result
	}
	return copy
}
