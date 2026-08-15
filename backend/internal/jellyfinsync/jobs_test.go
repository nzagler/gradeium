package jellyfinsync

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
)

type syncerFunc func(context.Context, string, Source, []jellyfin.LibraryMapping) (Result, error)

func (function syncerFunc) Sync(ctx context.Context, userID string, source Source, mappings []jellyfin.LibraryMapping) (Result, error) {
	return function(ctx, userID, source, mappings)
}

func TestJobManagerDeduplicatesAndRetainsCompletedResult(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	manager := NewJobManager(root, syncerFunc(func(ctx context.Context, _ string, _ Source, _ []jellyfin.LibraryMapping) (Result, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return Result{Scanned: 3, MoviesAdded: 2, Issues: []Issue{}}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}), time.Minute)
	defer manager.Close()

	status, launched := manager.Start("user", nil, nil)
	if !launched || status.State != JobRunning {
		t.Fatalf("first Start() = (%#v, %v)", status, launched)
	}
	<-started
	status, launched = manager.Start("user", nil, nil)
	if launched || status.State != JobRunning || calls.Load() != 1 {
		t.Fatalf("duplicate Start() = (%#v, %v), calls = %d", status, launched, calls.Load())
	}
	close(release)

	status = waitForJobState(t, manager, "user", JobCompleted)
	if status.Result == nil || status.Result.Scanned != 3 || status.Result.MoviesAdded != 2 {
		t.Fatalf("completed status = %#v", status)
	}
	if retained := manager.Status("user"); retained.State != JobCompleted || retained.Result == nil || retained.Result.Scanned != 3 {
		t.Fatalf("retained status = %#v", retained)
	}
}

func TestJobManagerReportsFailuresSafely(t *testing.T) {
	manager := NewJobManager(context.Background(), syncerFunc(func(context.Context, string, Source, []jellyfin.LibraryMapping) (Result, error) {
		return Result{}, errors.New("upstream included api-key-plaintext")
	}), time.Minute)
	defer manager.Close()

	if _, launched := manager.Start("user", nil, nil); !launched {
		t.Fatal("Start() did not launch the failed job")
	}
	status := waitForJobState(t, manager, "user", JobFailed)
	if status.Message != failedJobMessage || status.Result != nil || strings.Contains(status.Message, "api-key-plaintext") {
		t.Fatalf("failed status exposed unsafe details: %#v", status)
	}
}

func TestJobManagerRootCancellationStopsRunningJob(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	stopped := make(chan error, 1)
	manager := NewJobManager(root, syncerFunc(func(ctx context.Context, _ string, _ Source, _ []jellyfin.LibraryMapping) (Result, error) {
		close(started)
		<-ctx.Done()
		stopped <- ctx.Err()
		return Result{}, ctx.Err()
	}), time.Minute)
	defer manager.Close()

	manager.Start("user", nil, nil)
	<-started
	cancel()
	select {
	case err := <-stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("job stopped with %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("root cancellation did not stop the running job")
	}
	waitForJobState(t, manager, "user", JobFailed)
}

func waitForJobState(t *testing.T, manager *JobManager, userID string, want JobState) JobStatus {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := manager.Status(userID)
		if status.State == want {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("job did not reach %q; last status = %#v", want, manager.Status(userID))
	return JobStatus{}
}
