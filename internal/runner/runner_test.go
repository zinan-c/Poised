package runner

import (
	"context"
	"testing"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/adapters/examples/echo"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/store"
)

func TestRunJobSavesRun(t *testing.T) {
	registry := adapters.NewRegistry()
	if err := registry.Register(echo.New()); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	runStore := store.NewMemoryRunStore()
	jobRunner := New(registry, runStore, nil)

	run := jobRunner.RunJob(context.Background(), core.JobSpec{
		ID:      "example",
		Adapter: "echo",
		Timeout: "1s",
		Payload: []byte(`{"message":"hello"}`),
	})
	if run.Status != core.RunStatusSuccess {
		t.Fatalf("unexpected run status: %s", run.Status)
	}

	runs, err := runStore.ListRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("unexpected run count: %d", len(runs))
	}
}

func TestRunJobPersistsWithCanceledParentContext(t *testing.T) {
	registry := adapters.NewRegistry()
	if err := registry.Register(echo.New()); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	runStore := &recordingStore{}
	jobRunner := New(registry, runStore, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run := jobRunner.RunJob(ctx, core.JobSpec{
		ID:      "example",
		Adapter: "echo",
		Timeout: "1s",
		Payload: []byte(`{"message":"hello"}`),
	})

	if run.Status != core.RunStatusCanceled {
		t.Fatalf("unexpected run status: %s", run.Status)
	}
	if len(runStore.runs) != 1 {
		t.Fatalf("expected saved run, got %d", len(runStore.runs))
	}
	if err := runStore.saveContextErr; err != nil {
		t.Fatalf("save context should be independent from canceled parent: %v", err)
	}
}

func TestRunJobRecoversAdapterPanic(t *testing.T) {
	registry := adapters.NewRegistry()
	if err := registry.Register(panicAdapter{}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	runStore := &recordingStore{}
	jobRunner := New(registry, runStore, nil)

	run := jobRunner.RunJob(context.Background(), core.JobSpec{
		ID:      "panic",
		Adapter: "panic",
		Timeout: "1s",
		Payload: []byte(`{}`),
	})

	if run.Status != core.RunStatusFailed {
		t.Fatalf("unexpected run status: %s", run.Status)
	}
	if len(runStore.runs) != 1 {
		t.Fatalf("expected saved run, got %d", len(runStore.runs))
	}
}

type recordingStore struct {
	runs           []core.JobRun
	saveContextErr error
}

func (store *recordingStore) SaveRun(ctx context.Context, run core.JobRun) error {
	store.saveContextErr = ctx.Err()
	store.runs = append(store.runs, run)
	return nil
}

func (store *recordingStore) ListRuns(ctx context.Context, limit int) ([]core.JobRun, error) {
	return store.runs, nil
}

type panicAdapter struct{}

func (panicAdapter) Name() string {
	return "panic"
}

func (panicAdapter) Kind() string {
	return "test"
}

func (panicAdapter) Run(ctx context.Context, input core.RunInput) (core.RunResult, error) {
	panic("boom")
}
