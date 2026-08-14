package setup

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

type memorySetupStore struct {
	mutex    sync.Mutex
	complete bool
}

func (store *memorySetupStore) CompleteStatus(context.Context) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return store.complete, nil
}

func (store *memorySetupStore) Complete(context.Context) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.complete {
		return false, nil
	}
	store.complete = true
	return true, nil
}

func TestCompleteTransitionsOnlyOnceUnderConcurrency(t *testing.T) {
	service := NewService(&memorySetupStore{})
	const requests = 16
	var transitions atomic.Int32
	var waitGroup sync.WaitGroup
	for range requests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			transitioned, err := service.Complete(context.Background())
			if err != nil {
				t.Errorf("Complete returned an error: %v", err)
				return
			}
			if transitioned {
				transitions.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	if got := transitions.Load(); got != 1 {
		t.Fatalf("transitions = %d, want 1", got)
	}
	complete, err := service.CompleteStatus(context.Background())
	if err != nil || !complete {
		t.Fatalf("CompleteStatus = (%v, %v), want (true, nil)", complete, err)
	}
}
