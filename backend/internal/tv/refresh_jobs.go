package tv

import (
	"context"
	"sync"
	"time"
)

const DefaultRefreshJobTimeout = 20 * time.Minute

type BulkRefreshIssue struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

type BulkRefreshResult struct {
	Total     int                `json:"total"`
	Refreshed int                `json:"refreshed"`
	Failed    int                `json:"failed"`
	Issues    []BulkRefreshIssue `json:"issues"`
}

func (service *Service) RefreshAll(ctx context.Context, userID string) (BulkRefreshResult, error) {
	library, err := service.store.List(ctx, userID, false)
	if err != nil {
		return BulkRefreshResult{}, err
	}
	backlog, err := service.store.List(ctx, userID, true)
	if err != nil {
		return BulkRefreshResult{}, err
	}
	items := append(library, backlog...)
	result := BulkRefreshResult{Total: len(items), Issues: []BulkRefreshIssue{}}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		show, mapping, err := service.metadata(ctx, item.ProviderID)
		if err == nil {
			_, err = service.store.Refresh(ctx, userID, show, mapping)
		}
		if err != nil {
			result.Failed++
			result.Issues = append(result.Issues, BulkRefreshIssue{Title: item.Title, Reason: "Metadata could not be refreshed."})
			continue
		}
		result.Refreshed++
	}
	return result, nil
}

type RefreshJobState string

const (
	RefreshJobIdle      RefreshJobState = "idle"
	RefreshJobRunning   RefreshJobState = "running"
	RefreshJobCompleted RefreshJobState = "completed"
	RefreshJobFailed    RefreshJobState = "failed"
)

type RefreshJobStatus struct {
	State   RefreshJobState    `json:"state"`
	Result  *BulkRefreshResult `json:"result,omitempty"`
	Message string             `json:"message,omitempty"`
}

type RefreshJobManager struct {
	ctx     context.Context
	cancel  context.CancelFunc
	service *Service
	timeout time.Duration

	mu      sync.Mutex
	jobs    map[string]RefreshJobStatus
	stopped bool
	wait    sync.WaitGroup
}

func NewRefreshJobManager(root context.Context, service *Service, timeout time.Duration) *RefreshJobManager {
	if timeout <= 0 {
		timeout = DefaultRefreshJobTimeout
	}
	ctx, cancel := context.WithCancel(root)
	return &RefreshJobManager{ctx: ctx, cancel: cancel, service: service, timeout: timeout, jobs: make(map[string]RefreshJobStatus)}
}

func (manager *RefreshJobManager) Start(userID string) (RefreshJobStatus, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if current := manager.jobs[userID]; current.State == RefreshJobRunning {
		return cloneRefreshJobStatus(current), false
	}
	if manager.stopped {
		failed := RefreshJobStatus{State: RefreshJobFailed, Message: "TV metadata refresh could not be started."}
		manager.jobs[userID] = failed
		return failed, false
	}
	status := RefreshJobStatus{State: RefreshJobRunning}
	manager.jobs[userID] = status
	manager.wait.Add(1)
	go manager.run(userID)
	return status, true
}

func (manager *RefreshJobManager) Status(userID string) RefreshJobStatus {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	status, ok := manager.jobs[userID]
	if !ok {
		return RefreshJobStatus{State: RefreshJobIdle}
	}
	return cloneRefreshJobStatus(status)
}

func (manager *RefreshJobManager) Close() {
	manager.mu.Lock()
	if !manager.stopped {
		manager.stopped = true
		manager.cancel()
	}
	manager.mu.Unlock()
	manager.wait.Wait()
}

func (manager *RefreshJobManager) run(userID string) {
	defer manager.wait.Done()
	ctx, cancel := context.WithTimeout(manager.ctx, manager.timeout)
	defer cancel()
	result, err := manager.service.RefreshAll(ctx, userID)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err != nil {
		manager.jobs[userID] = RefreshJobStatus{State: RefreshJobFailed, Message: "TV metadata refresh could not be completed. Try again."}
		return
	}
	manager.jobs[userID] = RefreshJobStatus{State: RefreshJobCompleted, Result: &result}
}

func cloneRefreshJobStatus(status RefreshJobStatus) RefreshJobStatus {
	copy := status
	if status.Result != nil {
		result := *status.Result
		result.Issues = append([]BulkRefreshIssue(nil), status.Result.Issues...)
		copy.Result = &result
	}
	return copy
}
