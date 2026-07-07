package store

import (
	"context"
	"sync"

	"github.com/zinan-c/Poised/internal/core"
)

type MemoryRunStore struct {
	mutex sync.RWMutex
	runs  []core.JobRun
}

func NewMemoryRunStore() *MemoryRunStore {
	return &MemoryRunStore{
		runs: make([]core.JobRun, 0),
	}
}

func (store *MemoryRunStore) SaveRun(ctx context.Context, run core.JobRun) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.runs = append(store.runs, run)
	return nil
}

func (store *MemoryRunStore) ListRuns(ctx context.Context, limit int) ([]core.JobRun, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	store.mutex.RLock()
	defer store.mutex.RUnlock()

	if limit <= 0 || limit > len(store.runs) {
		limit = len(store.runs)
	}

	startIndex := len(store.runs) - limit
	result := make([]core.JobRun, 0, limit)
	for index := len(store.runs) - 1; index >= startIndex; index-- {
		result = append(result, store.runs[index])
	}

	return result, nil
}
