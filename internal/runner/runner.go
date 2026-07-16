package runner

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/store"
)

type Runner struct {
	registry *adapters.Registry
	store    store.RunStore
	logger   *slog.Logger
	mutex    sync.Mutex
	jobLocks map[string]*sync.Mutex
}

func New(registry *adapters.Registry, store store.RunStore, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		registry: registry,
		store:    store,
		logger:   logger,
		jobLocks: make(map[string]*sync.Mutex),
	}
}

func (runner *Runner) RunJob(ctx context.Context, job core.JobSpec) (finishedRun core.JobRun) {
	startedAt := time.Now()
	run := core.JobRun{
		ID:        runner.nextRunID(),
		JobID:     job.ID,
		Adapter:   job.Adapter,
		Status:    core.RunStatusFailed,
		StartedAt: startedAt,
	}

	jobLock := runner.lockJob(job.ID)
	jobLock.Lock()
	defer jobLock.Unlock()

	defer func() {
		if recovered := recover(); recovered != nil {
			run.Error = fmt.Sprintf("adapter panic: %v", recovered)
			finishedRun = runner.finish(run, core.RunResult{
				Status:  core.RunStatusFailed,
				Summary: "adapter panicked",
			})
		}
	}()

	adapter, exists := runner.registry.Get(job.Adapter)
	if !exists {
		run.Error = fmt.Sprintf("adapter %q is not registered", job.Adapter)
		return runner.finish(run, core.RunResult{Status: core.RunStatusFailed})
	}

	timeout := 30 * time.Second
	if job.Timeout != "" {
		parsedTimeout, err := time.ParseDuration(job.Timeout)
		if err != nil {
			run.Error = fmt.Sprintf("invalid timeout %q: %v", job.Timeout, err)
			return runner.finish(run, core.RunResult{Status: core.RunStatusFailed})
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
	if errors.Is(runContext.Err(), context.Canceled) {
		if run.Error == "" {
			run.Error = "job canceled"
		}
		result.Status = core.RunStatusCanceled
	}
	if result.Status == "" {
		if err != nil {
			result.Status = core.RunStatusFailed
		} else {
			result.Status = core.RunStatusSuccess
		}
	}

	return runner.finish(run, result)
}

func (runner *Runner) finish(run core.JobRun, result core.RunResult) core.JobRun {
	run.FinishedAt = time.Now()
	run.DurationMillis = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	run.Result = result
	run.Status = result.Status
	run.Summary = result.Summary

	persistContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := runner.store.SaveRun(persistContext, run); err != nil {
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

func (runner *Runner) lockJob(jobID string) *sync.Mutex {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()

	jobLock, exists := runner.jobLocks[jobID]
	if !exists {
		jobLock = &sync.Mutex{}
		runner.jobLocks[jobID] = jobLock
	}
	return jobLock
}

func (runner *Runner) nextRunID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		binary.BigEndian.PutUint64(bytes[0:8], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(bytes[8:16], uint64(time.Now().UnixNano()))
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}
