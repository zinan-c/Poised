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
