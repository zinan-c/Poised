package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/store"
)

type Runner struct {
	registry *adapters.Registry
	store    store.RunStore
	logger   *slog.Logger
	sequence atomic.Uint64
}

func New(registry *adapters.Registry, store store.RunStore, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		registry: registry,
		store:    store,
		logger:   logger,
	}
}

func (runner *Runner) RunJob(ctx context.Context, job core.JobSpec) core.JobRun {
	startedAt := time.Now()
	run := core.JobRun{
		ID:        runner.nextRunID(),
		JobID:     job.ID,
		Adapter:   job.Adapter,
		Status:    core.RunStatusFailed,
		StartedAt: startedAt,
	}

	adapter, exists := runner.registry.Get(job.Adapter)
	if !exists {
		run.Error = fmt.Sprintf("adapter %q is not registered", job.Adapter)
		return runner.finish(ctx, run, core.RunResult{Status: core.RunStatusFailed})
	}

	timeout := 30 * time.Second
	if job.Timeout != "" {
		parsedTimeout, err := time.ParseDuration(job.Timeout)
		if err != nil {
			run.Error = fmt.Sprintf("invalid timeout %q: %v", job.Timeout, err)
			return runner.finish(ctx, run, core.RunResult{Status: core.RunStatusFailed})
		}
		timeout = parsedTimeout
	}

	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := adapter.Run(runContext, core.RunInput{
		JobID:   job.ID,
		Payload: job.Payload,
		Metadata: map[string]any{
			"job_name": job.Name,
		},
	})
	if err != nil {
		run.Error = err.Error()
	}
	if errors.Is(runContext.Err(), context.DeadlineExceeded) {
		run.Error = "job timed out"
		result.Status = core.RunStatusFailed
	}
	if errors.Is(runContext.Err(), context.Canceled) && result.Status == "" {
		result.Status = core.RunStatusCanceled
	}
	if result.Status == "" {
		if err != nil {
			result.Status = core.RunStatusFailed
		} else {
			result.Status = core.RunStatusSuccess
		}
	}

	return runner.finish(ctx, run, result)
}

func (runner *Runner) finish(ctx context.Context, run core.JobRun, result core.RunResult) core.JobRun {
	run.FinishedAt = time.Now()
	run.DurationMillis = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	run.Result = result
	run.Status = result.Status
	run.Summary = result.Summary

	if err := runner.store.SaveRun(ctx, run); err != nil {
		runner.logger.Error("save run failed", "run_id", run.ID, "error", err)
	}

	runner.logger.Info("job run finished",
		"run_id", run.ID,
		"job_id", run.JobID,
		"adapter", run.Adapter,
		"status", run.Status,
		"duration_millis", run.DurationMillis,
	)
	return run
}

func (runner *Runner) nextRunID() string {
	sequence := runner.sequence.Add(1)
	return fmt.Sprintf("%d-%06d", time.Now().UnixNano(), sequence)
}
